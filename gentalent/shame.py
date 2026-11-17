"""!gtshame: a permanent record that one member shamed another over a nominee.

Forms:
    reply to someone's message with:  !gtshame [nominee] [reason]
    or tag them:                      !gtshame @Name [nominee] [reason]

The nominee defaults to the latest one, voting or finished. Each member gives at most one
shame per nominee. A shame records the target's vote at that moment and, for a reply, the
exact message being shamed; later vote changes and verdicts never alter it.
"""
import random
import re
from typing import Optional
from .errors import RuleError
from .net import plain_text
from .nominees import leading_reference
from .replies import inline, lines, literal

COMMAND = re.compile(r"^!gtshame(?:\s+(.*))?$", re.IGNORECASE | re.DOTALL)
REASON_LIMIT = 300
QUOTE_LIMIT = 200
USAGE = "Reply to their message with !gtshame reason, or tag them: !gtshame @Name reason"


def wants_reply_context(message: dict) -> bool:
    """A reply-shame needs the message it answers, fetched before handling."""
    return bool(message.get("linkedMessageID")) and bool(COMMAND.match(plain_text(message.get("text")).strip()))


def gif_dir(config):
    return config.path("media_dir") / "shame-gifs"


def run(store, request) -> None:
    source = request.source
    body = request.arg
    shamed = None
    if source.get("linkedMessageID"):
        if "linked" not in source or source["linked"] is None:
            raise RuleError("I couldn't load the message you replied to. " + USAGE)
        shamed = source["linked"]
        if store.is_own_message(shamed):
            return  # Replies to the bot's own messages are not supported yet; stay silent.
        target_id = shamed.get("senderID")
        tagged_id, after_tag = _tagged_member(store.config, source, body)
        if tagged_id is not None:
            if tagged_id != target_id:
                raise RuleError("You replied to one person but tagged another. Reply without a tag, "
                                "or tag without replying.")
            body = after_tag
    else:
        target_id, body = _tagged_member(store.config, source, body)
        if target_id is None:
            raise RuleError(USAGE)

    roster = store.config.roster
    if target_id not in roster:
        raise RuleError("You can only shame someone in this group.")
    if target_id == request.actor.id:
        raise RuleError("You can't shame yourself.")

    reference = leading_reference(store, body)
    if reference.case is None and reference.suggestion:
        raise RuleError(f"Did you mean {reference.suggestion}? Name them exactly as the bot shows them.")
    case = reference.case or store.current()
    if case is None:
        raise RuleError("There's no nominee to shame anyone over yet.")
    if store.db.execute("SELECT 1 FROM shames WHERE case_id=? AND shamer_id=?",
                        (case["id"], request.actor.id)).fetchone():
        raise RuleError(f"You already used your shame for {case['name']}.")

    vote_row = store.db.execute("SELECT vote FROM votes WHERE case_id=? AND sender_id=?",
                                (case["id"], target_id)).fetchone()
    vote = vote_row[0] if vote_row else None
    reason = inline(reference.rest, REASON_LIMIT)
    quote = _quote(shamed) if shamed else ""
    gif = pick_gif(store)
    store.db.execute(
        "INSERT INTO shames(case_id, shamer_id, target_id, reason, target_vote, shamed_message_id, "
        "shamed_text, shamed_at, gif, message_id, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
        (case["id"], request.actor.id, target_id, reason, vote,
         shamed.get("id") if shamed else None, quote or None, shamed.get("timestamp") if shamed else None,
         gif.name if gif else "", source.get("id"), source.get("timestamp") or ""),
    )
    store.queue(
        lines(
            "🔔 SHAME 🔔",
            f"@{request.actor.name} shames @{roster[target_id]['name']} over {case['name']}.",
            f"Reason: {literal(reason)}" if reason else "",
            f'They said: "{literal(quote)}"' if quote else "",
            f"Their vote right now: {vote.upper() if vote else 'not cast'}",
            "" if gif else "(No shame GIFs in the bank yet.)",
        ),
        kind="shame",
        attachment_path=str(gif) if gif else "",
        case_id=case["id"],
    )


def _tagged_member(config, source, body):
    """Resolve "@Name ..." to a roster member, preferring the longest written name.

    Names match the roster's full name or first name, case-insensitively. If the written name
    matches nobody but Beeper tagged exactly one member (mentions), that member is used and
    the first word is taken as the tag.
    """
    if not body.startswith("@"):
        return None, body
    written = body[1:]
    candidates = []
    for member_id, member in config.roster.items():
        full = member["name"]
        for name in {full, full.split()[0]}:
            if written.casefold().startswith(name.casefold()):
                after = written[len(name):]
                if not after or not after[0].isalnum():
                    candidates.append((len(name), member_id, after))
    if candidates:
        _, member_id, after = max(candidates)
        return member_id, after.strip()
    tagged = [m for m in source.get("mentions") or [] if m in config.roster]
    if len(tagged) == 1:
        return tagged[0], written.partition(" ")[2].strip()
    return None, body


def _quote(message: dict) -> str:
    text = inline(plain_text(message.get("text")), QUOTE_LIMIT)
    if text:
        return text
    return "[photo]" if message.get("attachments") else ""


def pick_gif(store) -> Optional[object]:
    """The least-used GIF in the bank, at random among ties; None when the bank is empty."""
    folder = gif_dir(store.config)
    gifs = sorted(folder.glob("*.gif")) if folder.is_dir() else []
    if not gifs:
        return None
    uses = dict(store.db.execute("SELECT gif, COUNT(*) FROM shames GROUP BY gif").fetchall())
    fewest = min(uses.get(g.name, 0) for g in gifs)
    return random.choice([g for g in gifs if uses.get(g.name, 0) == fewest])
