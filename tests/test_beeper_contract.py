"""Beeper Desktop API behavior, from the v5.0.0 spec and from the live Google Messages group.

Each fixture mirrors a real response shape captured on 2026-11-16 (Beeper 4.3.308), with
personal IDs replaced. doc/beeper-api.md says which parts are documented and which are only
observed. These tests prove the send code handles that behavior; they cannot detect Beeper
changing. `gentalent doctor --contract` checks the real API for that.
"""
from contextlib import redirect_stdout
import io
import tempfile
import unittest
from gentalent.net import APIError
from gentalent.runner import delivery_state, flush, same_text
from gentalent.state import Store
from support import make_config

# Observed: Google Messages pending IDs look like this. The spec's own example is
# "m1694783291234567", so code must not depend on the "~" prefix.
PENDING_ID = "~beeper-mautrix-go_1794872979269056428_2"

# Observed: the final numeric ID appears while sendStatus still reads PENDING.
FINAL_ID_STILL_PENDING = {
    "id": "739", "chatID": "demo-group", "accountID": "demo-google", "senderID": "alice",
    "timestamp": "2026-11-16T23:49:39.327Z", "sortKey": "4164", "type": "TEXT",
    "text": "@gentalent-bot says: reply", "isSender": True, "isDeleted": False,
    "sendStatus": {"reason": "Awaiting send confirmation", "status": "PENDING",
                   "timestamp": "2026-11-16T23:49:39.327Z", "deliveredToUsers": []},
}

# Observed: once delivered, the sendStatus key is absent (never "SUCCESS" in this chat).
DELIVERED_TEXT = {k: v for k, v in FINAL_ID_STILL_PENDING.items() if k != "sendStatus"}

# Observed: a reply with a photo arrives as two messages. The pending ID resolves to the
# IMAGE message, which has no "text" key; the text follows as a separate TEXT message.
DELIVERED_PHOTO = {
    "id": "747", "chatID": "demo-group", "accountID": "demo-google", "senderID": "alice",
    "timestamp": "2026-11-16T23:55:27.129Z", "sortKey": "4179", "type": "IMAGE",
    "isSender": True, "isDeleted": False,
    "attachments": [{"id": "mxc://local/x", "type": "img", "mimeType": "image/jpeg",
                     "fileName": "strip.jpg", "fileSize": 1, "isGif": False, "isSticker": False,
                     "size": {"width": 1, "height": 1}, "srcURL": "localmxc://x"}],
}


class DeliveryStateTest(unittest.TestCase):
    def test_documented_send_status_values(self):
        for status, expected in (("SUCCESS", "sent"), ("FAIL_RETRIABLE", "failed"),
                                 ("FAIL_PERMANENT", "failed"), ("PENDING", "pending")):
            message = dict(DELIVERED_TEXT, sendStatus={"status": status, "timestamp": "t"})
            self.assertEqual(delivery_state(message, PENDING_ID)[0], expected)

    def test_final_id_with_pending_status_is_still_pending(self):
        self.assertEqual(delivery_state(FINAL_ID_STILL_PENDING, PENDING_ID), ("pending", "739"))

    def test_absent_send_status_with_final_id_is_sent(self):
        self.assertEqual(delivery_state(DELIVERED_TEXT, PENDING_ID), ("sent", "739"))
        self.assertEqual(delivery_state(DELIVERED_PHOTO, PENDING_ID), ("sent", "747"))

    def test_absent_send_status_on_the_pending_id_itself_is_still_pending(self):
        message = dict(DELIVERED_TEXT, id=PENDING_ID)
        self.assertEqual(delivery_state(message, PENDING_ID)[0], "pending")

    def test_pending_ids_without_tilde_work(self):
        self.assertEqual(delivery_state(DELIVERED_TEXT, "m1694783291234567")[0], "sent")


# Observed: what was sent for the Jordana Brewster nomination, and what Beeper stored.
SENT_FORMATTED = ("@gentalent-bot says: @Avery would like to nominate Jordana Brewster as a 2026 "
                  "Generational Talent Nominee!  \n3/3 YES certifies. Any NO denies forever.  \n"
                  "1/3 votes are in.\n\nVote: !!Y yes · !!N no · !!C clear  \nRules: !gthelp")
STORED_FORMATTED = ("<p>@gentalent-bot says: @Avery would like to nominate Jordana Brewster as a 2026 "
                    "Generational Talent Nominee!<br>3/3 YES certifies. Any NO denies forever.<br>"
                    "1/3 votes are in.</p>\n<p>Vote: !!Y yes · !!N no · !!C clear<br>Rules: !gthelp</p>")


class StoredHtmlTest(unittest.TestCase):
    def test_formatted_reply_matches_its_stored_html(self):
        self.assertTrue(same_text(STORED_FORMATTED, SENT_FORMATTED))
        self.assertTrue(same_text("Tom &amp; Jerry<br>2/3 votes in.", "Tom & Jerry  \n2/3 votes in."))
        # Quoted chat text keeps its Markdown when sent; Beeper stores the rendered HTML.
        self.assertTrue(same_text('They said: &quot;<em>mid</em>&quot;<br>Next', 'They said: "*mid*"  \nNext'))
        self.assertTrue(same_text("I <3 you > all", "I <3 you > all"), "a single-line reply is stored as plain text")

    def test_different_words_still_differ(self):
        self.assertFalse(same_text(STORED_FORMATTED.replace("1/3", "2/3"), SENT_FORMATTED))


class ObservedSendLifecycleTest(unittest.TestCase):
    """Replays the sequence the live group went through, end to end through flush()."""

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.config = make_config(self.temp.name)
        self.store = Store(self.config.path("database"), self.config)
        self.responses = []
        self.sent = []

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def send(self, text, attachment_path=None):
        self.sent.append((text, attachment_path))
        return PENDING_ID

    def retrieve(self, message_id):
        self.assertEqual(message_id, PENDING_ID)
        response = self.responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response

    def flush(self):
        with redirect_stdout(io.StringIO()):
            flush(self.store, self, live=True)
        return self.store.db.execute("SELECT status, final_id FROM outbox").fetchone()

    def test_text_reply_pending_then_final_id_then_status_disappears(self):
        with self.store.db:
            self.store.queue("reply")
        self.assertEqual(tuple(self.flush()), ("pending", None))
        self.responses = [FINAL_ID_STILL_PENDING, DELIVERED_TEXT]
        self.assertEqual(tuple(self.flush()), ("pending", None))
        self.assertEqual(tuple(self.flush()), ("sent", "739"))
        self.assertEqual(len(self.sent), 1)

    def test_photo_reply_resolves_to_the_image_message(self):
        with self.store.db:
            self.store.queue("nomination", attachment_path="/media/strip.jpg")
        self.flush()
        self.responses = [DELIVERED_PHOTO]
        self.assertEqual(tuple(self.flush()), ("sent", "747"))
        self.assertEqual(self.sent, [("@gentalent-bot says: nomination", "/media/strip.jpg")])

    def test_pending_id_that_expired_while_unresolved_becomes_uncertain_once(self):
        # Observed: about 58 minutes after the send, the pending ID returned HTTP 404.
        with self.store.db:
            self.store.queue("reply")
        self.flush()
        self.responses = [APIError("API returned HTTP 404; no automatic write retry was made.", status_code=404)]
        self.assertEqual(tuple(self.flush()), ("uncertain", None))
        self.assertEqual(tuple(self.flush()), ("uncertain", None))
        self.assertEqual(len(self.sent), 1)


if __name__ == "__main__":
    unittest.main()
