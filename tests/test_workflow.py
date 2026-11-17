from pathlib import Path
import tempfile
import unittest
from gentalent.config import Config
from gentalent.generative import GeneratedMediaBundle
from gentalent.state import Store
from gentalent.workflow import format_nominee_name, split_nominee_and_reason


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
            "token_file": "unused.token",
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
            photos = []
            for n in (1, 2, 3):
                photo = out_dir / f"0{n}_fake.jpg"
                photo.write_bytes(b"fake-photo")
                photos.append(photo)
            sources = out_dir / "SOURCES.txt"
            sources.write_text("fake sources")
            return GeneratedMediaBundle(
                nominee_name=name,
                summary="Representative photos gathered.",
                artifact_dir=out_dir,
                photo_paths=photos,
                strip_path=strip,
                sources_path=sources,
                provider_used="codex:gpt-6.1-sol",
            )

        self.fake_media = fake_media
        self.photos = []
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
        rows = self.store.db.execute(
            "SELECT text, attachment_path, kind FROM outbox ORDER BY rowid LIMIT -1 OFFSET ?", (before,)
        ).fetchall()
        # Photo-only messages that followed the reply, in send order.
        self.photos = [Path(r["attachment_path"]).name for r in rows if r["kind"] == "photo"]
        replies = [r for r in rows if r["kind"] != "photo"]
        reply = replies[-1] if replies else None
        return handled, (reply["text"] if reply else None), (reply["attachment_path"] if reply else "")

    def use_layout(self, layout):
        self.store.close()
        self.config = Config(dict(self.config.data, photo_layout=layout), self.config.root)
        self.store = Store(self.config.path("database"), self.config,
                           media_generator=self.fake_media, clock=lambda: self.now[0])

    def verdict(self):
        return self.store.db.execute(
            "SELECT text, attachment_path FROM outbox WHERE kind='verdict' ORDER BY rowid DESC LIMIT 1"
        ).fetchone()

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
            "@gentalent-bot says: @Me would like to nominate Eva Longoria as a 2026 Generational Talent Nominee!  \n"
            "3/3 YES certifies. Any NO denies forever.  \n"
            "1/3 votes are in.\n\n"
            "Vote: !!Y yes · !!N no · !!C clear  \n"
            "Rules: !gthelp",
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
            "@gentalent-bot says: @Person2 has voted for Eva Longoria to be a generational talent!  \n2/3 votes in.",
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
        self.assertIn("3/3 votes in. Result in 1 minute.", reply)
        self.assertIn("Finish now: !final (@Me)", reply)
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
            "@gentalent-bot says: @Me changed their vote to NO on Samuel L. Jackson.  \n1/3 votes in.",
        )
        # Repeating !!N is silent
        _, reply, _ = self.send("me", "!!no")
        self.assertIsNone(reply)

        # Nominator can clear their own vote too -> drops to 0/3
        _, reply, _ = self.send("me", "!!CLEAR")
        self.assertEqual(
            reply,
            "@gentalent-bot says: @Me cleared their vote on Samuel L. Jackson.  \n0/3 votes in.",
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
        self.assertIn("3/3 votes in", reply)
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
        self.assertEqual(attach, "")
        self.assertEqual(self.photos, ["01_fake.jpg", "02_fake.jpg", "03_fake.jpg"])
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
        self.assertIn("Added 3 photos for Welvin Harris.", reply)
        self.assertEqual(attach, "")
        self.assertEqual(self.photos, ["01_fake.jpg", "02_fake.jpg", "03_fake.jpg"])
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

    def test_gthelp_gtstatus_and_gtlist_never_invoke_ai(self):
        # 1. !gthelp, !!gthelp, and !gt help all return concise rules without calling AI or opening a case
        for cmd in ("!gthelp", "!!gthelp", "!!GTHELP", "!!gt help", "!gt help"):
            handled, reply, attach = self.send("person1", cmd)
            self.assertTrue(handled)
            self.assertEqual(attach, "")
            self.assertIn("How GenTalent works", reply)
            self.assertIn("!gtstatus", reply)
            self.assertIn("!gtlist", reply)
            self.assertIsNone(self.store.current())
            self.assertEqual(self.ai_calls, [])

        # 2. Idle !gtstatus and !gtlist after one real denial
        self.send("me", "!gt Prior Denied. Pitch.")
        self.send("person1", "!!n")
        self.send("person2", "!!y")
        self.send("me", "!final")
        self.assertEqual(self.store.current()["phase"], "denied")

        for cmd in ("!gtstatus",):
            handled, reply, _ = self.send("me", cmd)
            self.assertTrue(handled)
            self.assertIn("No vote right now", reply)
            self.assertIn("Last: Prior Denied, denied", reply)
            self.assertIn("0 certified, 1 denied", reply)
            self.assertIn("!gt Name. Pitch", reply)

        for cmd in ("!gtlist",):
            handled, reply, _ = self.send("me", cmd)
            self.assertTrue(handled)
            self.assertIn("Winners (0)  \nNone yet", reply)
            self.assertIn("Now voting: none", reply)

        for cmd in ("!gtlist losers",):
            handled, reply, _ = self.send("me", cmd)
            self.assertTrue(handled)
            self.assertIn("Losers (1)  \n🚫 Prior Denied", reply)
            self.assertIn("Now voting: none", reply)

        # Invalid list mode
        _, reply, _ = self.send("me", "!gtlist invalid")
        self.assertIn("Use !gtlist, !gtlist winners, or !gtlist losers.", reply)

        # 3. Active 1/3 !gtstatus & !gtlist
        self.send("me", "!gt Eva Longoria. Iconic.")
        _, reply, _ = self.send("person1", "!gtstatus")
        self.assertIn("Now voting: Eva Longoria  \n1/3 votes in.", reply)
        self.assertIn("✅ Me: YES  \n⏳ Person1: waiting  \n⏳ Person2: waiting", reply)
        self.assertIn("Photos: send !!gt Eva Longoria", reply)
        self.assertIn("Vote: !!Y yes · !!N no · !!C clear", reply)

        _, reply, _ = self.send("person2", "!gtlist winners")
        self.assertIn("Winners (0)  \nNone yet", reply)
        self.assertIn("Now voting: Eva Longoria (1/3 votes in)", reply)

        # 4. Locked 3/3 !gtstatus & certified !gtlist
        self.send("person1", "!!y")
        self.send("person2", "!!y")
        self.now[0] += 15.0
        _, reply, _ = self.send("person1", "!gtstatus")
        self.assertIn("3/3 votes in, locked. Result in about 45s.", reply)
        self.assertIn("Finish now: !final (@Me)", reply)
        self.assertNotIn("Vote: !!Y", reply)

        self.send("me", "!final")
        _, reply, _ = self.send("person2", "!gtlist")
        self.assertIn("Winners (1)  \n🏆 Eva Longoria", reply)
        self.assertIn("Now voting: none", reply)
        self.assertEqual(self.ai_calls, [])

    def test_unrequested_short_aliases_are_not_commands(self):
        for cmd in ("!ghelp", "!gstatus", "!glist", "!!ghelp", "!glist losers"):
            self.assertEqual(self.send("person1", cmd), (False, None, ""))

    def test_outsiders_other_chats_and_edited_messages_are_ignored(self):
        base = {"chatID": "friends-group", "accountID": "gmessages-1", "senderID": "me",
                "text": "!gt Eva Longoria. Iconic.", "timestamp": "2026-11-16T18:00:00Z"}
        for n, changes in enumerate(({"senderID": "outsider"}, {"chatID": "another-chat"},
                                     {"accountID": "another-account"}, {"editedTimestamp": "2026-01-01"},
                                     {"isDeleted": True}, {"isHidden": True})):
            self.assertFalse(self.store.handle(dict(base, id=f"ignored-{n}", **changes)))
        self.assertIsNone(self.store.current())

    def test_replayed_message_is_not_counted_twice_after_restart(self):
        self.send("me", "!gt Eva Longoria. Iconic.")
        self.send("person1", "!!y", mid="vote-once")
        before = self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
        self.store.close()
        self.store = Store(self.config.path("database"), self.config, clock=lambda: self.now[0])
        self.assertEqual(self.send("person1", "!!y", mid="vote-once"), (False, None, ""))
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0], before)
        self.assertEqual(self.store.db.execute("SELECT COUNT(*) FROM votes").fetchone()[0], 2)

    def test_denials_cannot_be_deleted_or_changed(self):
        import sqlite3
        self.send("me", "!gt Eva Longoria. Iconic.")
        self.send("person1", "!!n")
        self.send("person2", "!!y")
        self.send("me", "!final")
        for sql in ("DELETE FROM denials", "UPDATE denials SET recorded_at='x'"):
            with self.assertRaises(sqlite3.IntegrityError):
                with self.store.db:
                    self.store.db.execute(sql)
        _, reply, _ = self.send("person1", "!gt EVA LONGORIA. Again.")
        self.assertIn("can never be nominated again", reply)

    def finish(self, *votes):
        for sender, vote in zip(("person1", "person2"), votes):
            self.send(sender, vote)
        self.send("me", "!final")

    def test_individual_layout_puts_the_strip_on_the_verdict(self):
        self.send("me", "!!gt Eva Longoria. Iconic.")
        self.finish("!!y", "!!y")
        row = self.verdict()
        self.assertIn("Congratulations Eva Longoria", row["text"])
        self.assertTrue(row["attachment_path"].endswith("strip.jpg"))

    def test_individual_layout_puts_the_strip_on_a_denial_too(self):
        self.send("me", "!!gt Eva Longoria. Iconic.")
        self.finish("!!n", "!!y")
        self.assertTrue(self.verdict()["attachment_path"].endswith("strip.jpg"))

    def test_verdict_has_no_photo_when_none_were_generated(self):
        self.send("me", "!gt Eva Longoria. Iconic.")
        self.finish("!!y", "!!y")
        self.assertEqual(self.verdict()["attachment_path"], "")
        self.assertEqual(self.ai_calls, [])

    def test_verdict_has_no_photo_when_the_strip_file_is_gone(self):
        self.send("me", "!!gt Eva Longoria. Iconic.")
        (Path(self.temp.name) / "media" / "fake" / "strip.jpg").unlink()
        self.finish("!!y", "!!y")
        self.assertEqual(self.verdict()["attachment_path"], "")

    def test_stitched_layout_attaches_the_strip_to_the_nomination_only(self):
        self.use_layout("stitched")
        _, reply, attach = self.send("me", "!!gt Eva Longoria. Iconic.")
        self.assertIn("would like to nominate Eva Longoria", reply)
        self.assertTrue(attach.endswith("strip.jpg"))
        self.assertEqual(self.photos, [])
        self.finish("!!y", "!!y")
        self.assertEqual(self.verdict()["attachment_path"], "")

    def test_photo_messages_have_no_text_and_follow_the_nomination(self):
        self.send("me", "!!gt Eva Longoria. Iconic.")
        rows = self.store.db.execute("SELECT text, kind FROM outbox ORDER BY rowid").fetchall()
        self.assertEqual([r["kind"] for r in rows], ["reply", "photo", "photo", "photo"])
        self.assertTrue(rows[0]["text"].startswith("@gentalent-bot says:"))
        self.assertEqual([r["text"] for r in rows[1:]], ["", "", ""])

    def test_unknown_photo_layout_is_rejected(self):
        data = dict(self.config.data, photo_layout="grid", members=self.config.data["members"])
        with self.assertRaises(ValueError):
            Config(data, self.config.root).photo_layout

    def test_no_reply_line_starts_with_markdown_syntax(self):
        import re
        self.send("me", "!gthelp")
        self.send("me", "!gtstatus")
        self.send("me", "!gt Eva Longoria. Iconic.")
        self.send("person1", "!!n")
        self.send("me", "!gtstatus")
        self.send("person1", "!gtlist")
        self.send("person1", "!gtlist losers")
        texts = [r[0] for r in self.store.db.execute("SELECT text FROM outbox WHERE text <> ''")]
        self.assertGreaterEqual(len(texts), 7)
        for text in texts:
            for line in text.split("\n"):
                self.assertIsNone(re.match(r"\s*([-*+#>]|\d+[.)])\s", line), line)
            for block in text.split("\n\n"):
                inner = block.split("\n")
                self.assertTrue(all(line.endswith("  ") for line in inner[:-1]), block)
