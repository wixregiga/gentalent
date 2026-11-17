import argparse
import getpass
from html import escape
import json
import os
from pathlib import Path
import sys
import uuid
from .config import Config, PREFIX
from .net import Beeper, APIError, secret_write
from .runner import arm, lock_database, probe_read, run, flush
from .state import Store
from . import tumblr

ROOT = Path(__file__).resolve().parent.parent


def make_parser():
    p = argparse.ArgumentParser(description="GenTalent: one verified group, three final votes.")
    p.add_argument("--config", default=str(ROOT / "config.json"))
    sub = p.add_subparsers(dest="command", required=True)
    sub.add_parser("init", help="Create a local config from the example; never overwrite.")
    sub.add_parser("demo", help="Offline complete demo using fictional participants and fake services.")
    sub.add_parser("set-secret", help="Save a Beeper token through a hidden prompt.")
    d = sub.add_parser("doctor", help="Check local API availability; group reads require --chat.")
    d.add_argument("--chat", action="store_true")
    sub.add_parser("accounts", help="List connected Beeper accounts and connection status.")
    lg = sub.add_parser("list-groups", help="List recent group chats (metadata only, no message bodies).")
    lg.add_argument("--account")
    lg.add_argument("--limit", type=int, default=50)
    f = sub.add_parser("find-chat", help="Search group metadata using your exact title words.")
    f.add_argument("query")
    f.add_argument("--account")
    i = sub.add_parser("inspect-chat", help="Inspect metadata for one explicitly selected chat ID.")
    i.add_argument("chat_id")
    c = sub.add_parser("connect-tumblr", help="Authorize Tumblr interactively; no posting.")
    c.add_argument("--port", type=int, default=8766)
    for name in ("arm", "run", "status", "register", "preview", "export", "probe-send",
                 "publish", "reconcile-post", "resolve-send"):
        s = sub.add_parser(name)
        s.add_argument("--live", action="store_true",
                       help="Select the real ballot. run/probe-send/publish enable external writes.")
        if name == "run":
            s.add_argument("--once", action="store_true")
        elif name == "register":
            s.add_argument("identity")
            s.add_argument("name")
            s.add_argument("--alias", action="append", default=[])
            s.add_argument("--previously-denied", action="store_true", help="Import a confirmed prior group denial permanently.")
        elif name == "export":
            s.add_argument("--directory", default="exports")
        elif name == "publish":
            s.add_argument("--blog", help="Required for live publication; must equal config.")
        elif name == "reconcile-post":
            s.add_argument("post_id")
        elif name == "resolve-send":
            s.add_argument("outbox_id")
            s.add_argument("--outcome", required=True, choices=["sent", "not-sent"])
    probe = sub.add_parser("probe-read", help="Watch fresh test messages; does not process votes or send.")
    probe.add_argument("--seconds", type=int, default=120)
    gm = sub.add_parser("generate-media", help="Run the subscription-only multi-model photo & strip builder for a nominee.")
    gm.add_argument("name")
    gm.add_argument("--pitch", default="")
    gm.add_argument("--cli", choices=["codex", "claude", "agy"], default=None)
    return p


def metadata(chat):
    return {key: chat.get(key) for key in ("id", "accountID", "network", "title", "type", "participants")}


def account_summary(account):
    user = account.get("user") or {}
    return {
        "accountID": account.get("accountID"),
        "network": account.get("network"),
        "status": account.get("status"),
        "bridge": account.get("bridge"),
        "user": {k: user.get(k) for k in ("id", "fullName", "username", "isSelf")},
    }


def main(argv=None):
    os.umask(0o077)
    args = make_parser().parse_args(argv)
    if args.command == "demo":
        from .demo import demo
        demo()
        return 0
    if args.command == "init":
        path = Path(args.config).expanduser().resolve()
        if path.exists():
            raise ValueError("Config already exists; no overwrite.")
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text((ROOT / "config.example.json").read_text())
        print("Created", path, "— fill in the confirmed group and roster IDs.")
        return 0
    config = Config.load(args.config)
    if args.command == "set-secret":
        token = getpass.getpass("Beeper access token (hidden; do not paste into chat): ").strip()
        if not token:
            raise ValueError("Token was empty.")
        secret_write(config.path("token_file"), token)
        print("Token saved privately in WSL.")
        return 0
    if args.command == "connect-tumblr":
        tumblr.connect(config, args.port)
        return 0
    if args.command == "generate-media":
        from .generative import SubscriptionAgentRunner
        bundle = SubscriptionAgentRunner(config).generate(args.name, args.pitch, cli_override=args.cli)
        print(json.dumps({
            "nominee_name": bundle.nominee_name,
            "summary": bundle.summary,
            "provider_used": bundle.provider_used,
            "strip_path": str(bundle.strip_path),
            "photo_paths": [str(p) for p in bundle.photo_paths],
            "sources_path": str(bundle.sources_path),
        }, indent=2, ensure_ascii=False))
        return 0
    if args.command in ("accounts", "list-groups", "find-chat", "inspect-chat"):
        api = Beeper(config, discovery=True)
        if args.command == "accounts":
            items = api.request("GET", "/v1/accounts")
            print(json.dumps([account_summary(a) for a in items], indent=2, ensure_ascii=False))
        elif args.command == "list-groups":
            query = {"limit": max(1, min(args.limit, 200))}
            if args.account:
                query["accountIDs"] = [args.account]
            result = api.request("GET", "/v1/chats", query=query)
            groups = [metadata(c) for c in result.get("items", []) if c.get("type") == "group"]
            print(json.dumps({"items": groups, "has_more": result.get("hasMore", False)},
                             indent=2, ensure_ascii=False))
        elif args.command == "find-chat":
            query = {"query": args.query, "type": "group", "limit": 20}
            if args.account:
                query["accountIDs"] = [args.account]
            result = api.request("GET", "/v1/chats/search", query=query)
            print(json.dumps({"items": [metadata(c) for c in result.get("items", [])],
                              "has_more": result.get("hasMore", False)}, indent=2, ensure_ascii=False))
            print("Confirm one underlying Google Messages chat; do not select by title alone.")
        else:
            from urllib.parse import quote
            result = api.request("GET", "/v1/chats/" + quote(args.chat_id, safe=""),
                                 query={"maxParticipantCount": -1})
            print(json.dumps(metadata(result), indent=2, ensure_ascii=False))
        return 0
    if args.command == "doctor":
        api = Beeper(config, discovery=not args.chat)
        api.request("GET", "/v1/info")
        api.request("GET", "/v1/accounts")
        print("Local Beeper API is reachable and token is accepted.")
        if args.chat:
            api.verify_chat()
            print("Exact group, account, and three-person roster match.")
        return 0
    config.validate()
    if args.command == "probe-read":
        if not 1 <= args.seconds <= 3600:
            raise ValueError("Use 1–3600 seconds.")
        probe_read(config, Beeper(config), args.seconds)
        return 0
    path = config.path("database")
    if not args.live:
        path = path.with_name("dryrun-" + path.name)
    with lock_database(path):
        store = Store(path, config)
        try:
            if args.command == "register":
                store.register(args.identity, args.name, args.alias)
                if args.previously_denied:
                    store.import_denial(args.identity)
                print("Registered canonical identity locally. Use the same identity for all aliases.")
            elif args.command == "arm":
                print(arm(store, Beeper(config)))
            elif args.command == "run":
                run(store, Beeper(config), args.live, args.once)
            elif args.command == "status":
                print(json.dumps(store.snapshot(), indent=2, ensure_ascii=False))
            elif args.command == "preview":
                print(json.dumps(store.payload(), indent=2, ensure_ascii=False))
            elif args.command == "export":
                export(store, Path(args.directory))
            elif args.command == "probe-send":
                key = None
                with store.db:
                    key = store.queue("connection test · " + uuid.uuid4().hex[:8] +
                                      " · please confirm receipt on each phone platform", "test")
                flush(store, Beeper(config) if args.live else None, args.live)
                print("Test operation:", key)
            elif args.command == "publish":
                print(json.dumps(store.payload(), indent=2, ensure_ascii=False))
                if args.live:
                    if args.blog != config.data["tumblr_blog"]:
                        raise ValueError("Pass --blog with the exact configured destination for live publication.")
                    print("Public record:", tumblr.publish(store, tumblr.Tumblr(config)))
                else:
                    print("Preview only; no Tumblr connection or post.")
            elif args.command == "reconcile-post":
                if not args.live:
                    raise ValueError("Use --live to reconcile the real blog.")
                print(tumblr.reconcile(store, tumblr.Tumblr(config), args.post_id))
            elif args.command == "resolve-send":
                row = store.db.execute("SELECT * FROM outbox WHERE id=?", (args.outbox_id,)).fetchone()
                if not row or row["status"] not in ("uncertain", "failed"):
                    raise ValueError("Only an uncertain or failed send can be resolved.")
                with store.db:
                    store.db.execute("UPDATE outbox SET status=? WHERE id=?",
                                    ("sent" if args.outcome == "sent" else "queued", args.outbox_id))
                print("Recorded your checked outcome. A not-sent reply will be eligible on the next live run.")
        finally:
            store.close()
    return 0


def export(store, directory):
    directory = directory.expanduser().resolve()
    directory.mkdir(parents=True, exist_ok=True)
    row = store.current()
    payload = store.payload()
    (directory / f"case-{row['id']}.json").write_text(json.dumps(payload, indent=2, ensure_ascii=False))
    if row["phase"] == "certified":
        name = escape(row["name"])
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="700" viewBox="0 0 1200 700">'
               '<rect width="1200" height="700" fill="#151e2b"/>'
               '<rect x="36" y="36" width="1128" height="628" rx="18" fill="none" stroke="#d6b56d" stroke-width="3"/>'
               '<g fill="#f3e2b8" text-anchor="middle" font-family="serif">'
               '<text x="600" y="200" font-size="42">GenTalent</text>'
               f'<text x="600" y="340" font-size="54">{name}</text>'
               '<text x="600" y="440" font-size="30">Certified Generational Talent</text>'
               '<text x="600" y="510" font-size="22">Unanimous final verdict</text></g></svg>')
        (directory / f"award-{row['id']}.svg").write_text(svg)
    print("Exported public preview and certified award card, if applicable, to", directory)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, APIError, OSError) as error:
        print("GenTalent:", str(error), file=sys.stderr)
        raise SystemExit(1)
