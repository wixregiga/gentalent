from pathlib import Path
import sqlite3
import time
import unicodedata
import uuid
from .config import PREFIX
from .workflow import check_keyword, parse_action, check_state, execute_strategy


class RuleError(Exception):
    pass


def normalized(value):
    return "".join(c for c in unicodedata.normalize("NFKC", value).casefold()
                   if c.isalnum())


SCHEMA = """
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS nominees(id TEXT PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS aliases(alias TEXT PRIMARY KEY, nominee_id TEXT REFERENCES nominees(id));
CREATE TABLE IF NOT EXISTS denials(nominee_id TEXT PRIMARY KEY REFERENCES nominees(id), recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TRIGGER IF NOT EXISTS keep_denials_delete BEFORE DELETE ON denials BEGIN SELECT RAISE(ABORT,'Denials are permanent'); END;
CREATE TRIGGER IF NOT EXISTS keep_denials_update BEFORE UPDATE ON denials BEGIN SELECT RAISE(ABORT,'Denials are permanent'); END;
CREATE TABLE IF NOT EXISTS cases(
  id INTEGER PRIMARY KEY, nominee_id TEXT NOT NULL UNIQUE REFERENCES nominees(id),
  nominator TEXT NOT NULL, phase TEXT NOT NULL,
  ai_invoked INTEGER NOT NULL DEFAULT 0,
  ai_artifact_dir TEXT NOT NULL DEFAULT '',
  all_votes_at REAL,
  messages_sent INTEGER NOT NULL DEFAULT 0);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_case ON cases((1)) WHERE phase='voting';
CREATE TABLE IF NOT EXISTS votes(
  case_id INTEGER REFERENCES cases(id), sender_id TEXT, vote TEXT NOT NULL,
  PRIMARY KEY(case_id,sender_id));
CREATE TABLE IF NOT EXISTS exhibits(
  id INTEGER PRIMARY KEY, case_id INTEGER REFERENCES cases(id),
  text TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS processed(message_id TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS outbox(
  id TEXT PRIMARY KEY, text TEXT NOT NULL, status TEXT NOT NULL,
  pending_id TEXT, final_id TEXT, kind TEXT NOT NULL,
  attachment_path TEXT NOT NULL DEFAULT '',
  case_id INTEGER);
"""


class Store:
    def __init__(self, path, config, media_generator=None, clock=None):
        self.config = config
        self.media_generator = media_generator
        self.clock = clock or time.time
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path)
        self.db.row_factory = sqlite3.Row
        self.db.execute("PRAGMA foreign_keys=ON")
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.executescript(SCHEMA)
        binding = self.get("binding")
        if binding and binding != config.binding:
            raise ValueError("Database belongs to another chat/roster. Use a separate database.")
        with self.db:
            self.set("binding", config.binding)
            # A crash after a POST may mean a real send occurred. Never automatically repeat it.
            self.db.execute("UPDATE outbox SET status='uncertain' WHERE status='sending'")

    def close(self):
        self.db.close()

    def get(self, key, default=None):
        row = self.db.execute("SELECT value FROM meta WHERE key=?", (key,)).fetchone()
        return row[0] if row else default

    def set(self, key, value):
        self.db.execute("INSERT INTO meta VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
                        (key, str(value)))

    def current(self):
        return self.db.execute(
            "SELECT c.*,n.name FROM cases c JOIN nominees n ON n.id=c.nominee_id ORDER BY c.id DESC LIMIT 1"
        ).fetchone()

    def queue(self, text, kind="reply", attachment_path="", case_id=None):
        key = uuid.uuid4().hex[:12]
        self.db.execute(
            "INSERT INTO outbox(id,text,status,kind,attachment_path,case_id) VALUES(?,?,?,?,?,?)",
            (key, f"{PREFIX} {text}", "queued", kind, str(attachment_path or ""), case_id),
        )
        if case_id is not None:
            self.db.execute("UPDATE cases SET messages_sent = messages_sent + 1 WHERE id=?", (case_id,))
        return key

    def generate_media(self, nominee_name, pitch=""):
        if self.media_generator is None:
            from .generative import SubscriptionAgentRunner
            self.media_generator = SubscriptionAgentRunner(self.config).generate
        return self.media_generator(nominee_name, pitch)

    def finalize_case(self, case_id):
        row = self.db.execute(
            "SELECT c.*, n.name FROM cases c JOIN nominees n ON n.id=c.nominee_id WHERE c.id=?",
            (case_id,),
        ).fetchone()
        if row is None or row["phase"] != "voting":
            return False
        votes = [v[0] for v in self.db.execute("SELECT vote FROM votes WHERE case_id=?", (case_id,))]
        if len(votes) < 3:
            return False
        if all(v == "yes" for v in votes):
            self.db.execute("UPDATE cases SET phase='certified' WHERE id=?", (case_id,))
            self.queue(
                f"Congratulations {row['name']}! You are officially a Generational Talent!",
                kind="verdict",
                case_id=case_id,
            )
        else:
            self.db.execute("UPDATE cases SET phase='denied' WHERE id=?", (case_id,))
            self.db.execute("INSERT OR IGNORE INTO denials(nominee_id) VALUES(?)", (row["nominee_id"],))
            yes_count = sum(1 for v in votes if v == "yes")
            self.queue(
                f"{row['name']} did not receive a unanimous vote ({yes_count}/3 yes) "
                f"and is permanently denied from Generational Talent.",
                kind="verdict",
                case_id=case_id,
            )
        return True

    def finalize_due(self, now_epoch=None):
        now_ts = self.clock() if now_epoch is None else float(now_epoch)
        cur = self.current()
        if cur is None or cur["phase"] != "voting" or cur["all_votes_at"] is None:
            return False
        if now_ts - float(cur["all_votes_at"]) < 60.0:
            return False
        if self.db.in_transaction:
            return self.finalize_case(cur["id"])
        with self.db:
            return self.finalize_case(cur["id"])

    def handle(self, message):
        msg_id = message.get("id")
        is_own_outbox = bool(
            msg_id and self.db.execute(
                "SELECT 1 FROM outbox WHERE pending_id=? OR final_id=?",
                (msg_id, msg_id),
            ).fetchone()
        )
        # 1. Check if the keyword was invoked
        invocation = check_keyword(message, self.config, is_own_outbox=is_own_outbox)
        if invocation is None:
            return False
        # 2. Check the action being requested and by whom
        request = parse_action(invocation, message["senderID"], self.config)
        with self.db:
            if self.db.execute("SELECT 1 FROM processed WHERE message_id=?", (msg_id,)).fetchone():
                return False
            self.db.execute("INSERT INTO processed VALUES(?)", (msg_id,))
            self.finalize_due()
            try:
                # 3. Check the current state against what is being asked
                state_check = check_state(self, request, normalized, RuleError)
                # 4. Execute the right strategy / action
                execute_strategy(self, request, state_check, normalized)
            except RuleError as error:
                cur = self.current()
                self.queue(str(error), case_id=cur["id"] if cur else None)
        return True
