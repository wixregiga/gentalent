"""👍 / 👎 on the nomination announcement count as !!Y / !!N.

Reacting creates no new message, so while a vote is open each poll re-reads the
announcement and acts only on changes since the last read (`reaction_votes` table):
a new 👍 or 👎 votes, switching changes the vote, removing it clears the vote it cast (a vote
typed since then stays). The vote then
goes through the same checks as a typed vote (locking at 3/3, nothing after the verdict).
Reading state rather than events means a reaction made while the bot was down still counts.

Reaction votes are silent unless `announce_reaction_votes` is true; the 1-minute timer
after the third vote still runs and the verdict is still posted. Rule refusals for
reactions (for example, voting is locked) are never posted.
"""
from typing import Optional
from .errors import RuleError
from .net import APIError
from .nominees import normalized
from .workflow import ActionRequest, Actor, check_state, execute_strategy

VOTES = {"👍": "yes", "👎": "no"}
# Skin-tone modifiers (U+1F3FB to U+1F3FF) and emoji/text variation selectors are dropped,
# so 👍🏿 and 👍︎ count the same as 👍.
_UNSTYLED = dict.fromkeys([*range(0x1F3FB, 0x1F400), 0xFE0E, 0xFE0F])


def vote_for(reaction_key: str) -> Optional[str]:
    return VOTES.get((reaction_key or "").translate(_UNSTYLED))


def votes_by_member(message: dict, roster) -> dict:
    """The vote each roster member's reactions express; members whose reactions conflict
    (both 👍 and 👎, possible on networks that allow several) are left out."""
    found = {}
    for reaction in message.get("reactions") or []:
        member = reaction.get("participantID")
        vote = vote_for(reaction.get("reactionKey"))
        if member in roster and vote:
            found.setdefault(member, set()).add(vote)
    return {member: votes.pop() for member, votes in found.items() if len(votes) == 1}


def announcement_id(store, case_id):
    """The delivered message ID of the case's nomination announcement: its first reply."""
    row = store.db.execute(
        "SELECT final_id FROM outbox WHERE case_id=? ORDER BY rowid LIMIT 1", (case_id,)
    ).fetchone()
    return row[0] if row else None


def sync(store, api) -> None:
    """Apply reaction changes on the open nomination. A failed read waits for the next poll."""
    case = store.current()
    if case is None or case["phase"] != "voting":
        return
    message_id = announcement_id(store, case["id"])
    if not message_id:
        return  # not delivered yet
    try:
        message = api.retrieve(message_id)
    except APIError as error:
        print(f"Reaction check skipped: {error}", flush=True)
        return
    roster = store.config.roster
    now = votes_by_member(message, roster)
    seen = dict(store.db.execute(
        "SELECT member_id, vote FROM reaction_votes WHERE case_id=?", (case["id"],)).fetchall())
    for member_id in roster:
        before, after = seen.get(member_id), now.get(member_id)
        if before == after:
            continue
        with store.db:
            store.db.execute(
                "INSERT INTO reaction_votes(case_id, member_id, vote) VALUES(?,?,?) "
                "ON CONFLICT(case_id, member_id) DO UPDATE SET vote=excluded.vote",
                (case["id"], member_id, after),
            )
            if after:
                _vote(store, member_id, after)
            elif _current_vote(store, case["id"], member_id) == before:
                # Removing a 👍 retracts the yes it cast; a vote typed since then stays.
                _vote(store, member_id, "clear")


def _current_vote(store, case_id, member_id):
    row = store.db.execute("SELECT vote FROM votes WHERE case_id=? AND sender_id=?",
                           (case_id, member_id)).fetchone()
    return row[0] if row else None


def _vote(store, member_id, vote) -> None:
    member = store.config.roster[member_id]
    request = ActionRequest(
        kind="vote",
        actor=Actor(id=member_id, name=member["name"], platform=member["platform"],
                    is_owner=member_id == store.config.data["owner_id"]),
        year=0,
        vote=vote,
        announce=store.config.announce_reaction_votes,
    )
    try:
        execute_strategy(store, request, check_state(store, request, normalized, RuleError), normalized)
    except RuleError:
        pass  # e.g. voting is locked: a reaction never gets a refusal reply
