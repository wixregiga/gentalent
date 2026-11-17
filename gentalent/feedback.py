"""!gtfeedback / !gtsuggest: log an idea for the bot and confirm it. No AI."""
import re
from .errors import RuleError

COMMAND = re.compile(r"^!gt(feedback|suggest)(?:\s+(.*))?$", re.IGNORECASE | re.DOTALL)
TEXT_LIMIT = 2000


def run(store, request) -> None:
    word = "suggestion" if request.kind == "suggest" else "feedback"
    text = request.arg.strip()
    if not text:
        raise RuleError(f"Add your {word} after the command, e.g. !gt{request.kind} show who voted last.")
    if len(text) > TEXT_LIMIT:
        raise RuleError(f"Keep it under {TEXT_LIMIT} characters.")
    store.db.execute(
        "INSERT OR IGNORE INTO feedback(message_id, sender_id, kind, text, said_at) VALUES(?,?,?,?,?)",
        (request.source.get("id"), request.actor.id, word, text, request.source.get("timestamp") or ""),
    )
    store.queue(f"Thanks @{request.actor.name}, your {word} is logged and will be considered for a future release.")


def entries(db):
    return [dict(r) for r in db.execute(
        "SELECT said_at, sender_id, kind, text FROM feedback ORDER BY id")]
