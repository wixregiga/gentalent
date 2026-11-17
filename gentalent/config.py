from dataclasses import dataclass
from pathlib import Path
import hashlib
import json
from urllib.parse import urlsplit

PREFIX = "@gentalent-bot says:"


@dataclass(frozen=True)
class Config:
    data: dict
    root: Path

    @classmethod
    def load(cls, path):
        path = Path(path).expanduser().resolve()
        return cls(json.loads(path.read_text()), path.parent)

    def path(self, key):
        p = Path(self.data[key]).expanduser()
        return p if p.is_absolute() else self.root / p

    @property
    def roster(self):
        return {m["id"]: m for m in self.data["members"]}

    def validate_base(self):
        u = urlsplit(self.data["beeper_url"])
        if u.scheme != "http" or u.hostname not in ("localhost", "127.0.0.1", "::1") or u.username or u.password or u.query or u.fragment or u.path not in ("", "/"):
            raise ValueError("Use a local loopback Beeper URL without credentials or a path.")

    def validate(self):
        self.validate_base()
        d = self.data
        values = [d.get("chat_id", ""), d.get("account_id", ""), d.get("owner_id", "")]
        if not all(values) or any("REPLACE" in v for v in values):
            raise ValueError("Configure the exact chat, account, and owner IDs first.")
        members = d.get("members", [])
        if len(members) != 3 or len(self.roster) != 3:
            raise ValueError("Exactly three distinct eligible sender IDs are required.")
        if any(not m["id"] or "REPLACE" in m["id"] for m in members):
            raise ValueError("Replace the three roster sender IDs.")
        if d["owner_id"] not in self.roster:
            raise ValueError("The owner must be in the three-person roster.")
        if any(m.get("platform") not in ("android", "iphone", "unknown") for m in members):
            raise ValueError("Confirm each member's platform: android, iphone, or unknown.")
        if not 1 <= float(d.get("poll_seconds", 3)) <= 3600:
            raise ValueError("poll_seconds must be between 1 and 3600.")

    @property
    def binding(self):
        scope = [self.data["chat_id"], self.data["account_id"],
                 self.data["owner_id"], sorted(self.roster)]
        return hashlib.sha256(json.dumps(scope).encode()).hexdigest()
