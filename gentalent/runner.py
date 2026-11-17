from contextlib import contextmanager
from datetime import datetime, timezone
import fcntl
import json
from pathlib import Path
import time
from .net import APIError


def now():
    return datetime.now(timezone.utc).isoformat()


def fresh(message, after):
    try:
        return datetime.fromisoformat(message["timestamp"].replace("Z", "+00:00")) >= datetime.fromisoformat(after)
    except (KeyError, ValueError, TypeError):
        return False


@contextmanager
def lock_database(path):
    path = Path(str(path) + ".lock")
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError("Another GenTalent operation is using this database.") from None
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def arm(store, api):
    if store.get("armed"):
        raise ValueError("Already armed. Restart run normally; rearming could discard missed commands.")
    api.verify_chat()
    page = api.messages()
    with store.db:
        store.set("cursor", page.get("newestCursor") or "")
        store.set("armed", now())
    return "Armed for new messages only. Historical commands will not be processed."


def new_messages(api, cursor, after):
    seen_cursors = set()
    while True:
        page = api.messages(cursor)
        items = sorted(page.get("items", []), key=lambda m: (m.get("timestamp", ""), m.get("sortKey", "")))
        newest = page.get("newestCursor") or cursor
        if not cursor and page.get("hasMore"):
            raise APIError("No baseline cursor and history spans multiple pages; establish a fresh baseline.")
        yield [m for m in items if fresh(m, after)], newest
        if not page.get("hasMore"):
            break
        if not newest or newest == cursor:
            raise APIError("Beeper did not advance a pagination cursor; stopped without guessing.")
        if newest in seen_cursors:
            raise APIError("Beeper repeated a pagination cursor; stopped without guessing.")
        seen_cursors.add(newest)
        cursor = newest


def flush(store, api, live=False):
    for row in store.db.execute("SELECT * FROM outbox WHERE status IN ('queued','pending') ORDER BY rowid").fetchall():
        attachment_path = row["attachment_path"] if "attachment_path" in row.keys() else ""
        if not live:
            if row["status"] == "queued":
                suffix = f" [ATTACHMENT: {attachment_path}]" if attachment_path else ""
                print("PREVIEW:", row["text"] + suffix)
                with store.db:
                    store.db.execute("UPDATE outbox SET status='preview' WHERE id=?", (row["id"],))
            continue
        if row["status"] == "queued":
            with store.db:
                store.db.execute("UPDATE outbox SET status='sending' WHERE id=?", (row["id"],))
            try:
                pending = api.send(row["text"], attachment_path=attachment_path) if attachment_path else api.send(row["text"])
            except Exception:
                with store.db:
                    store.db.execute("UPDATE outbox SET status='uncertain' WHERE id=?", (row["id"],))
                raise APIError(f"Send {row['id']} is uncertain. Inspect the exact thread before resolving it.") from None
            with store.db:
                store.db.execute("UPDATE outbox SET status='pending',pending_id=? WHERE id=?", (pending, row["id"]))
            print(f"Send {row['id']}: pending; recipient confirmation is still required.")
        else:
            try:
                message = api.retrieve(row["pending_id"])
            except APIError as err:
                if getattr(err, "status_code", None) == 404:
                    matched_id = None
                    try:
                        recent = sorted(
                            api.messages().get("items", []),
                            key=lambda m: (m.get("timestamp", ""), m.get("sortKey", "")),
                        )
                        used = {
                            r[0]
                            for r in store.db.execute(
                                "SELECT final_id FROM outbox WHERE final_id IS NOT NULL"
                            ).fetchall()
                        }
                        for item in recent:
                            iid = str(item.get("id") or "")
                            if (
                                iid
                                and not iid.startswith("~")
                                and iid not in used
                                and item.get("isSender")
                                and (item.get("text") or "").strip() == row["text"].strip()
                            ):
                                matched_id = iid
                                break
                    except APIError:
                        pass
                    with store.db:
                        if matched_id:
                            store.db.execute(
                                "UPDATE outbox SET status='sent',final_id=? WHERE id=?",
                                (matched_id, row["id"]),
                            )
                            print(f"Send {row['id']}: matched delivered message {matched_id} after pending ID expired.")
                        else:
                            store.db.execute(
                                "UPDATE outbox SET status='uncertain' WHERE id=?",
                                (row["id"],),
                            )
                            print(f"Send {row['id']}: pending ID expired (HTTP 404); marked uncertain without resending.")
                    continue
                raise
            send_status = (message.get("sendStatus") or {}).get("status")
            msg_id = str(message.get("id") or "")
            delivered_without_status = (
                send_status is None
                and bool(msg_id)
                and msg_id != row["pending_id"]
                and not msg_id.startswith("~")
            )
            if send_status == "SUCCESS" or delivered_without_status:
                with store.db:
                    store.db.execute(
                        "UPDATE outbox SET status='sent',final_id=? WHERE id=?",
                        (msg_id, row["id"]),
                    )
                print(f"Send {row['id']}: network success (id={msg_id}); recipient confirmation is still required.")
            elif send_status in ("FAIL_RETRIABLE", "FAIL_PERMANENT"):
                with store.db:
                    store.db.execute("UPDATE outbox SET status='failed' WHERE id=?", (row["id"],))
                print(f"Send {row['id']}: network reported failure; no automatic duplicate send.")


def poll_once(store, api, live=False):
    after = store.get("armed")
    if not after:
        raise ValueError("Run arm first, in the same dry/live mode, before processing messages.")
    api.verify_chat()
    for messages, cursor in new_messages(api, store.get("cursor") or None, after):
        for message in messages:
            store.handle(message)
        with store.db:
            store.set("cursor", cursor or "")
    store.finalize_due()
    flush(store, api, live)


def run(store, api, live=False, once=False):
    print("LIVE replies enabled." if live else "DRY RUN: isolated ballot, previews only.")
    try:
        while True:
            if once:
                poll_once(store, api, live)
                return
            try:
                poll_once(store, api, live)
            except APIError as error:
                print(f"Transient API error: {error}", flush=True)
            time.sleep(float(store.config.data.get("poll_seconds", 3)))
    except KeyboardInterrupt:
        print("\nStopped. Ballot and cursor are saved; start the same command to resume.")


def probe_read(config, api, seconds=120):
    api.verify_chat()
    baseline = api.messages()
    cursor = baseline.get("newestCursor")
    after = now()
    observed = set()
    seen_platforms = set()
    end = time.monotonic() + seconds
    print("Ready. Send fresh labeled test messages from each phone platform now.")
    try:
        while time.monotonic() < end:
            for messages, newest in new_messages(api, cursor, after):
                for m in messages:
                    if (m.get("id") in observed or m.get("chatID") != config.data["chat_id"]
                            or m.get("accountID") != config.data["account_id"]
                            or m.get("senderID") not in config.roster):
                        continue
                    observed.add(m["id"])
                    member = config.roster[m["senderID"]]
                    seen_platforms.add(member["platform"])
                    print(json.dumps({"sender_id": m["senderID"], "name": member["name"],
                        "platform": member["platform"], "own_message": m.get("isSender"),
                        "text": m.get("text", "")[:240],
                        "attachment_types": [a.get("type") for a in m.get("attachments", [])],
                        "link_count": len(m.get("links", []))}, ensure_ascii=False))
                cursor = newest
            time.sleep(float(config.data.get("poll_seconds", 3)))
    except KeyboardInterrupt:
        pass
    required = {m["platform"] for m in config.roster.values() if m["platform"] != "unknown"}
    print("Observed platforms:", ", ".join(sorted(seen_platforms)) or "none")
    print("Read coverage complete." if required <= seen_platforms else "Read coverage incomplete.")
    print("The separate labeled reply and recipient confirmation are still required.")
