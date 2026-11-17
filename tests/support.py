from datetime import datetime, timezone
from pathlib import Path
from gentalent.config import Config


def make_config(root):
    return Config({
        "chat_id": "demo-group",
        "account_id": "demo-google",
        "owner_id": "alice",
        "members": [
            {"id": "alice", "name": "Alice", "platform": "android"},
            {"id": "bob", "name": "Bob", "platform": "iphone"},
            {"id": "carol", "name": "Carol", "platform": "android"},
        ],
        "beeper_url": "http://localhost:23373",
        "database": "demo.sqlite3",
        "token_file": "unused.token",
        "poll_seconds": 1,
        "media_dir": "media",
        "ai_providers": [
            {"cli": "codex", "model": "gpt-6.1-sol"},
            {"cli": "claude", "model": "sonnet"},
            {"cli": "agy", "model": "gemini-4-carbon-medium"},
        ],
    }, Path(root))


def message(text, sender="alice", mid=None, timestamp=None):
    return {"id": mid or text + ":" + sender, "chatID": "demo-group",
            "accountID": "demo-google", "senderID": sender, "text": text,
            "timestamp": timestamp or datetime.now(timezone.utc).isoformat(),
            "isSender": sender == "alice"}
