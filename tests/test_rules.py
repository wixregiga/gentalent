import tempfile
import unittest
from gentalent.demo import demo_config, message, FakeTumblr
from gentalent.state import Store
from gentalent.tumblr import publish, reconcile
from gentalent.net import APIError


class RulesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = demo_config(self.temp.name)
        self.store = Store(self.config.path("database"), self.config)
        self.store.register("demo:aster", "Aster Example", ["Aster"])
        self.store.register("demo:birch", "Birch Example", ["Birch"])

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def command(self, text, sender="alice", mid=None):
        return self.store.handle(message(text, sender, mid))

    def nominate(self):
        self.command("!nominate Aster")
        self.command("!openvote")

    def test_unanimous_final_votes_and_nominator_auto_yes(self):
        self.nominate()
        self.command("!vote no")
        self.command("!vote yes", "bob")
        self.assertEqual(self.store.current()["phase"], "voting")
        self.command("!vote yes", "carol")
        self.assertEqual(self.store.current()["phase"], "certified")
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM votes").fetchone()[0], 3)

    def test_denial_waits_for_all_three_and_survives_alias_and_restart(self):
        self.nominate()
        self.command("!vote no", "bob")
        self.assertEqual(self.store.current()["phase"], "voting")
        self.command("!vote yes", "carol")
        self.store.close()
        self.store = Store(self.config.path("database"), self.config)
        self.command("!nominate ＡＳＴＥＲ EXAMPLE", mid="renominate")
        self.assertEqual(self.store.current()["phase"], "denied")
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM cases").fetchone()[0], 1)
        self.assertIn("can never", self.store.db.execute("SELECT text FROM outbox ORDER BY rowid DESC").fetchone()[0])

    def test_one_active_case_and_registered_identity(self):
        self.command("!nominate An unregistered person")
        self.assertIsNone(self.store.current())
        self.command("!nominate Aster")
        self.command("!nominate Birch", "bob")
        self.assertEqual(self.store.current()["nominee_id"], "demo:aster")
        with self.assertRaises(ValueError):
            self.store.register("demo:duplicate", "Different", ["Aster"])

    def test_scope_outsiders_and_edits_are_ignored(self):
        for changes in ({"senderID": "outsider"}, {"chatID": "another-chat"},
                        {"accountID": "another-account"}, {"editedTimestamp": "2026-01-01"},
                        {"isDeleted": True}, {"isHidden": True}):
            m = message("!nominate Aster")
            m.update(changes)
            self.assertFalse(self.store.handle(m))
        self.assertIsNone(self.store.current())
        self.assertTrue(self.command("!nominate Aster"))

    def test_replay_does_not_count_or_reply_twice(self):
        self.nominate()
        self.command("!vote yes", "bob", "unique-vote")
        before = self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
        self.store.close()
        self.store = Store(self.config.path("database"), self.config)
        self.assertFalse(self.command("!vote yes", "bob", "unique-vote"))
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0], before)

    def test_publication_approval_invalidated_by_verdict_and_edit(self):
        self.nominate()
        self.command("!public Exact approved public text.")
        self.command("!approve-public")
        client = FakeTumblr()
        url = publish(self.store, client)
        self.command("!vote yes", "bob")
        self.command("!vote yes", "carol")
        with self.assertRaises(ValueError):
            publish(self.store, client)
        self.command("!approve-public", mid="approve-final")
        self.assertEqual(publish(self.store, client), url)
        self.assertEqual(publish(self.store, client), url)
        self.assertEqual((client.creates, client.edits), (1, 1))
        self.assertEqual(len(client.posts), 1)
        self.assertNotIn("sender_id", str(self.store.payload()))

    def test_only_owner_can_approve_and_only_nominator_selects_exhibits(self):
        self.command("!nominate Aster")
        self.command("!exhibit Not mine", "bob")
        self.command("!public Not approved", "bob")
        self.command("!approve-public", "bob")
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM exhibits").fetchone()[0], 0)
        self.assertIsNone(self.store.current()["approved_hash"])

    def test_uncertain_publication_never_automatically_creates_twice(self):
        self.command("!nominate Aster")
        self.command("!public Exact public text.")
        self.command("!approve-public")
        client = FakeTumblr()
        original = client.write
        def ambiguous(payload, post_id=None):
            original(payload, post_id)
            raise TimeoutError()
        client.write = ambiguous
        with self.assertRaises(APIError):
            publish(self.store, client)
        self.assertEqual(self.store.current()["publish_state"], "uncertain")
        with self.assertRaises(ValueError):
            publish(self.store, client)
        self.assertEqual(client.creates, 1)
        reconcile(self.store, client, "1001")
        self.assertEqual(self.store.current()["publish_state"], "published")

    def test_imported_prior_denial_is_permanent_and_cannot_conflict_with_active_case(self):
        import sqlite3
        self.store.import_denial("demo:birch")
        self.command("!nominate Birch")
        self.assertIsNone(self.store.current())
        with self.assertRaises(sqlite3.IntegrityError):
            with self.store.db:
                self.store.db.execute("DELETE FROM denials")
        self.command("!nominate Aster")
        with self.assertRaises(ValueError):
            self.store.import_denial("demo:aster")
