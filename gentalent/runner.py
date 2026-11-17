from contextlib import contextmanager
from datetime import datetime, timezone
import fcntl
from pathlib import Path
import sqlite3
import time
from .net import APIError

SEND_FAILURES = ("FAIL_RETRIABLE", "FAIL_PERMANENT")


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


def same_text(a, b):
    """Compare reply text ignoring whitespace; Beeper may normalize line breaks on the way out."""
    return " ".join((a or "").split()) == " ".join((b or "").split())


def delivery_state(message, pending_id):
    """Classify a message retrieved by its pendingMessageID.

    The spec says sendStatus is present only "when reported by the bridge". Google Messages
    omits it once a message is delivered, so a final ID that differs from the pending ID with
    no sendStatus also means sent. See doc/beeper-api.md.
    """
    status = (message.get("sendStatus") or {}).get("status")
    final_id = str(message.get("id") or "")
    if status in SEND_FAILURES:
        return "failed", final_id
    if status == "SUCCESS" or (status is None and final_id and final_id != pending_id):
        return "sent", final_id
    return "pending", final_id


def flush(store, api, live=False):
    for row in store.db.execute("SELECT * FROM outbox WHERE status IN ('queued','pending') ORDER BY rowid").fetchall():
        if not live:
            if row["status"] == "queued":
                suffix = f" [ATTACHMENT: {row['attachment_path']}]" if row["attachment_path"] else ""
                print("PREVIEW:", row["text"] + suffix)
                with store.db:
                    store.db.execute("UPDATE outbox SET status='preview' WHERE id=?", (row["id"],))
            continue
        if row["status"] == "queued":
            with store.db:
                store.db.execute("UPDATE outbox SET status='sending' WHERE id=?", (row["id"],))
            try:
                pending = api.send(row["text"], attachment_path=row["attachment_path"] or None)
            except Exception:
                with store.db:
                    store.db.execute("UPDATE outbox SET status='uncertain' WHERE id=?", (row["id"],))
                raise APIError(f"Send {row['id']} is uncertain. Check the thread, then use resolve-send.") from None
            with store.db:
                store.db.execute("UPDATE outbox SET status='pending',pending_id=? WHERE id=?", (pending, row["id"]))
            print(f"Send {row['id']}: pending.")
            continue
        try:
            message = api.retrieve(row["pending_id"])
        except APIError as error:
            if error.status_code != 404:
                raise
            with store.db:
                store.db.execute("UPDATE outbox SET status='uncertain' WHERE id=?", (row["id"],))
            print(f"Send {row['id']}: Beeper no longer knows its pending ID (HTTP 404); marked uncertain. "
                  "Check the thread, then use resolve-send.")
            continue
        state, final_id = delivery_state(message, row["pending_id"])
        if state == "pending":
            continue
        with store.db:
            store.db.execute("UPDATE outbox SET status=?,final_id=? WHERE id=?", (state, final_id or None, row["id"]))
        print(f"Send {row['id']}: {state} (id={final_id}).")


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


def open_unlocked(path, read_only=False, timeout=10):
    """Connect without the worker lock, for commands that must work while the live bot runs.

    Never construct Store here: its startup marks in-flight 'sending' rows uncertain.
    """
    if not Path(path).exists():
        raise ValueError(f"No database at {path}.")
    target = Path(path).resolve().as_uri() + ("?mode=ro" if read_only else "")
    db = sqlite3.connect(target, uri=True, timeout=timeout)
    db.row_factory = sqlite3.Row
    return db


def resolve_send(db, api, outbox_id, message_id=None):
    """Record a checked outcome for an uncertain or failed reply. Never re-queues a send.

    With message_id, Beeper must confirm that message is ours, in the configured chat, and
    carries this reply's text. For a photo reply it must carry the same file name and exact
    byte size as the local strip (Beeper's fileSize matched the local file for every photo
    reply sent so far); the pending ID of a photo reply resolves to its IMAGE message.
    Without message_id, the reply is recorded as not delivered (status 'failed').
    """
    row = db.execute("SELECT * FROM outbox WHERE id=?", (outbox_id,)).fetchone()
    if row is None or row["status"] not in ("uncertain", "failed"):
        raise ValueError("Only an uncertain or failed reply can be resolved.")
    if message_id is None:
        with db:
            db.execute("UPDATE outbox SET status='failed' WHERE id=? AND status=?", (outbox_id, row["status"]))
        return f"Reply {outbox_id} recorded as not delivered. It will not be resent."
    if db.execute("SELECT 1 FROM outbox WHERE final_id=? AND id<>?", (message_id, outbox_id)).fetchone():
        raise ValueError(f"Message {message_id} is already recorded for another reply.")
    message = api.retrieve(message_id)
    if not message.get("isSender"):
        raise ValueError(f"Message {message_id} was not sent by the connected account.")
    if row["attachment_path"]:
        local = Path(row["attachment_path"])
        found = [(a.get("fileName"), a.get("fileSize")) for a in message.get("attachments") or []]
        if not local.is_file() or found != [(local.name, local.stat().st_size)]:
            raise ValueError(f"Message {message_id} does not carry this reply's {local.name}.")
    elif not same_text(message.get("text"), row["text"]):
        raise ValueError(f"Message {message_id} text differs from reply {outbox_id}.")
    with db:
        changed = db.execute(
            "UPDATE outbox SET status='sent',final_id=? WHERE id=? AND status=?",
            (message_id, outbox_id, row["status"]),
        ).rowcount
    if changed != 1:
        raise ValueError(f"Reply {outbox_id} changed while resolving; nothing was recorded.")
    return f"Reply {outbox_id} recorded as sent (message {message_id})."


def status_report(db):
    cases = [dict(r) for r in db.execute(
        "SELECT c.id, n.name, c.phase, (SELECT COUNT(*) FROM votes v WHERE v.case_id=c.id) AS votes "
        "FROM cases c JOIN nominees n ON n.id=c.nominee_id ORDER BY c.id")]
    attention = [dict(r) for r in db.execute(
        "SELECT id, status, kind, case_id, substr(text, 1, 80) AS text FROM outbox "
        "WHERE status IN ('queued', 'sending', 'pending', 'uncertain') ORDER BY rowid")]
    counts = dict(db.execute("SELECT status, COUNT(*) FROM outbox GROUP BY status").fetchall())
    return {"cases": cases, "outbox_counts": counts, "outbox_needing_attention": attention}


def contract_report(api, db, spec):
    """Check the live Beeper API against the behavior doc/beeper-api.md relies on. Read-only.

    Returns a list of failures; empty means every check passed.
    """
    failures = []
    schemas = spec.get("components", {}).get("schemas", {})
    statuses = schemas.get("SendStatus", {}).get("properties", {}).get("status", {}).get("enum")
    if sorted(statuses or []) != sorted(["SUCCESS", "PENDING", *SEND_FAILURES]):
        failures.append(f"SendStatus values changed: {statuses}")
    if "sendStatus" in schemas.get("Message", {}).get("required", []):
        failures.append("Message.sendStatus became required; the absent-status rule needs review.")
    get_message = spec.get("paths", {}).get("/v1/chats/{chatID}/messages/{messageID}", {}).get("get", {})
    if "pendingMessageID" not in (get_message.get("description") or ""):
        failures.append("GET message no longer documents lookup by pendingMessageID.")
    rows = db.execute("SELECT id, final_id, text, attachment_path FROM outbox "
                      "WHERE status='sent' AND final_id IS NOT NULL ORDER BY rowid DESC LIMIT 3").fetchall()
    if not rows:
        failures.append("No sent replies recorded yet; message checks skipped.")
    for row in rows:
        message = api.retrieve(row["final_id"])
        state, final_id = delivery_state(message, pending_id="")
        if state != "sent" or final_id != row["final_id"]:
            failures.append(f"Delivered reply {row['id']} (message {row['final_id']}) now reads as {state}.")
        if not message.get("isSender"):
            failures.append(f"Message {row['final_id']} no longer reads as sent by this account.")
        if not row["attachment_path"] and not same_text(message.get("text"), row["text"]):
            failures.append(f"Message {row['final_id']} text differs from what was sent.")
    return failures
