import argparse
import getpass
import json
import os
from pathlib import Path
import sqlite3
import sys
from urllib.parse import quote
from .config import Config
from .net import Beeper, APIError, secret_write
from .feedback import entries as feedback_entries
from contextlib import nullcontext
from .runner import (arm, contract_report, lock_database, lock_live_chat, open_unlocked, resolve_send, run,
                     status_report)
from .state import Store

ROOT = Path(__file__).resolve().parent.parent


def make_parser():
    p = argparse.ArgumentParser(description="GenTalent: one verified group, three final votes.")
    p.add_argument("--config", default=str(ROOT / "config.json"))
    sub = p.add_subparsers(dest="command", required=True)
    sub.add_parser("init", help="Create a local config from the example; never overwrite.")
    sub.add_parser("set-secret", help="Save a Beeper token through a hidden prompt.")
    d = sub.add_parser("doctor", help="Check local API availability; group reads require --chat.")
    d.add_argument("--chat", action="store_true")
    d.add_argument("--contract", action="store_true",
                   help="Also check the Beeper behavior GenTalent relies on, using recent live replies (read-only).")
    sub.add_parser("accounts", help="List connected Beeper accounts and connection status.")
    lg = sub.add_parser("list-groups", help="List recent group chats (metadata only, no message bodies).")
    lg.add_argument("--account")
    lg.add_argument("--limit", type=int, default=50)
    f = sub.add_parser("find-chat", help="Search group metadata using your exact title words.")
    f.add_argument("query")
    f.add_argument("--account")
    i = sub.add_parser("inspect-chat", help="Inspect metadata for one explicitly selected chat ID.")
    i.add_argument("chat_id")
    a = sub.add_parser("arm", help="Start a ballot database at the newest message; old commands are skipped.")
    a.add_argument("--if-unarmed", action="store_true", help="Do nothing if this database is already armed.")
    r = sub.add_parser("run", help="Poll the group and reply. Without --live, replies are previews only.")
    r.add_argument("--once", action="store_true")
    sub.add_parser("status", help="Show cases and replies needing attention (read-only; safe while running).")
    sub.add_parser("feedback", help="List !gtfeedback and !gtsuggest entries (read-only; safe while running).")
    rs = sub.add_parser("resolve-send", help="Record a checked outcome for an uncertain or failed reply.")
    rs.add_argument("outbox_id")
    outcome = rs.add_mutually_exclusive_group(required=True)
    outcome.add_argument("--message-id", help="Beeper message ID that delivered this reply; verified against Beeper.")
    outcome.add_argument("--not-delivered", action="store_true", help="Record that the reply never arrived. It is not resent.")
    for s in (a, r, rs, sub.choices["status"], sub.choices["feedback"]):
        s.add_argument("--live", action="store_true", help="Use the real ballot. run --live sends replies.")
    gm = sub.add_parser("generate-media", help="Run the subscription-only multi-model photo strip builder.")
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


def discovery(args, api):
    if args.command == "accounts":
        return [account_summary(a) for a in api.request("GET", "/v1/accounts")]
    if args.command == "list-groups":
        query = {"limit": max(1, min(args.limit, 200))}
        if args.account:
            query["accountIDs"] = [args.account]
        result = api.request("GET", "/v1/chats", query=query)
        return {"items": [metadata(c) for c in result.get("items", []) if c.get("type") == "group"],
                "has_more": result.get("hasMore", False)}
    if args.command == "find-chat":
        query = {"query": args.query, "type": "group", "limit": 20}
        if args.account:
            query["accountIDs"] = [args.account]
        result = api.request("GET", "/v1/chats/search", query=query)
        return {"items": [metadata(c) for c in result.get("items", [])],
                "has_more": result.get("hasMore", False),
                "note": "Confirm one underlying Google Messages chat; do not select by title alone."}
    result = api.request("GET", "/v1/chats/" + quote(args.chat_id, safe=""), query={"maxParticipantCount": -1})
    return metadata(result)


def database_path(config, live):
    path = config.path("database")
    return path if live else path.with_name("dryrun-" + path.name)


def main(argv=None):
    os.umask(0o077)
    args = make_parser().parse_args(argv)
    if args.command == "init":
        path = Path(args.config).expanduser().resolve()
        if path.exists():
            raise ValueError("Config already exists; no overwrite.")
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text((ROOT / "config.example.json").read_text())
        print("Created", path, "- fill in the confirmed group and roster IDs.")
        return 0
    config = Config.load(args.config)
    if args.command == "set-secret":
        token = getpass.getpass("Beeper access token (hidden; do not paste into chat): ").strip()
        if not token:
            raise ValueError("Token was empty.")
        secret_write(config.path("token_file"), token)
        print("Token saved privately.")
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
        print(json.dumps(discovery(args, Beeper(config, discovery=True)), indent=2, ensure_ascii=False))
        return 0
    if args.command == "doctor":
        api = Beeper(config, discovery=not (args.chat or args.contract))
        api.request("GET", "/v1/info")
        api.request("GET", "/v1/accounts")
        print("Local Beeper API is reachable and token is accepted.")
        if args.chat or args.contract:
            api.verify_chat()
            print("Exact group, account, and three-person roster match.")
        if args.contract:
            db = open_unlocked(database_path(config, live=True), read_only=True)
            try:
                failures = contract_report(api, db, api.request("GET", "/v1/spec"))
            finally:
                db.close()
            if failures:
                raise ValueError("Beeper contract check failed: " + " | ".join(failures))
            print("Beeper behavior matches doc/beeper-api.md.")
        return 0
    config.validate()
    path = database_path(config, args.live)
    if args.command == "status":
        db = open_unlocked(path, read_only=True)
        try:
            print(json.dumps(status_report(db), indent=2, ensure_ascii=False))
        finally:
            db.close()
        return 0
    if args.command == "feedback":
        db = open_unlocked(path, read_only=True)
        try:
            print(json.dumps(feedback_entries(db), indent=2, ensure_ascii=False))
        finally:
            db.close()
        return 0
    if args.command == "resolve-send":
        db = open_unlocked(path)
        try:
            print(resolve_send(db, Beeper(config), args.outbox_id,
                               None if args.not_delivered else args.message_id))
        finally:
            db.close()
        return 0
    live_run = args.command == "run" and args.live
    with lock_live_chat(config) if live_run else nullcontext(), lock_database(path):
        store = Store(path, config)
        try:
            if args.command == "arm":
                if args.if_unarmed and store.get("armed"):
                    print("Already armed.")
                else:
                    print(arm(store, Beeper(config)))
            elif args.command == "run":
                run(store, Beeper(config), args.live, args.once)
        finally:
            store.close()
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, APIError, OSError) as error:
        print("GenTalent:", str(error), file=sys.stderr)
        raise SystemExit(1)
    except sqlite3.OperationalError as error:
        if "locked" not in str(error):
            raise
        print("GenTalent: the database is busy (the bot may be generating photos). Try again in a minute.",
              file=sys.stderr)
        raise SystemExit(1)
