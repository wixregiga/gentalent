from contextlib import redirect_stdout
import io
from pathlib import Path
import sqlite3
import tempfile
import unittest
from gentalent.runner import contract_report, flush, lock_database, open_unlocked, resolve_send, status_report
from gentalent.state import Store
from support import make_config


class FakeRetrieve:
    def __init__(self, messages):
        self.messages = messages
        self.sent = []

    def retrieve(self, message_id):
        return self.messages[message_id]

    def send(self, text, attachment_path=None):
        self.sent.append(text)
        return "never"


class ResolveSendTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = make_config(self.temp.name)
        self.path = self.config.path("database")
        self.strip = Path(self.temp.name) / "strip.jpg"
        self.strip.write_bytes(b"x" * 1234)
        store = Store(self.path, self.config)
        with store.db:
            self.text_id = store.queue("help text")
            self.photo_id = store.queue("nomination", attachment_path=str(self.strip))
            self.sent_id = store.queue("already fine")
            store.db.execute("UPDATE outbox SET status='uncertain' WHERE id IN (?,?)", (self.text_id, self.photo_id))
            store.db.execute("UPDATE outbox SET status='sent', final_id='900' WHERE id=?", (self.sent_id,))
        store.close()
        self.db = open_unlocked(self.path)
        self.api = FakeRetrieve({
            "264": {"id": "264", "isSender": True, "text": "@gentalent-bot says: help text"},
            "265": {"id": "265", "isSender": True, "text": "@gentalent-bot says: something else"},
            "266": {"id": "266", "isSender": False, "text": "@gentalent-bot says: help text"},
            "267": {"id": "267", "isSender": True, "type": "IMAGE",
                    "attachments": [{"fileName": "strip.jpg", "fileSize": 1234}]},
            "268": {"id": "268", "isSender": True, "type": "IMAGE",
                    "attachments": [{"fileName": "other.jpg", "fileSize": 1234}]},
            "269": {"id": "269", "isSender": True, "type": "IMAGE",
                    "attachments": [{"fileName": "strip.jpg", "fileSize": 999}]},
        })

    def tearDown(self):
        self.db.close()
        self.temp.cleanup()

    def row(self, key):
        return tuple(self.db.execute("SELECT status, final_id FROM outbox WHERE id=?", (key,)).fetchone())

    def test_text_reply_needs_matching_own_message(self):
        for bad in ("265", "266"):
            with self.assertRaises(ValueError):
                resolve_send(self.db, self.api, self.text_id, bad)
        self.assertEqual(self.row(self.text_id), ("uncertain", None))
        resolve_send(self.db, self.api, self.text_id, "264")
        self.assertEqual(self.row(self.text_id), ("sent", "264"))

    def test_photo_reply_needs_same_file_name_and_byte_size(self):
        for bad in ("268", "269"):
            with self.assertRaises(ValueError):
                resolve_send(self.db, self.api, self.photo_id, bad)
        resolve_send(self.db, self.api, self.photo_id, "267")
        self.assertEqual(self.row(self.photo_id), ("sent", "267"))

    def test_message_already_recorded_for_another_reply_is_refused(self):
        self.api.messages["900"] = {"id": "900", "isSender": True, "text": "@gentalent-bot says: help text"}
        with self.assertRaises(ValueError):
            resolve_send(self.db, self.api, self.text_id, "900")

    def test_only_uncertain_or_failed_rows_can_be_resolved(self):
        with self.assertRaises(ValueError):
            resolve_send(self.db, self.api, self.sent_id, "264")
        with self.assertRaises(ValueError):
            resolve_send(self.db, self.api, "no-such-row", "264")

    def test_not_delivered_is_recorded_and_never_resent(self):
        resolve_send(self.db, self.api, self.text_id)
        self.assertEqual(self.row(self.text_id), ("failed", None))
        store = Store(self.path, self.config)
        try:
            with redirect_stdout(io.StringIO()):
                flush(store, self.api, live=True)
        finally:
            store.close()
        self.assertEqual(self.api.sent, [])

    def test_status_report_lists_only_unresolved_rows(self):
        resolve_send(self.db, self.api, self.photo_id)
        with self.db:
            self.db.execute("UPDATE outbox SET status='superseded' WHERE id=?", (self.sent_id,))
        ro = open_unlocked(self.path, read_only=True)
        report = status_report(ro)
        ro.close()
        self.assertEqual([r["id"] for r in report["outbox_needing_attention"]], [self.text_id])
        self.assertEqual(report["outbox_counts"], {"uncertain": 1, "failed": 1, "superseded": 1})

    def test_failed_reply_can_be_resolved_to_its_own_recorded_message(self):
        with self.db:
            self.db.execute("UPDATE outbox SET status='failed', final_id='264' WHERE id=?", (self.text_id,))
        resolve_send(self.db, self.api, self.text_id, "264")
        self.assertEqual(self.row(self.text_id), ("sent", "264"))

    def test_resolve_works_while_the_worker_lock_is_held(self):
        with lock_database(self.path):
            resolve_send(self.db, self.api, self.text_id, "264")
        self.assertEqual(self.row(self.text_id), ("sent", "264"))

    def test_resolve_reports_busy_instead_of_waiting_forever(self):
        worker = sqlite3.connect(self.path, timeout=0)
        worker.execute("BEGIN IMMEDIATE")
        try:
            busy = open_unlocked(self.path, timeout=0.2)
            with self.assertRaises(sqlite3.OperationalError):
                resolve_send(busy, self.api, self.text_id, "264")
            busy.close()
        finally:
            worker.rollback()
            worker.close()
        self.assertEqual(self.row(self.text_id), ("uncertain", None))


SPEC = {
    "components": {"schemas": {
        "SendStatus": {"properties": {"status": {"enum": ["SUCCESS", "PENDING", "FAIL_RETRIABLE", "FAIL_PERMANENT"]}}},
        "Message": {"required": ["id", "chatID", "accountID", "senderID", "timestamp", "sortKey"]},
    }},
    "paths": {"/v1/chats/{chatID}/messages/{messageID}": {"get": {
        "description": "Retrieve a message by final message ID, pendingMessageID, or Matrix event ID."}}},
}


class ContractReportTest(unittest.TestCase):
    """The checker itself. The real oracle is `gentalent doctor --contract` against live Beeper."""

    def setUp(self):
        self.db = sqlite3.connect(":memory:")
        self.db.row_factory = sqlite3.Row
        self.db.execute("CREATE TABLE outbox(rowid_ INTEGER PRIMARY KEY, id TEXT, status TEXT, final_id TEXT, "
                        "text TEXT, attachment_path TEXT)")
        self.db.execute("INSERT INTO outbox(id,status,final_id,text,attachment_path) "
                        "VALUES('a','sent','739','@gentalent-bot says: hi','')")
        self.api = FakeRetrieve({"739": {"id": "739", "isSender": True, "text": "@gentalent-bot says: hi"}})

    def test_passes_on_documented_and_observed_behavior(self):
        self.assertEqual(contract_report(self.api, self.db, SPEC), [])

    def test_flags_each_kind_of_drift(self):
        import copy
        spec = copy.deepcopy(SPEC)
        spec["components"]["schemas"]["SendStatus"]["properties"]["status"]["enum"].append("QUEUED")
        spec["components"]["schemas"]["Message"]["required"].append("sendStatus")
        spec["paths"]["/v1/chats/{chatID}/messages/{messageID}"]["get"]["description"] = "By message ID."
        self.api.messages["739"] = {"id": "739", "isSender": False, "text": "edited",
                                    "sendStatus": {"status": "PENDING"}}
        failures = contract_report(self.api, self.db, spec)
        self.assertEqual(len(failures), 6, failures)


if __name__ == "__main__":
    unittest.main()
