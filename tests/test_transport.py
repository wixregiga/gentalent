from datetime import datetime, timezone, timedelta
import io
from contextlib import redirect_stdout
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import tempfile
import threading
import unittest
from urllib.parse import unquote, urlsplit
from gentalent.config import Config
from gentalent.net import Beeper, APIError, secret_write
from gentalent.runner import arm, poll_once, flush, new_messages, lock_database
from gentalent.state import Store
from support import make_config, message


class FakeBeeper:
    def __init__(self):
        self.pages = [{"items": [], "newestCursor": "start", "hasMore": False}]
        self.sent = []
        self.ambiguous = False
        self.calls = []

    def verify_chat(self):
        return {}

    def messages(self, cursor=None):
        self.calls.append(cursor)
        return self.pages.pop(0)

    def send(self, text, attachment_path=None):
        self.sent.append(text)
        if self.ambiguous:
            raise TimeoutError()
        return "pending-1"

    def retrieve(self, pending):
        return {"id": "final-1", "sendStatus": {"status": "SUCCESS"}}


class TransportTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = make_config(self.temp.name)
        self.store = Store(self.config.path("database"), self.config)

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def test_arm_skips_old_messages_and_persists_new_cursor(self):
        fake = FakeBeeper()
        fake.pages[0]["items"] = [message("!gt Aster. Old pitch.", mid="old")]
        arm(self.store, fake)
        self.assertIsNone(self.store.current())
        new = message("!gt Aster. New pitch.", mid="new")
        fake.pages = [{"items": [new], "newestCursor": "next", "hasMore": False}]
        with redirect_stdout(io.StringIO()):
            poll_once(self.store, fake, live=False)
        self.assertEqual(self.store.current()["phase"], "voting")
        self.assertEqual(self.store.get("cursor"), "next")
        self.assertEqual(fake.sent, [])
        with self.assertRaises(ValueError):
            arm(self.store, fake)

    def test_multiple_pages_are_consumed_after_opaque_cursor(self):
        fake = FakeBeeper()
        fake.pages = [
            {"items": [message("!gtstatus", mid="1")], "newestCursor": "opaque-a", "hasMore": True},
            {"items": [message("!gtstatus", mid="2")], "newestCursor": "opaque-b", "hasMore": False}]
        cutoff = (datetime.now(timezone.utc) - timedelta(seconds=1)).isoformat()
        pages = list(new_messages(fake, "start", cutoff))
        self.assertEqual([p[1] for p in pages], ["opaque-a", "opaque-b"])
        self.assertEqual(fake.calls, ["start", "opaque-a"])

    def test_ambiguous_send_is_not_repeated_after_restart(self):
        self.store.handle(message("!gt Aster. Pitch."))
        fake = FakeBeeper()
        fake.ambiguous = True
        with self.assertRaises(APIError):
            flush(self.store, fake, live=True)
        self.store.close()
        self.store = Store(self.config.path("database"), self.config)
        flush(self.store, fake, live=True)
        self.assertEqual(len(fake.sent), 1)
        self.assertEqual(self.store.db.execute("SELECT status FROM outbox").fetchone()[0], "uncertain")

    def test_pending_send_resolves_without_resending(self):
        self.store.handle(message("!gt Aster. Pitch."))
        fake = FakeBeeper()
        with redirect_stdout(io.StringIO()):
            flush(self.store, fake, live=True)
            self.assertEqual(self.store.db.execute("SELECT status FROM outbox").fetchone()[0], "pending")
            self.store.close()
            self.store = Store(self.config.path("database"), self.config)
            flush(self.store, fake, live=True)
        self.assertEqual(len(fake.sent), 1)
        self.assertEqual(self.store.db.execute("SELECT status FROM outbox").fetchone()[0], "sent")

    def test_single_worker_lock_and_scope_binding(self):
        with lock_database(self.config.path("database")):
            with self.assertRaises(ValueError):
                with lock_database(self.config.path("database")):
                    pass
        data = dict(self.config.data, chat_id="another-chat")
        with self.assertRaises(ValueError):
            Store(self.config.path("database"), Config(data, self.config.root))

    def test_http_protocol_exact_scope_and_no_cross_chat_send(self):
        state = {"foreign": False, "posts": [], "uploads": []}
        expected = set(self.config.roster)
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def reply(self, body):
                encoded = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)
            def do_GET(self):
                if self.headers.get("Authorization") != "Bearer test-local-token":
                    self.send_error(401)
                    return
                path = unquote(urlsplit(self.path).path)
                if path == "/v1/chats/demo-group":
                    self.reply({"id": "wrong-group" if state["foreign"] else "demo-group",
                        "accountID": "demo-google", "type": "group",
                        "participants": {"hasMore": False, "items": [{"id": x} for x in expected]}})
                elif path == "/v1/chats/demo-group/messages":
                    self.reply({"items": [], "newestCursor": "opaque", "hasMore": False})
                else:
                    self.send_error(404)
            def do_POST(self):
                self.assert_path = unquote(urlsplit(self.path).path)
                raw = self.rfile.read(int(self.headers["Content-Length"]))
                if self.assert_path == "/v1/assets/upload":
                    state["uploads"].append((self.headers.get("Content-Type", ""), raw))
                    self.reply({"uploadID": "up-123", "fileName": "strip.jpg"})
                    return
                if self.assert_path != "/v1/chats/demo-group/messages":
                    self.send_error(404)
                    return
                state["posts"].append(json.loads(raw))
                self.reply({"chatID": "demo-group", "pendingMessageID": "p1"})
        with ThreadingHTTPServer(("127.0.0.1", 0), Handler) as server:
            worker = threading.Thread(target=server.serve_forever, daemon=True)
            worker.start()
            try:
                data = dict(self.config.data, beeper_url=f"http://127.0.0.1:{server.server_port}")
                config = Config(data, self.config.root)
                secret_write(config.path("token_file"), "test-local-token")
                api = Beeper(config)
                api.verify_chat()
                self.assertEqual(api.send("labeled test"), "p1")
                strip = Path(self.temp.name) / "strip.jpg"
                strip.write_bytes(b"\xff\xd8\xff\xe0fakejpeg")
                self.assertEqual(api.send("with photo", attachment_path=str(strip)), "p1")
                self.assertEqual(len(state["uploads"]), 1)
                self.assertIn("multipart/form-data", state["uploads"][0][0])
                self.assertIn(b"fakejpeg", state["uploads"][0][1])
                self.assertEqual(state["posts"][1]["attachment"]["uploadID"], "up-123")
                state["foreign"] = True
                with self.assertRaises(APIError):
                    api.send("must not be sent")
                self.assertEqual(len(state["posts"]), 2)
            finally:
                server.shutdown()
                worker.join()

    def test_configuration_rejects_remote_api_and_invalid_roster(self):
        for changes in ({"beeper_url": "https://example.com"},
                        {"beeper_url": "http://user:password@localhost:23373"},
                        {"members": self.config.data["members"][:2]}):
            with self.assertRaises(ValueError):
                Config(dict(self.config.data, **changes), self.config.root).validate()

    def test_stalled_cursor_fails_instead_of_silently_skipping_backlog(self):
        fake = FakeBeeper()
        fake.pages = [{"items": [], "newestCursor": "same", "hasMore": True}]
        cutoff = datetime.now(timezone.utc).isoformat()
        with self.assertRaises(APIError):
            list(new_messages(fake, "same", cutoff))

    def test_pending_send_without_send_status_resolves_when_final_id_assigned(self):
        self.store.handle(message("!gt Aster. Pitch."))
        fake = FakeBeeper()
        fake.retrieve = lambda pending: {"id": "264", "chatID": "demo-group", "accountID": "demo-google"}
        with redirect_stdout(io.StringIO()):
            flush(self.store, fake, live=True)
            self.assertEqual(self.store.db.execute("SELECT status FROM outbox").fetchone()[0], "pending")
            flush(self.store, fake, live=True)
        row = self.store.db.execute("SELECT status, final_id FROM outbox").fetchone()
        self.assertEqual((row["status"], row["final_id"]), ("sent", "264"))

    def test_expired_pending_id_404_marks_uncertain_without_guessing_or_resending(self):
        with self.store.db:
            stale = self.store.queue("stale reply")
            self.store.db.execute("UPDATE outbox SET status='pending', pending_id='~stale' WHERE id=?", (stale,))
            newer = self.store.queue("newer reply")
        fake = FakeBeeper()

        def expired(pending):
            raise APIError("API returned HTTP 404; no automatic write retry was made.", status_code=404)

        fake.retrieve = expired
        with redirect_stdout(io.StringIO()):
            flush(self.store, fake, live=True)
        rows = {r["id"]: (r["status"], r["final_id"]) for r in self.store.db.execute("SELECT * FROM outbox")}
        self.assertEqual(rows[stale], ("uncertain", None))
        self.assertEqual(rows[newer], ("pending", None))
        self.assertEqual(fake.sent, ["@gentalent-bot says: newer reply"])
        self.assertEqual(fake.calls, [], "a 404 must not trigger a history scan")

    def test_non_404_retrieve_error_propagates_and_leaves_row_pending(self):
        with self.store.db:
            key = self.store.queue("reply")
            self.store.db.execute("UPDATE outbox SET status='pending', pending_id='~p' WHERE id=?", (key,))
        fake = FakeBeeper()

        def server_error(pending):
            raise APIError("API returned HTTP 500; no automatic write retry was made.", status_code=500)

        fake.retrieve = server_error
        with self.assertRaises(APIError):
            flush(self.store, fake, live=True)
        self.assertEqual(self.store.db.execute("SELECT status FROM outbox").fetchone()[0], "pending")
