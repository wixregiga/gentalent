import sqlite3
import tempfile
import unittest
from gentalent.config import Config
from gentalent.net import APIError
from gentalent.nominees import leading_reference
from gentalent.runner import poll_once, with_reply_context
from gentalent.shame import _tagged_member, gif_dir, pick_gif
from gentalent.state import Store
from support import make_config

MEMBERS = [
    {"id": "avery", "name": "Avery", "platform": "android"},
    {"id": "dee", "name": "Dee Diaz", "platform": "unknown"},
    {"id": "nate", "name": "Nate Lane", "platform": "unknown"},
]


class ShameTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        base = make_config(self.temp.name)
        self.config = Config(dict(base.data, owner_id="avery", members=MEMBERS), base.root)
        self.store = Store(self.config.path("database"), self.config)
        self.n = 0

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def say(self, sender, text, **extra):
        self.n += 1
        message = {"id": extra.pop("mid", f"m{self.n}"), "chatID": "demo-group", "accountID": "demo-google",
                   "senderID": sender, "text": text, "timestamp": f"2026-11-17T00:00:{self.n:02d}Z", **extra}
        before = self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
        self.store.handle(message)
        rows = self.store.db.execute("SELECT text, attachment_path FROM outbox ORDER BY rowid LIMIT -1 OFFSET ?",
                                     (before,)).fetchall()
        return rows[-1] if rows else None, message

    def bank(self, *names):
        folder = gif_dir(self.config)
        folder.mkdir(parents=True, exist_ok=True)
        for name in names:
            (folder / name).write_bytes(b"GIF89a")

    def shames(self):
        return [dict(r) for r in self.store.db.execute("SELECT * FROM shames ORDER BY id")]

    def test_reply_shame_is_a_permanent_snapshot_of_that_moment(self):
        self.bank("cersei.gif")
        self.say("avery", "!gt Jordana Brewster. Fast and Furious icon.")
        self.say("dee", "!!n")
        _, said = self.say("dee", "nah she's *mid*")
        reply, _ = self.say("avery", "!gtshame hater", linkedMessageID=said["id"], linked=said)

        self.assertIn("🔔 SHAME 🔔", reply["text"])
        self.assertIn("@Avery shames @Dee Diaz over Jordana Brewster.", reply["text"])
        self.assertIn('They said: "nah she\'s \\*mid\\*"', reply["text"])  # escaped: shows as typed
        self.assertIn("Their vote right now: NO", reply["text"])
        self.assertTrue(reply["attachment_path"].endswith("cersei.gif"))
        [record] = self.shames()
        self.assertEqual((record["target_id"], record["reason"], record["target_vote"],
                          record["shamed_message_id"], record["shamed_text"]),
                         ("dee", "hater", "no", said["id"], "nah she's *mid*"))

        # He changes his mind and the vote finishes: the shame stays exactly as it was.
        self.say("dee", "!!y")
        self.say("nate", "!!y")
        self.say("avery", "!final")
        self.assertEqual(self.shames(), [record])
        for sql in ("DELETE FROM shames", "UPDATE shames SET reason='x'"):
            with self.assertRaises(sqlite3.IntegrityError):
                with self.store.db:
                    self.store.db.execute(sql)

        # One shame given per nominee, even at someone else; others can still shame afterwards,
        # and with no nominee named it means the latest one, finished or not.
        reply, _ = self.say("avery", "!gtshame @Nate Lane also bad")
        self.assertIn("You already used your shame for Jordana Brewster.", reply["text"])
        reply, _ = self.say("nate", "!gtshame @Deedee lol")  # "Deedee" is not a roster name...
        self.assertIn("Reply to their message", reply["text"])
        self.say("nate", "!gtshame @Deedee lol", mentions=["dee"])  # ...but Beeper tagged him.
        self.assertEqual([(s["shamer_id"], s["target_id"], s["reason"], s["target_vote"])
                          for s in self.shames()[1:]], [("nate", "dee", "lol", "yes")])

    def test_self_shame_and_replies_to_the_bot_spend_nothing(self):
        self.bank("cersei.gif")
        self.say("avery", "!gt Jordana Brewster. Icon.")
        _, own = self.say("avery", "she is great")
        reply, _ = self.say("avery", "!gtshame", linkedMessageID=own["id"], linked=own)
        self.assertIn("You can't shame yourself.", reply["text"])
        reply, _ = self.say("avery", "!gtshame @avery lol")
        self.assertIn("You can't shame yourself.", reply["text"])

        bot = {"id": "bot-1", "senderID": "avery", "text": "<p>@gentalent-bot says: @Dee Diaz has voted</p>"}
        self.assertIsNone(self.say("nate", "!gtshame", linkedMessageID="bot-1", linked=bot)[0])
        self.assertEqual(self.shames(), [])
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM outbox WHERE attachment_path<>''").fetchone()[0], 0)

    def test_nominee_references_people_write(self):
        self.say("avery", "!gt Adriana Lima. Icon.")
        self.store.db.execute("UPDATE cases SET phase='certified'")
        self.say("avery", "!gt Sandra bullock 👀. Icon.")
        for text, name, rest in (
            ("Adriana Lima. she wins", "Adriana Lima", "she wins"),
            ("adrianalima she wins", "Adriana Lima", "she wins"),
            ("ADRIANA LIMA", "Adriana Lima", ""),
            ("Sandra Bullock lol", "Sandra Bullock 👀", "lol"),
        ):
            ref = leading_reference(self.store, text)
            self.assertEqual((ref.case["name"], ref.rest), (name, rest), text)
        typo = leading_reference(self.store, "Adrina Lima. she wins")
        self.assertEqual((typo.case, typo.suggestion), (None, "Adriana Lima"))
        for first_name in ("Sandra he voted yes", "Sandra's fault"):
            self.assertEqual(leading_reference(self.store, first_name).suggestion, "Sandra Bullock 👀")
        plain = leading_reference(self.store, "he's a hater")
        self.assertEqual((plain.case, plain.rest, plain.suggestion), (None, "he's a hater", ""))
        reply, _ = self.say("nate", "!gtshame @Avery Adrina Lima. lol")
        self.assertIn("Did you mean Adriana Lima?", reply["text"])
        self.assertEqual(self.shames(), [])

    def test_tagged_names(self):
        for body, mentions, expected in (
            ("@Dee Diaz jordana. x", [], ("dee", "jordana. x")),
            ("@dee diaz", [], ("dee", "")),
            ("@Nate lol", [], ("nate", "lol")),
            ("@Nate Lane lol", [], ("nate", "lol")),
            ("@Nateson lol", [], (None, "@Nateson lol")),
            ("no tag here", ["dee"], (None, "no tag here")),
        ):
            self.assertEqual(_tagged_member(self.config, {"mentions": mentions}, body), expected, body)

    def test_reply_context_fetch(self):
        class Api:
            calls = []
            status = None

            def retrieve(self, message_id):
                self.calls.append(message_id)
                if self.status:
                    raise APIError("failed", status_code=self.status)
                return {"id": message_id, "senderID": "dee", "text": "hi"}

        api = Api()
        plain_reply = {"id": "r1", "linkedMessageID": "x", "text": "lol same"}
        self.assertIs(with_reply_context(api, plain_reply), plain_reply)
        self.assertEqual(api.calls, [], "only reply-shames fetch the replied-to message")
        shame_reply = {"id": "r2", "linkedMessageID": "x", "text": "!gtshame hater"}
        self.assertEqual(with_reply_context(api, shame_reply)["linked"]["senderID"], "dee")
        for status in (404, 500):
            api.status = status
            self.assertIsNone(with_reply_context(api, shame_reply)["linked"])

        self.say("avery", "!gt Jordana Brewster. Icon.")
        reply, _ = self.say("nate", "!gtshame hater", linkedMessageID="gone", linked=None)
        self.assertIn("I couldn't load the message you replied to.", reply["text"])

    def test_a_failing_reply_fetch_never_stalls_the_poll(self):
        class Api:
            def verify_chat(self):
                return {}

            def messages(self, cursor=None):
                stamp = "2999-01-01T00:00:0{}Z"
                return {"newestCursor": "c1", "hasMore": False, "items": [
                    {"id": "p1", "chatID": "demo-group", "accountID": "demo-google", "senderID": "nate",
                     "text": "!gtshame hater", "linkedMessageID": "x", "timestamp": stamp.format(1)},
                    {"id": "p2", "chatID": "demo-group", "accountID": "demo-google", "senderID": "nate",
                     "text": "!gthelp", "timestamp": stamp.format(2)}]}

            def retrieve(self, message_id):
                raise APIError("API returned HTTP 500", status_code=500)

        self.say("avery", "!gt Jordana Brewster. Icon.")
        with self.store.db:
            self.store.set("armed", "2026-01-01T00:00:00+00:00")
        poll_once(self.store, Api(), live=False)
        self.assertEqual(self.store.get("cursor"), "c1")
        texts = [r[0] for r in self.store.db.execute("SELECT text FROM outbox ORDER BY rowid")][-2:]
        self.assertIn("I couldn't load the message you replied to.", texts[0])
        self.assertIn("How GenTalent works", texts[1])

    def test_bot_messages_are_recognized_without_a_recorded_id(self):
        self.say("avery", "!gt Jordana Brewster. Icon.")
        with self.store.db:
            self.store.queue_photo("/media/jordana_brewster/photos/01_jordana_brewster.jpg", 1)
        html = {"id": "u1", "senderID": "avery", "text": "<p>@gentalent-bot says: hi<br>there</p>"}
        photo = {"id": "u2", "senderID": "avery", "attachments": [{"fileName": "01_jordana_brewster.jpg"}]}
        own_gif = {"id": "u3", "senderID": "avery", "attachments": [{"fileName": "38381e86.gif"}]}
        friend = dict(photo, id="u4", senderID="dee")
        self.assertEqual([self.store.is_own_message(m) for m in (html, photo, own_gif, friend)],
                         [True, True, False, False])

    def test_reply_that_tags_someone_else_is_refused(self):
        self.say("avery", "!gt Jordana Brewster. Icon.")
        _, said = self.say("dee", "she's mid")
        reply, _ = self.say("avery", "!gtshame @Nate hater", linkedMessageID=said["id"], linked=said)
        self.assertIn("You replied to one person but tagged another.", reply["text"])
        self.say("avery", "!gtshame @Deedee hater", linkedMessageID=said["id"], linked=said, mentions=["dee"])
        self.assertEqual([(s["target_id"], s["reason"]) for s in self.shames()], [("dee", "hater")])

    def test_gif_rotation_and_empty_bank(self):
        self.assertIsNone(pick_gif(self.store))
        self.say("avery", "!gt Jordana Brewster. Icon.")
        reply, _ = self.say("nate", "!gtshame @Avery lol")
        self.assertIn("(No shame GIFs in the bank yet.)", reply["text"])
        self.bank("a.gif", "b.gif")
        with self.store.db:
            self.store.db.execute(
                "INSERT INTO shames(case_id, shamer_id, target_id, gif, message_id, created_at) "
                "VALUES(1, 'dee', 'nate', 'a.gif', 'x', 't')")
        self.assertEqual(pick_gif(self.store).name, "b.gif")


class FeedbackTest(unittest.TestCase):
    def test_logged_confirmed_and_never_duplicated(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        config = make_config(temp.name)
        store = Store(config.path("database"), config)
        self.addCleanup(store.close)

        def say(text, mid):
            store.handle({"id": mid, "chatID": "demo-group", "accountID": "demo-google", "senderID": "bob",
                          "text": text, "timestamp": "2026-11-17T00:00:00Z"})
            return store.db.execute("SELECT text FROM outbox ORDER BY rowid DESC LIMIT 1").fetchone()[0]

        self.assertIn("Thanks @Bob, your feedback is logged", say("!gtfeedback show who voted last", "f1"))
        self.assertIn("your suggestion is logged", say("!GTSUGGEST a shame leaderboard", "f2"))
        self.assertIn("Add your feedback after the command", say("!gtfeedback", "f3"))
        say("!gtfeedback show who voted last", "f1")  # the same message again
        rows = store.db.execute("SELECT kind, text, sender_id FROM feedback ORDER BY id").fetchall()
        self.assertEqual([tuple(r) for r in rows],
                         [("feedback", "show who voted last", "bob"), ("suggestion", "a shame leaderboard", "bob")])


if __name__ == "__main__":
    unittest.main()
