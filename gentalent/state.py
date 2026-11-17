import hashlib
import json
from pathlib import Path
import re
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


def digest(payload):
    return hashlib.sha256(json.dumps(payload, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


class Store:
    def __init__(self, path, config, classifier=None, media_generator=None, clock=None):
        self.config = config
        self.classifier = classifier
        self.media_generator = media_generator
        self.clock = clock or time.time
        path = Path(path)
        path.parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(path)
        self.db.row_factory = sqlite3.Row
        self.db.execute("PRAGMA foreign_keys=ON")
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.executescript("""
        CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS nominees(id TEXT PRIMARY KEY, name TEXT NOT NULL);
        CREATE TABLE IF NOT EXISTS aliases(alias TEXT PRIMARY KEY, nominee_id TEXT REFERENCES nominees(id));
        CREATE TABLE IF NOT EXISTS denials(nominee_id TEXT PRIMARY KEY REFERENCES nominees(id), recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
        CREATE TRIGGER IF NOT EXISTS keep_denials_delete BEFORE DELETE ON denials BEGIN SELECT RAISE(ABORT,'Denials are permanent'); END;
        CREATE TRIGGER IF NOT EXISTS keep_denials_update BEFORE UPDATE ON denials BEGIN SELECT RAISE(ABORT,'Denials are permanent'); END;
        CREATE TABLE IF NOT EXISTS cases(
          id INTEGER PRIMARY KEY, nominee_id TEXT NOT NULL UNIQUE REFERENCES nominees(id),
          nominator TEXT NOT NULL, phase TEXT NOT NULL,
          public_text TEXT NOT NULL DEFAULT '', public_links TEXT NOT NULL DEFAULT '[]',
          approved_hash TEXT, post_id TEXT, published_hash TEXT, public_url TEXT,
          publish_state TEXT NOT NULL DEFAULT 'idle',
          ai_invoked INTEGER NOT NULL DEFAULT 0,
          ai_artifact_dir TEXT NOT NULL DEFAULT '',
          all_votes_at REAL,
          messages_sent INTEGER NOT NULL DEFAULT 0);
        CREATE UNIQUE INDEX IF NOT EXISTS one_active_case ON cases((1)) WHERE phase IN ('case','voting');
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
        """)
        self._migrate_schema()
        binding = self.get("binding")
        if binding and binding != config.binding:
            raise ValueError("Database belongs to another chat/roster. Use a separate database.")
        with self.db:
            self.set("binding", config.binding)
            # A crash after a POST may mean a real send occurred. Never automatically repeat it.
            self.db.execute("UPDATE outbox SET status='uncertain' WHERE status='sending'")
            self.db.execute("UPDATE cases SET publish_state='uncertain' WHERE publish_state='sending'")

    def _migrate_schema(self):
        case_cols = {row["name"] for row in self.db.execute("PRAGMA table_info(cases)").fetchall()}
        outbox_cols = {row["name"] for row in self.db.execute("PRAGMA table_info(outbox)").fetchall()}
        with self.db:
            if "ai_invoked" not in case_cols:
                self.db.execute("ALTER TABLE cases ADD COLUMN ai_invoked INTEGER NOT NULL DEFAULT 0")
            if "ai_artifact_dir" not in case_cols:
                self.db.execute("ALTER TABLE cases ADD COLUMN ai_artifact_dir TEXT NOT NULL DEFAULT ''")
            if "all_votes_at" not in case_cols:
                self.db.execute("ALTER TABLE cases ADD COLUMN all_votes_at REAL")
            if "messages_sent" not in case_cols:
                self.db.execute("ALTER TABLE cases ADD COLUMN messages_sent INTEGER NOT NULL DEFAULT 1")
            if "attachment_path" not in outbox_cols:
                self.db.execute("ALTER TABLE outbox ADD COLUMN attachment_path TEXT NOT NULL DEFAULT ''")
            if "case_id" not in outbox_cols:
                self.db.execute("ALTER TABLE outbox ADD COLUMN case_id INTEGER")

    def close(self):
        self.db.close()

    def get(self, key, default=None):
        row = self.db.execute("SELECT value FROM meta WHERE key=?", (key,)).fetchone()
        return row[0] if row else default

    def set(self, key, value):
        self.db.execute("INSERT INTO meta VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
                        (key, str(value)))

    def register(self, identity, name, aliases=()):
        if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9:._/-]{1,199}", identity):
            raise ValueError("Use a stable identity such as wikidata:Q123 or demo:person.")
        if not name.strip() or len(name) > 200:
            raise ValueError("Provide a name between 1 and 200 characters.")
        keys = {normalized(v) for v in [identity, name, *aliases]}
        if "" in keys:
            raise ValueError("Aliases must contain letters or numbers.")
        with self.db:
            old = self.db.execute("SELECT * FROM nominees WHERE id=?", (identity,)).fetchone()
            if old and old["name"] != name:
                raise ValueError("An existing identity's name is immutable; add an alias instead.")
            for key in keys:
                row = self.db.execute("SELECT nominee_id FROM aliases WHERE alias=?", (key,)).fetchone()
                if row and row[0] != identity:
                    raise ValueError("That name/alias already belongs to a different identity.")
            self.db.execute("INSERT OR IGNORE INTO nominees VALUES(?,?)", (identity, name))
            self.db.executemany("INSERT OR IGNORE INTO aliases VALUES(?,?)",
                                [(key, identity) for key in keys])

    def import_denial(self, identity):
        with self.db:
            prior = self.db.execute("SELECT phase FROM cases WHERE nominee_id=?", (identity,)).fetchone()
            if prior and prior[0] != "denied":
                raise ValueError("A prior denial conflicts with an existing active/certified case; resolve that record first.")
            self.db.execute("INSERT OR IGNORE INTO denials(nominee_id) VALUES(?)", (identity,))

    def current(self):
        return self.db.execute(
            "SELECT c.*,n.name FROM cases c JOIN nominees n ON n.id=c.nominee_id ORDER BY c.id DESC LIMIT 1"
        ).fetchone()

    def active(self):
        row = self.current()
        if row is None or row["phase"] not in ("case", "voting"):
            raise RuleError("There is no active nomination.")
        return row

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
            self.media_generator = SubscriptionAgentRunner(self.config)
        if callable(self.media_generator):
            return self.media_generator(nominee_name, pitch)
        return self.media_generator.generate(nominee_name, pitch)

    def finalize_case(self, case_id):
        row = self.db.execute(
            "SELECT c.*, n.name FROM cases c JOIN nominees n ON n.id=c.nominee_id WHERE c.id=?",
            (case_id,),
        ).fetchone()
        if row is None or row["phase"] != "voting":
            return False
        votes = [
            v[0]
            for v in self.db.execute("SELECT vote FROM votes WHERE case_id=?", (case_id,)).fetchall()
        ]
        if len(votes) < 3:
            return False
        if all(v == "yes" for v in votes):
            self.db.execute(
                "UPDATE cases SET phase='certified', approved_hash=NULL WHERE id=?",
                (case_id,),
            )
            self.queue(
                f"Congratulations {row['name']}! You are officially a Generational Talent!",
                kind="verdict",
                case_id=case_id,
            )
        else:
            self.db.execute(
                "UPDATE cases SET phase='denied', approved_hash=NULL WHERE id=?",
                (case_id,),
            )
            self.db.execute(
                "INSERT OR IGNORE INTO denials(nominee_id) VALUES(?)",
                (row["nominee_id"],),
            )
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
        request = parse_action(invocation, message["senderID"], self.config, self.classifier)
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

    def _command(self, command, arg, sender):
        if command == "!nominate":
            alias = self.db.execute("SELECT nominee_id FROM aliases WHERE alias=?", (normalized(arg),)).fetchone()
            if not alias:
                raise RuleError("Unknown nominee. Ask the organizer to register one stable identity and aliases first.")
            if self.db.execute("SELECT 1 FROM denials WHERE nominee_id=?", (alias[0],)).fetchone():
                raise RuleError("This nominee was denied and can never be nominated again.")
            prior = self.db.execute("SELECT phase FROM cases WHERE nominee_id=?", (alias[0],)).fetchone()
            if prior:
                if prior[0] == "denied":
                    raise RuleError("This nominee was denied and can never be nominated again.")
                raise RuleError("This nominee already has a case or a final certification.")
            cur = self.current()
            if cur and cur["phase"] in ("case", "voting"):
                raise RuleError("Finish the active nomination first.")
            case_id = self.db.execute("INSERT INTO cases(nominee_id,nominator,phase) VALUES(?,?,'case')",
                                       (alias[0], sender)).lastrowid
            self.db.execute("INSERT INTO votes VALUES(?,?,'yes')", (case_id, sender))
            self.queue(f"{self.current()['name']} nominated · 1 of 3 votes in", case_id=case_id)
            return
        if command in ("!status", "!summary"):
            row = self.current()
            if not row:
                raise RuleError("There is no nomination yet.")
            votes = self.db.execute("SELECT vote FROM votes WHERE case_id=?", (row["id"],)).fetchall()
            msg = f"{row['name']} · {row['phase']} · {len(votes)} of 3 votes in"
            if command == "!summary":
                exhibits = self.db.execute("SELECT text FROM exhibits WHERE case_id=? ORDER BY id", (row["id"],))
                msg += " · Selected exhibits: " + ("; ".join(e[0] for e in exhibits) or "none")
            self.queue(msg[:4000])
            return
        if command in ("!public", "!public-link", "!approve-public"):
            if sender != self.config.data["owner_id"]:
                raise RuleError("Only the organizer can prepare or approve the public record.")
            row = self.current()
            if not row:
                raise RuleError("There is no nomination yet.")
            if row["publish_state"] in ("sending", "uncertain"):
                raise RuleError("Resolve the uncertain Tumblr operation before changing the approved record.")
            if command == "!public":
                if not arg or len(arg) > 8000:
                    raise RuleError("Use !public followed by the exact approved case text (up to 8000 characters).")
                self.db.execute("UPDATE cases SET public_text=?,approved_hash=NULL WHERE id=?", (arg, row["id"]))
                self.queue("Public case text prepared. Review with preview before !approve-public.")
            elif command == "!public-link":
                from urllib.parse import urlsplit
                parsed = urlsplit(arg)
                if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or len(arg) > 2000:
                    raise RuleError("Use !public-link with one public HTTPS exhibit URL.")
                links = json.loads(row["public_links"])
                if arg not in links:
                    links.append(arg)
                if len(links) > 20:
                    raise RuleError("The MVP allows at most 20 public exhibit links.")
                self.db.execute("UPDATE cases SET public_links=?,approved_hash=NULL WHERE id=?",
                                (json.dumps(links), row["id"]))
                self.queue("Public exhibit link prepared; approval is required before publication.")
            else:
                if not row["public_text"]:
                    raise RuleError("Prepare the public case text with !public first.")
                self.db.execute("UPDATE cases SET approved_hash=? WHERE id=?", (digest(self.payload()), row["id"]))
                self.queue("Current public record approved. The organizer can now publish this exact version.")
            return
        row = self.active()
        if command == "!exhibit":
            if sender != row["nominator"]:
                raise RuleError("Only the nominator can select the case's exhibits.")
            if row["phase"] not in ("case", "voting"):
                raise RuleError("Exhibits are frozen once voting concludes.")
            if not arg or len(arg) > 2000:
                raise RuleError("Use !exhibit with a reason, source link, or an attachment label.")
            self.db.execute("INSERT INTO exhibits(case_id,text) VALUES(?,?)", (row["id"], arg))
            self.queue("Exhibit recorded privately. Public sharing requires explicit organizer selection.")
        elif command == "!openvote":
            if sender not in (row["nominator"], self.config.data["owner_id"]):
                raise RuleError("Only the organizer or nominator can open final voting.")
            if row["phase"] != "case":
                raise RuleError("Final voting is already open.")
            self.db.execute("UPDATE cases SET phase='voting',approved_hash=NULL WHERE id=?", (row["id"],))
            self.queue("Final voting open · use !vote yes or !vote no · the nominator's yes is already final")
        elif command == "!vote":
            if row["phase"] != "voting":
                raise RuleError("Open final voting with !openvote first.")
            if arg.casefold() not in ("yes", "no"):
                raise RuleError("Use exactly !vote yes or !vote no.")
            if self.db.execute("SELECT 1 FROM votes WHERE case_id=? AND sender_id=?", (row["id"], sender)).fetchone():
                raise RuleError("Your final vote is already recorded and cannot change.")
            self.db.execute("INSERT INTO votes VALUES(?,?,?)", (row["id"], sender, arg.casefold()))
            votes = [v[0] for v in self.db.execute("SELECT vote FROM votes WHERE case_id=?", (row["id"],))]
            if len(votes) == 3:
                result = "certified" if all(v == "yes" for v in votes) else "denied"
                self.db.execute("UPDATE cases SET phase=?,approved_hash=NULL WHERE id=?", (result, row["id"]))
                if result == "denied":
                    self.db.execute("INSERT OR IGNORE INTO denials(nominee_id) VALUES(?)", (row["nominee_id"],))
                self.queue(f"{row['name']} · {result.upper()} · all 3 final votes recorded", "verdict")
            else:
                self.queue(f"Final vote recorded · {len(votes)} of 3 votes in")

    def payload(self):
        row = self.current()
        if not row:
            raise ValueError("No case to preview.")
        heading = f"{row['name']} — {row['phase'].title()}"
        content = [{"type": "text", "subtype": "heading1", "text": heading},
                   {"type": "text", "text": row["public_text"] or "(Public case text not prepared.)"}]
        if row["phase"] == "certified":
            content.append({"type": "text", "text": "🏛️ Certified Generational Talent"})
        for link in json.loads(row["public_links"]):
            content.append({"type": "text", "text": link,
                            "formatting": [{"start": 0, "end": len(link), "type": "link", "url": link}]})
        return {"content": content, "state": "published", "tags": "GenTalent",
                "slug": f"gentalent-{row['id']}"}

    def snapshot(self):
        return {
            "cases": [dict(r) for r in self.db.execute(
                "SELECT c.id,n.name,c.nominee_id,c.phase,c.public_url,c.publish_state FROM cases c JOIN nominees n ON n.id=c.nominee_id ORDER BY c.id")],
            "outbox": [dict(r) for r in self.db.execute("SELECT id,status,kind FROM outbox ORDER BY rowid")],
        }
