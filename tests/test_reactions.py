import tempfile
import unittest
from gentalent.config import Config
from gentalent.net import APIError
from gentalent.reactions import sync, vote_for
from gentalent.state import Store
from support import make_config


def reaction(member, key):
    # The shape Beeper returned for a real 👍 on message #686 (Google Messages, one per person).
    return {"id": member, "reactionKey": key, "participantID": member, "emoji": True}


class ReactionVoteTest(unittest.TestCase):
    def setUp(self, announce=False):
        self.temp = tempfile.TemporaryDirectory()
        base = make_config(self.temp.name)
        self.config = Config(dict(base.data, announce_reaction_votes=announce), base.root)
        self.now = [1000.0]
        self.store = Store(self.config.path("database"), self.config, clock=lambda: self.now[0])
        self.n = 0
        self.say("alice", "!gt Jordana Brewster. Icon.")
        with self.store.db:  # the nomination announcement was delivered as message 770
            self.store.db.execute("UPDATE outbox SET status='sent', final_id='770'")
        self.message = {"id": "770", "reactions": []}
        self.reads = 0

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def retrieve(self, message_id):
        self.reads += 1
        self.assertEqual(message_id, "770")
        return self.message

    def say(self, sender, text):
        self.n += 1
        self.store.handle({"id": f"m{self.n}", "chatID": "demo-group", "accountID": "demo-google",
                           "senderID": sender, "text": text, "timestamp": "2026-11-17T00:00:00Z"})

    def react(self, *pairs):
        self.message["reactions"] = [reaction(m, k) for m, k in pairs]
        before = self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
        sync(self.store, self)
        return [r[0] for r in self.store.db.execute("SELECT text FROM outbox ORDER BY rowid LIMIT -1 OFFSET ?",
                                                    (before,))]

    def votes(self):
        return dict(self.store.db.execute("SELECT sender_id, vote FROM votes"))

    def test_any_skin_tone_counts(self):
        for key in ("👍", "👍️", "👍🏻", "👍🏼", "👍🏽", "👍🏾", "👍🏿"):
            self.assertEqual(vote_for(key), "yes", key)
        for key in ("👎", "👎🏿"):
            self.assertEqual(vote_for(key), "no", key)
        for key in ("😂", "😡", "thumbsup", ""):
            self.assertIsNone(vote_for(key), key)

    def test_silent_reaction_votes_follow_the_typed_vote_rules(self):
        self.assertEqual(self.react(("bob", "👍🏽")), [])
        self.assertEqual(self.votes()["bob"], "yes")
        self.react(("bob", "👎"))
        self.assertEqual(self.votes()["bob"], "no")
        self.react()  # removed
        self.assertNotIn("bob", self.votes())
        self.react(("bob", "😂"), ("outsider", "👍"))
        self.assertEqual(self.votes(), {"alice": "yes"})

        self.assertEqual(self.react(("bob", "👍"), ("carol", "👍")), [])
        self.assertIsNotNone(self.store.current()["all_votes_at"], "3/3 starts the timer, unannounced")
        self.assertEqual(self.react(("bob", "👍"), ("carol", "👎")), [], "locked: no change, no refusal reply")
        self.assertEqual(self.votes()["carol"], "yes")
        self.now[0] += 60
        self.store.finalize_due()
        self.assertEqual(self.store.current()["phase"], "certified")
        verdict = self.store.db.execute("SELECT text FROM outbox ORDER BY rowid DESC LIMIT 1").fetchone()[0]
        self.assertIn("Congratulations Jordana Brewster!", verdict)

    def test_removing_a_reaction_keeps_a_vote_typed_since(self):
        self.react(("bob", "👍"))
        self.say("bob", "!!N")
        self.react()
        self.assertEqual(self.votes()["bob"], "no")

    def test_reactions_count_while_the_bot_was_away_and_are_not_reapplied(self):
        self.message["reactions"] = [reaction("bob", "👍")]
        sync(self.store, self)
        sync(self.store, self)
        self.assertEqual(self.votes()["bob"], "yes")
        self.say("bob", "!!N")
        sync(self.store, self)  # the same 👍 is not a new vote
        self.assertEqual(self.votes()["bob"], "no")

    def test_announced_when_configured(self):
        self.tearDown()
        self.setUp(announce=True)
        [reply] = self.react(("bob", "👍"))
        self.assertIn("@Bob has voted for Jordana Brewster to be a generational talent!", reply)

    def test_unreadable_or_undelivered_announcement_changes_nothing(self):
        def failing(message_id):
            raise APIError("API returned HTTP 500", status_code=500)

        self.retrieve = failing
        sync(self.store, self)
        self.assertEqual(self.votes(), {"alice": "yes"})
        del self.retrieve
        with self.store.db:
            self.store.db.execute("UPDATE outbox SET final_id=NULL, status='pending'")
        sync(self.store, self)
        self.assertEqual(self.reads, 0, "no read until the announcement has a delivered ID")


if __name__ == "__main__":
    unittest.main()
