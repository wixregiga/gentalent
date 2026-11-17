from pathlib import Path
import tempfile
import unittest
from gentalent.config import Config
from gentalent.generative import GeneratedMediaBundle
from gentalent.state import Store
from gentalent.workflow import ActionRequest, format_nominee_name, split_nominee_and_reason


def scenario_config(root):
    return Config(
        {
            "chat_id": "friends-group",
            "account_id": "gmessages-1",
            "owner_id": "me",
            "members": [
                {"id": "me", "name": "Me", "platform": "android"},
                {"id": "person1", "name": "Person1", "platform": "iphone"},
                {"id": "person2", "name": "Person2", "platform": "android"},
            ],
            "beeper_url": "http://localhost:23373",
            "database": "scenario.sqlite3",
            "poll_seconds": 1,
            "tumblr_blog": "example.tumblr.com",
            "token_file": "unused.token",
            "tumblr_credentials": "unused.json",
        },
        Path(root),
    )


class WorkflowTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = scenario_config(self.temp.name)
        self.now = [1000.0]
        self.ai_calls = []

        def fake_media(name, pitch):
            self.ai_calls.append((name, pitch))
            out_dir = Path(self.temp.name) / "media" / "fake"
            out_dir.mkdir(parents=True, exist_ok=True)
            strip = out_dir / "strip.jpg"
            strip.write_bytes(b"fake-strip-jpeg")
            sources = out_dir / "SOURCES.txt"
            sources.write_text("fake sources")
            return GeneratedMediaBundle(
                nominee_name=name,
                summary="Representative photos gathered.",
                artifact_dir=out_dir,
                photo_paths=[],
                strip_path=strip,
                sources_path=sources,
                provider_used="codex:gpt-6.1-sol",
            )

        self.store = Store(
            self.config.path("database"),
            self.config,
            media_generator=fake_media,
            clock=lambda: self.now[0],
        )
        self.seq = 0

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def send(self, sender, text, timestamp="2026-11-16T18:00:00Z", mid=None):
        self.seq += 1
        before = self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
        event = {
            "id": mid or f"msg-{self.seq}",
            "chatID": "friends-group",
            "accountID": "gmessages-1",
            "senderID": sender,
            "text": text,
            "timestamp": timestamp,
            "isSender": sender == "me",
        }
        handled = self.store.handle(event)
        after = self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
        reply = None
        if after > before:
            reply = self.store.db.execute(
                "SELECT text, attachment_path FROM outbox ORDER BY rowid DESC LIMIT 1"
            ).fetchone()
        return handled, (reply["text"] if reply else None), (reply["attachment_path"] if reply else "")

    def test_eva_longoria_scenario_and_final_command(self):
        handled, reply, attach = self.send(
            "me",
            "!gt Eva longoria. She made desperate housewives what it was. "
            "She's still got it today and she has no major scandals despite being in the spotlight for so long. "
            "She's a generational talent for sure.",
        )
        self.assertTrue(handled)
        self.assertEqual(attach, "")
        self.assertEqual(
            reply,
            "@gentalent-bot says: @Me would like to nominate Eva Longoria as a 2026 Generational Talent Nominee "
            "(3/3 YES certifies; any NO denies forever). Vote with !!Y, !!N, or !!C (!gthelp for rules). 1/3 votes are in.",
        )
        self.assertEqual(self.store.current()["phase"], "voting")
        self.assertEqual(self.store.current()["name"], "Eva Longoria")
        self.assertEqual(self.ai_calls, [])

        # Casual debate without keyword is ignored
        self.assertEqual(self.send("person1", "I don't know. I hated her acting in that show."), (False, None, ""))

        # Second member votes !!Yes
        handled, reply, _ = self.send("person2", "!!Yes")
        self.assertTrue(handled)
        self.assertEqual(
            reply,
            "@gentalent-bot says: @Person2 has voted for Eva Longoria to be a generational talent! 2/3 votes submitted.",
        )

        # More casual banter ignored
        self.assertEqual(self.send("me", "That is the worst take I've heard in years"), (False, None, ""))
        self.assertEqual(
            self.send("person1", "But I did like her in that other movie so my vote is yes."),
            (False, None, ""),
        )
        self.assertEqual(self.send("me", "You have to use the right keyword @person1"), (False, None, ""))

        # Third member votes !!YES -> 3/3 votes in, enters 1-minute finalization buffer
        handled, reply, _ = self.send("person1", "!!YES")
        self.assertTrue(handled)
        self.assertIn("Finalizing in 1 minute", reply)
        self.assertEqual(self.store.current()["phase"], "voting")

        # Non-nominator cannot finalize early
        handled, reply, _ = self.send("person2", "!final")
        self.assertTrue(handled)
        self.assertIn("Only @Me", reply)
        self.assertEqual(self.store.current()["phase"], "voting")

        # Initial nominator sends !final -> certified immediately
        handled, reply, _ = self.send("me", "!final")
        self.assertTrue(handled)
        self.assertEqual(
            reply,
            "@gentalent-bot says: Congratulations Eva Longoria! You are officially a Generational Talent!",
        )
        self.assertEqual(self.store.current()["phase"], "certified")

        # Voting or !final after finalization triggers ZERO messages
        for sender, txt in (("me", "!!Y"), ("person1", "!!N"), ("person2", "!!C"), ("me", "!final")):
            _, post_reply, _ = self.send(sender, txt)
            self.assertIsNone(post_reply)

        # Duplicate nomination of already-certified nominee is blocked without AI
        handled, reply, _ = self.send("person2", "!!gt Eva Longoria. Nominating her again.")
        self.assertTrue(handled)
        self.assertIn("already been nominated", reply)
        self.assertEqual(self.ai_calls, [])

    def test_vote_letters_changes_clearing_duplicates_and_nominator_changes(self):
        # Empty state: !!C / !!Y / !!N trigger no messages
        for txt in ("!!C", "!!clear", "!!Y", "!!N"):
            _, reply, _ = self.send("person1", txt)
            self.assertIsNone(reply)

        self.send("me", "!gt Samuel L. Jackson. Pulp Fiction and Snakes on a Plane.")

        # Clearing an empty vote does NOT trigger a message
        _, reply, _ = self.send("person1", "!!c")
        self.assertIsNone(reply)

        # Duplicate same vote triggers NO message (even for nominator's auto-yes)
        _, reply, _ = self.send("me", "!!Y")
        self.assertIsNone(reply)

        # Nominator CAN change their own vote before 3rd vote is cast
        _, reply, _ = self.send("me", "!!n")
        self.assertEqual(
            reply,
            "@gentalent-bot says: @Me changed their vote to NO on Samuel L. Jackson. 1/3 votes submitted.",
        )
        # Repeating !!N is silent
        _, reply, _ = self.send("me", "!!no")
        self.assertIsNone(reply)

        # Nominator can clear their own vote too -> drops to 0/3
        _, reply, _ = self.send("me", "!!CLEAR")
        self.assertEqual(
            reply,
            "@gentalent-bot says: @Me cleared their vote on Samuel L. Jackson. 0/3 votes submitted.",
        )
        # Clearing again while empty is silent
        _, reply, _ = self.send("me", "!!c")
        self.assertIsNone(reply)

        # Cast 3 votes with !!y / !!n letters
        _, reply, _ = self.send("person1", "!!n")
        self.assertIn("voted against Samuel L. Jackson", reply)
        self.assertIn("1/3", reply)
        _, reply, _ = self.send("person1", "!!y")
        self.assertIn("changed their vote to YES", reply)
        self.assertIn("1/3", reply)
        _, reply, _ = self.send("person1", "!!c")
        self.assertIn("cleared their vote", reply)
        self.assertIn("0/3", reply)

        self.send("me", "!!y")
        self.send("person1", "!!n")
        _, reply, _ = self.send("person2", "!!y")
        self.assertIn("3/3 votes submitted", reply)
        self.assertEqual(self.store.current()["phase"], "voting")

        # Votes are locked after 3rd vote cast: repeating same vote is silent, changing is blocked
        _, reply, _ = self.send("person2", "!!y")
        self.assertIsNone(reply)
        _, reply, _ = self.send("person1", "!!y")
        self.assertIn("Voting is locked", reply)

        # 59 seconds later: NOT finalized yet
        self.now[0] += 59.0
        self.assertFalse(self.store.finalize_due())
        self.assertEqual(self.store.current()["phase"], "voting")

        # 60 seconds total elapsed: finalized permanently as denied
        self.now[0] += 1.0
        self.assertTrue(self.store.finalize_due())
        self.assertEqual(self.store.current()["phase"], "denied")
        verdict = self.store.db.execute("SELECT text FROM outbox ORDER BY rowid DESC LIMIT 1").fetchone()[0]
        self.assertIn("permanently denied", verdict)

        # Post-finalization vote / !!gt cannot trigger AI or messages
        _, reply, _ = self.send("person1", "!!y")
        self.assertIsNone(reply)
        _, reply, _ = self.send("person2", "!!gt SAMUEL L JACKSON")
        self.assertIn("can never be nominated again", reply)
        self.assertEqual(self.ai_calls, [])

    def test_generative_gt_invokes_ai_once_and_blocks_repeat(self):
        handled, reply, attach = self.send(
            "me",
            "!!gt Welvin Harris. Viral Instagram comedian.",
        )
        self.assertTrue(handled)
        self.assertIn("Welvin Harris", reply)
        self.assertTrue(attach.endswith("strip.jpg"))
        self.assertEqual(len(self.ai_calls), 1)

        # Calling !!gt again for the active nominee does NOT invoke AI a second time
        handled, reply, attach2 = self.send("person1", "!!gt Welvin Harris. More photos please.")
        self.assertTrue(handled)
        self.assertIn("already created", reply)
        self.assertEqual(attach2, "")
        self.assertEqual(len(self.ai_calls), 1)

        # Calling !!gt for a different nominee while active does NOT invoke AI either
        handled, reply, _ = self.send("person2", "!!gt Eva Longoria. Desperate Housewives.")
        self.assertTrue(handled)
        self.assertIn("Finish voting on Welvin Harris first", reply)
        self.assertEqual(len(self.ai_calls), 1)

    def test_gt_can_upgrade_once_to_generative_and_ignores_unrelated_words(self):
        # !!yesterday / !!cat / !!normal must NOT trigger votes
        self.send("me", "!gt Welvin Harris. Viral icon.")
        self.assertEqual(self.send("person1", "!!yesterday"), (False, None, ""))
        self.assertEqual(self.send("person1", "!!cat"), (False, None, ""))
        self.assertEqual(self.send("person1", "!!normal"), (False, None, ""))

        # Upgrading an active !gt nomination via !!gt invokes AI once
        handled, reply, attach = self.send("person1", "!!gt Welvin Harris. Add photos.")
        self.assertTrue(handled)
        self.assertIn("Added 3 representative photos", reply)
        self.assertTrue(attach.endswith("strip.jpg"))
        self.assertEqual(len(self.ai_calls), 1)

        # Second !!gt does NOT invoke AI again
        _, reply, _ = self.send("person2", "!!gt Welvin Harris")
        self.assertIn("already created", reply)
        self.assertEqual(len(self.ai_calls), 1)

    def test_self_echo_bot_messages_are_never_reprocessed(self):
        self.send("me", "!gt Eva longoria. Iconic.")
        outbox_row = self.store.db.execute("SELECT id, text FROM outbox ORDER BY rowid DESC LIMIT 1").fetchone()
        with self.store.db:
            self.store.db.execute(
                "UPDATE outbox SET pending_id='p-echo', final_id='f-echo' WHERE id=?",
                (outbox_row["id"],),
            )
        self.assertEqual(self.send("me", outbox_row["text"], mid="echo-by-text"), (False, None, ""))
        self.assertEqual(self.send("me", "!!Y", mid="f-echo"), (False, None, ""))

    def test_name_and_reason_parsing_edge_cases(self):
        cases = [
            ("Eva longoria. She made desperate housewives.", ("Eva Longoria", "She made desperate housewives.")),
            ("Samuel L. jackson. Pulp Fiction.", ("Samuel L. Jackson", "Pulp Fiction.")),
            ("j.k. rowling. Harry Potter.", ("J.K. Rowling", "Harry Potter.")),
            ("Robert downey jr. Iron Man.", ("Robert Downey Jr.", "Iron Man.")),
            ("Dr. Dre. Chronic 2001.", ("Dr. Dre", "Chronic 2001.")),
            ("Jean-claude van damme - Bloodsport.", ("Jean-Claude Van Damme", "Bloodsport.")),
            ("LeBron james", ("LeBron James", "")),
        ]
        for raw, expected in cases:
            raw_name, reason = split_nominee_and_reason(raw)
            self.assertEqual((format_nominee_name(raw_name), reason), expected)

    def test_future_classifier_hook_seam(self):
        calls = []

        def custom_classifier(invocation, actor):
            calls.append((invocation.keyword, actor.id))
            return ActionRequest(
                kind="nominate",
                actor=actor,
                year=2026,
                nominee_name="Custom Resolved Name",
                reason="Extracted by classifier hook",
            )

        self.store.classifier = custom_classifier
        handled, reply, _ = self.send("me", "!gt some natural language pitch")
        self.assertTrue(handled)
        self.assertEqual(calls, [("!gt", "me")])
        self.assertIn("Custom Resolved Name", reply)

    def test_gthelp_gtstatus_and_gtlist_never_invoke_ai(self):
        # 1. !gthelp, !!gthelp, !ghelp, and !!gt help all return concise rules without calling AI or opening a case
        for cmd in ("!gthelp", "!!gthelp", "!!GTHELP", "!ghelp", "!!ghelp", "!!gt help", "!gt help"):
            handled, reply, attach = self.send("person1", cmd)
            self.assertTrue(handled)
            self.assertEqual(attach, "")
            self.assertIn("Rules:", reply)
            self.assertIn("!gtstatus", reply)
            self.assertIn("!gtlist", reply)
            self.assertIsNone(self.store.current())
            self.assertEqual(self.ai_calls, [])

        # 2. Idle !gtstatus and !gtlist
        self.store.register("prior:denied", "Prior Denied")
        self.store.import_denial("prior:denied")

        for cmd in ("!gtstatus", "!gstatus"):
            handled, reply, _ = self.send("me", cmd)
            self.assertTrue(handled)
            self.assertIn("No active nomination", reply)
            self.assertIn("0 certified, 1 denied", reply)
            self.assertIn("!gt Name. Pitch", reply)

        for cmd in ("!gtlist", "!glist"):
            handled, reply, _ = self.send("me", cmd)
            self.assertTrue(handled)
            self.assertIn("Winners (0): none yet", reply)
            self.assertIn("Current nominee: none", reply)

        for cmd in ("!gtlist losers", "!glist losers"):
            handled, reply, _ = self.send("me", cmd)
            self.assertTrue(handled)
            self.assertIn("Losers (1): Prior Denied", reply)
            self.assertIn("Current nominee: none", reply)

        # Invalid list mode
        _, reply, _ = self.send("me", "!gtlist invalid")
        self.assertIn("Use !gtlist, !gtlist winners, or !gtlist losers.", reply)

        # 3. Active 1/3 !gtstatus & !gtlist
        self.send("me", "!gt Eva Longoria. Iconic.")
        _, reply, _ = self.send("person1", "!gtstatus")
        self.assertIn("Active: Eva Longoria (1/3", reply)
        self.assertIn("@Me:YES", reply)
        self.assertIn("waiting on @Person1, @Person2", reply)
        self.assertIn("!!gt Eva Longoria for photos", reply)

        _, reply, _ = self.send("person2", "!gtlist winners")
        self.assertIn("Winners (0): none yet", reply)
        self.assertIn("Current nominee: Eva Longoria (1/3 votes in)", reply)

        # 4. Locked 3/3 !gtstatus & certified !gtlist
        self.send("person1", "!!y")
        self.send("person2", "!!y")
        self.now[0] += 15.0
        _, reply, _ = self.send("person1", "!gtstatus")
        self.assertIn("3/3 locked", reply)
        self.assertIn("finalizing in ~45s", reply)
        self.assertIn("!final (@Me)", reply)

        self.send("me", "!final")
        _, reply, _ = self.send("person2", "!gtlist")
        self.assertIn("Winners (1): Eva Longoria", reply)
        self.assertIn("Current nominee: none", reply)
        self.assertEqual(self.ai_calls, [])
