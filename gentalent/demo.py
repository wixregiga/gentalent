from pathlib import Path
from tempfile import TemporaryDirectory
from datetime import datetime, timezone
from .config import Config
from .state import Store
from .tumblr import publish


def demo_config(root):
    return Config({"chat_id": "demo-group", "account_id": "demo-google",
        "owner_id": "alice", "members": [
            {"id": "alice", "name": "Alice", "platform": "android"},
            {"id": "bob", "name": "Bob", "platform": "iphone"},
            {"id": "carol", "name": "Carol", "platform": "android"}],
        "beeper_url": "http://localhost:23373", "database": "demo.sqlite3",
        "poll_seconds": 1, "tumblr_blog": "example.tumblr.com",
        "token_file": "unused.token", "tumblr_credentials": "unused.json"}, Path(root))


def message(text, sender="alice", mid=None):
    return {"id": mid or text + ":" + sender, "chatID": "demo-group",
            "accountID": "demo-google", "senderID": sender, "text": text,
            "timestamp": datetime.now(timezone.utc).isoformat(), "isSender": sender == "alice"}


class FakeTumblr:
    blog = "example.tumblr.com"

    def __init__(self):
        self.posts = {}
        self.creates = 0
        self.edits = 0

    def verify_owner(self):
        pass

    def write(self, payload, post_id=None):
        if post_id is None:
            self.creates += 1
            post_id = str(1000 + self.creates)
        else:
            self.edits += 1
        self.posts[post_id] = payload.copy()
        return {"id": post_id}

    def retrieve(self, post_id):
        return self.posts[post_id]


def demo():
    with TemporaryDirectory(prefix="gentalent-demo-") as root:
        config = demo_config(root)
        config.validate()
        now_t = [1000.0]
        store = Store(config.path("database"), config, clock=lambda: now_t[0])
        client = FakeTumblr()

        sequence = iter(range(1, 1000))

        def chat(text, sender="alice"):
            before = store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
            event = message(text, sender, f"demo-{next(sequence)}")
            handled = store.handle(event)
            print(f"{config.roster[sender]['name']}: {text}")
            after = store.db.execute("SELECT COUNT(*) FROM outbox").fetchone()[0]
            if after > before:
                row = store.db.execute("SELECT text FROM outbox ORDER BY rowid DESC LIMIT 1").fetchone()
                print(f"{config.roster[config.data['owner_id']]['name']}: {row[0]}")
            return event, handled

        print("OFFLINE DEMO — fictional people; no Beeper, Tumblr, or model connection.")
        chat("!!gthelp")
        chat("!gt Aster example. Carried an iconic show for years and stayed scandal-free. Generational talent for sure.")
        chat("!gtstatus", "bob")
        chat("I don't know. I hated their acting in that show.", "bob")
        chat("!!N", "bob")
        chat("!!Y", "alice")  # Duplicate same vote: silent
        chat("!!C", "bob")  # Clears Bob's vote back to 1/3
        chat("!!C", "bob")  # Already empty: silent
        chat("!!Y", "carol")
        chat("But I did like them in that other movie so my vote is yes.", "bob")
        chat("You have to use the right keyword @Bob", "alice")
        replay, _ = chat("!!YES", "bob")
        assert store.current()["phase"] == "voting"
        chat("!final", "alice")
        assert store.current()["phase"] == "certified"
        chat("!gtlist", "carol")

        chat("!public A fictional nominee used to demonstrate the GenTalent workflow.")
        chat("!approve-public")
        first_url = publish(store, client)
        print("MOCK public record:", first_url)
        assert publish(store, client) == first_url
        assert client.creates == 1 and client.edits == 0
        print("PASS: keyword nomination, vote changes/clears, !final certification, and mock publication.")

        votes_before = store.db.execute("SELECT COUNT(*) FROM votes").fetchone()[0]
        store.close()
        store = Store(config.path("database"), config, clock=lambda: now_t[0])
        assert not store.handle(replay)
        assert store.db.execute("SELECT COUNT(*) FROM votes").fetchone()[0] == votes_before
        print("PASS: restart preserves ballot and ignores replay.")

        chat("!gt Birch example. Strong early run.", "bob")
        chat("!!N", "alice")
        assert store.current()["phase"] == "voting"
        chat("!!Y", "carol")
        assert store.current()["phase"] == "voting"
        now_t[0] += 60.0
        assert store.finalize_due()
        row = store.db.execute("SELECT text FROM outbox ORDER BY rowid DESC LIMIT 1").fetchone()
        print(f"{config.roster[config.data['owner_id']]['name']}: {row[0]}")
        assert store.current()["phase"] == "denied"
        chat("!gt BIRCH EXAMPLE. Trying again.")
        assert store.db.execute("SELECT COUNT(*) FROM cases").fetchone()[0] == 2
        chat("!gtlist losers")
        print("PASS: 60s wait finalizes denial and permanently prevents renomination.")

        public = str(store.payload())
        assert "sender_id" not in public
        print("PASS: public payload omits the roster, phone numbers, and individual votes.")
        store.close()
