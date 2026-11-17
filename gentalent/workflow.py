from dataclasses import dataclass
from datetime import datetime, timezone
import hashlib
import re
from typing import Callable, Optional


HONORIFIC_PREFIXES = {"dr", "mr", "mrs", "ms", "mx", "prof", "rev", "st", "mt", "capt", "col", "gen", "sgt"}
NAME_SUFFIXES = {"jr", "sr"}
BOT_PREFIXES = ("@gentalent-bot says:", "@gentalent says:", "🏛️ GenTalent")
LEGACY_COMMANDS = (
    "!nominate",
    "!exhibit",
    "!openvote",
    "!vote",
    "!status",
    "!summary",
    "!public",
    "!public-link",
    "!approve-public",
)
GT_SUBCOMMANDS = {
    "status": "!status",
    "summary": "!summary",
    "exhibit": "!exhibit",
    "public": "!public",
    "public-link": "!public-link",
    "approve-public": "!approve-public",
}
VOTE_ALIASES = {
    "y": "yes",
    "yes": "yes",
    "n": "no",
    "no": "no",
    "c": "clear",
    "clear": "clear",
}


@dataclass(frozen=True)
class Actor:
    id: str
    name: str
    platform: str
    is_owner: bool


@dataclass(frozen=True)
class Invocation:
    keyword: str
    body: str
    timestamp: str


@dataclass(frozen=True)
class ActionRequest:
    kind: str
    actor: Actor
    year: int
    nominee_name: str = ""
    reason: str = ""
    vote: str = ""
    command: str = ""
    arg: str = ""


@dataclass(frozen=True)
class StateCheck:
    nominee_id: str = ""
    canonical_name: str = ""
    case_id: Optional[int] = None
    votes_in: int = 0
    noop: bool = False
    prev_vote: Optional[str] = None
    upgrade_existing_case: bool = False
    nominator_name: str = ""


# Optional hook signature reserved for future background CLI classification
# (codex, agy, or claude-code) without changing the 4-step pipeline.
ClassifierHook = Callable[[Invocation, Actor], Optional[ActionRequest]]


def message_year(timestamp: str) -> int:
    if timestamp:
        try:
            return datetime.fromisoformat(timestamp.replace("Z", "+00:00")).year
        except ValueError:
            pass
    return datetime.now(timezone.utc).year


def _is_initial_token(token: str) -> bool:
    return bool(re.fullmatch(r"[A-Za-z](?:\.[A-Za-z])*", token))


def split_nominee_and_reason(raw: str) -> tuple[str, str]:
    text = raw.strip()
    if not text:
        return "", ""
    if re.match(r"^nominate\s+", text, flags=re.IGNORECASE):
        text = re.sub(r"^nominate\s+", "", text, count=1, flags=re.IGNORECASE).strip()
    if not text:
        return "", ""

    # 1. Explicit quoted nominee name: !gt "Eva Longoria" Reason...
    quoted = re.match(r'^["“]([^"”]+)["”](?:\s*[.:;,\-—–]*\s*(.*))?$', text, flags=re.DOTALL)
    if quoted:
        return quoted.group(1).strip(), (quoted.group(2) or "").strip()

    # 2. Multi-line nomination: first line is nominee, remaining lines are pitch
    if "\n" in text:
        first_line, rest = text.split("\n", 1)
        first_name, inline_reason = split_nominee_and_reason(first_line)
        combined = " ".join(part for part in (inline_reason, rest.strip()) if part)
        return first_name, combined

    # 3. Scan left-to-right for the earliest non-abbreviation sentence/clause break
    candidates: list[tuple[int, int, bool]] = []

    for sep_match in re.finditer(r"\s+[—–-]\s+|:\s+|[!?]+\s+", text):
        candidates.append((sep_match.start(), sep_match.end(), False))

    for dot_match in re.finditer(r"\.(\s+|$)", text):
        dot_pos = dot_match.start()
        prefix = text[:dot_pos].strip()
        if not prefix:
            continue
        tokens = prefix.split()
        last_word = tokens[-1].lstrip("(\"“'")
        lower_word = last_word.casefold()
        if _is_initial_token(last_word):
            continue
        if lower_word in HONORIFIC_PREFIXES:
            continue
        keep_dot = lower_word in NAME_SUFFIXES and len(tokens) > 1
        candidates.append((dot_pos, dot_match.end(), keep_dot))
        break

    if not candidates:
        return text.rstrip(".").strip(), ""

    split_start, split_end, keep_dot = min(candidates, key=lambda item: item[0])
    name_end = split_start + (1 if keep_dot else 0)
    return text[:name_end].strip(), text[split_end:].strip()


def format_nominee_name(raw_name: str) -> str:
    cleaned = " ".join(raw_name.split())
    if not cleaned:
        return ""
    all_shouted = len(cleaned) > 3 and cleaned == cleaned.upper() and any(c.isalpha() for c in cleaned)

    def format_part(part: str) -> str:
        trailing_dot = "." if part.endswith(".") else ""
        core = part[:-1] if trailing_dot else part
        if not core:
            return part
        if re.fullmatch(r"[A-Za-z](?:\.[A-Za-z])+", core):
            return core.upper() + trailing_dot
        if len(core) == 1 and core.isalpha() and trailing_dot:
            return core.upper() + "."
        if core.islower() or all_shouted:
            return core[:1].upper() + core[1:].lower() + trailing_dot
        return core + trailing_dot

    words = []
    for word in cleaned.split(" "):
        words.append("-".join(format_part(piece) for piece in word.split("-")))
    return " ".join(words)


def check_keyword(message: dict, config, is_own_outbox: bool = False) -> Optional[Invocation]:
    """Step 1: Check whether a valid roster member invoked a GenTalent keyword."""
    d = config.data
    if message.get("chatID") != d["chat_id"] or message.get("accountID") != d["account_id"]:
        return None
    sender = message.get("senderID")
    if (
        sender not in config.roster
        or not message.get("id")
        or message.get("isDeleted")
        or message.get("editedTimestamp")
        or message.get("isHidden")
        or is_own_outbox
    ):
        return None

    text = (message.get("text") or "").strip()
    if not text:
        return None
    lowered = text.casefold()
    if any(lowered.startswith(prefix.casefold()) for prefix in BOT_PREFIXES):
        return None

    timestamp = message.get("timestamp") or ""

    if re.match(r"^!{1,2}gt?help\s*$", text, flags=re.IGNORECASE):
        return Invocation(keyword="!gthelp", body="", timestamp=timestamp)

    if re.match(r"^!{1,2}gt?status\s*$", text, flags=re.IGNORECASE):
        return Invocation(keyword="!gtstatus", body="", timestamp=timestamp)

    list_match = re.match(r"^!{1,2}gt?list(?:\s+(.*))?$", text, flags=re.IGNORECASE | re.DOTALL)
    if list_match:
        return Invocation(keyword="!gtlist", body=(list_match.group(1) or "").strip(), timestamp=timestamp)

    # Check !!gt before !gt
    gen_match = re.match(r"^!!gt(?:\s+(.*))?$", text, flags=re.IGNORECASE | re.DOTALL)
    if gen_match:
        return Invocation(keyword="!!gt", body=(gen_match.group(1) or "").strip(), timestamp=timestamp)

    gt_match = re.match(r"^!gt(?:\s+(.*))?$", text, flags=re.IGNORECASE | re.DOTALL)
    if gt_match:
        return Invocation(keyword="!gt", body=(gt_match.group(1) or "").strip(), timestamp=timestamp)

    vote_match = re.match(
        r"^!!(yes|y|no|n|clear|c)(?:$|[\s!.,;:\-—–].*)$",
        text,
        flags=re.IGNORECASE | re.DOTALL,
    )
    if vote_match:
        canonical_vote = VOTE_ALIASES[vote_match.group(1).casefold()]
        return Invocation(keyword="!!" + canonical_vote, body=text, timestamp=timestamp)

    if re.match(r"^!final\s*$", text, flags=re.IGNORECASE):
        return Invocation(keyword="!final", body="", timestamp=timestamp)

    if text.startswith("!"):
        command, _, arg = text.partition(" ")
        if command.casefold() in LEGACY_COMMANDS:
            return Invocation(keyword=command.casefold(), body=arg.strip(), timestamp=timestamp)

    return None


def parse_action(
    invocation: Invocation,
    sender_id: str,
    config,
    classifier: Optional[ClassifierHook] = None,
) -> ActionRequest:
    """Step 2: Determine what action is being requested and by whom."""
    member = config.roster[sender_id]
    actor = Actor(
        id=sender_id,
        name=member["name"],
        platform=member["platform"],
        is_owner=sender_id == config.data["owner_id"],
    )
    year = message_year(invocation.timestamp)

    if classifier is not None:
        classified = classifier(invocation, actor)
        if classified is not None:
            return classified

    if invocation.keyword in ("!gt", "!!gt"):
        head, _, tail = invocation.body.partition(" ")
        head_cf = head.casefold() if head else ""
        if head_cf == "help" and not tail.strip():
            return ActionRequest(kind="help", actor=actor, year=year)
        if head_cf == "status" and not tail.strip():
            return ActionRequest(kind="gtstatus", actor=actor, year=year)
        if head_cf == "list":
            return ActionRequest(kind="gtlist", actor=actor, year=year, arg=tail.strip())
        if invocation.keyword == "!gt":
            mapped = GT_SUBCOMMANDS.get(head_cf) if head_cf else None
            if mapped and (
                mapped in ("!status", "!summary", "!approve-public") and not tail.strip()
                or mapped not in ("!status", "!summary", "!approve-public")
            ):
                return ActionRequest(kind="legacy", actor=actor, year=year, command=mapped, arg=tail.strip())
        raw_name, reason = split_nominee_and_reason(invocation.body)
        return ActionRequest(
            kind="generative_nominate" if invocation.keyword == "!!gt" else "nominate",
            actor=actor,
            year=year,
            nominee_name=format_nominee_name(raw_name),
            reason=reason,
        )

    if invocation.keyword == "!gthelp":
        return ActionRequest(kind="help", actor=actor, year=year)

    if invocation.keyword == "!gtstatus":
        return ActionRequest(kind="gtstatus", actor=actor, year=year)

    if invocation.keyword == "!gtlist":
        return ActionRequest(kind="gtlist", actor=actor, year=year, arg=invocation.body)

    if invocation.keyword in ("!!yes", "!!no", "!!clear"):
        return ActionRequest(kind="vote", actor=actor, year=year, vote=invocation.keyword[2:])

    if invocation.keyword == "!final":
        return ActionRequest(kind="finalize", actor=actor, year=year)

    return ActionRequest(
        kind="legacy",
        actor=actor,
        year=year,
        command=invocation.keyword,
        arg=invocation.body,
    )


def make_nominee_id(key: str) -> str:
    if len(key) <= 160:
        return f"nominee:{key}"
    short_hash = hashlib.sha256(key.encode()).hexdigest()[:12]
    return f"nominee:{key[:140]}-{short_hash}"


def check_state(store, request: ActionRequest, normalized_fn, rule_error_cls) -> StateCheck:
    """Step 3: Validate the requested action against the current ballot & DB state."""
    if request.kind in ("nominate", "generative_nominate"):
        prefix_hint = "!!gt" if request.kind == "generative_nominate" else "!gt"
        key = normalized_fn(request.nominee_name)
        if not request.nominee_name or not key:
            raise rule_error_cls(
                f"Include a nominee name after {prefix_hint}, e.g. {prefix_hint} Eva Longoria. Why they're a generational talent."
            )
        if len(request.nominee_name) > 80:
            raise rule_error_cls(
                f"Put a period after the nominee's name, e.g. {prefix_hint} Eva Longoria. Why they're a generational talent."
            )

        alias_row = store.db.execute(
            "SELECT nominee_id FROM aliases WHERE alias=?", (key,)
        ).fetchone()

        cur = store.current()
        if cur and cur["phase"] in ("case", "voting"):
            votes_in = store.db.execute(
                "SELECT COUNT(*) FROM votes WHERE case_id=?", (cur["id"],)
            ).fetchone()[0]
            same_nominee = normalized_fn(cur["name"]) == key or (
                alias_row and alias_row[0] == cur["nominee_id"]
            )
            if not same_nominee:
                raise rule_error_cls(
                    f"Finish voting on {cur['name']} first ({votes_in}/3 votes in). Respond with !!Y or !!N."
                )
            if request.kind == "generative_nominate" and not cur["ai_invoked"] and cur["all_votes_at"] is None:
                return StateCheck(
                    nominee_id=cur["nominee_id"],
                    canonical_name=cur["name"],
                    case_id=cur["id"],
                    votes_in=votes_in,
                    upgrade_existing_case=True,
                )
            if request.kind == "generative_nominate" and cur["ai_invoked"]:
                raise rule_error_cls(
                    f"Generative media was already created for {cur['name']} ({votes_in}/3 votes in). Respond with !!Y or !!N."
                )
            raise rule_error_cls(
                f"{cur['name']} is already nominated ({votes_in}/3 votes in). Respond with !!Y or !!N to cast your vote."
            )

        if alias_row:
            nominee_id = alias_row[0]
            nominee_row = store.db.execute(
                "SELECT name FROM nominees WHERE id=?", (nominee_id,)
            ).fetchone()
            canonical_name = nominee_row["name"] if nominee_row else request.nominee_name

            if store.db.execute("SELECT 1 FROM denials WHERE nominee_id=?", (nominee_id,)).fetchone():
                raise rule_error_cls(
                    f"{canonical_name} was previously denied and can never be nominated again."
                )
            prior = store.db.execute(
                "SELECT phase FROM cases WHERE nominee_id=?", (nominee_id,)
            ).fetchone()
            if prior:
                if prior["phase"] == "denied":
                    raise rule_error_cls(
                        f"{canonical_name} was previously denied and can never be nominated again."
                    )
                raise rule_error_cls(
                    f"{canonical_name} has already been nominated and is officially a Generational Talent!"
                )
            return StateCheck(nominee_id=nominee_id, canonical_name=canonical_name)

        return StateCheck(nominee_id=make_nominee_id(key), canonical_name=request.nominee_name)

    if request.kind == "vote":
        cur = store.current()
        if cur is None or cur["phase"] in ("certified", "denied"):
            return StateCheck(noop=True)
        if cur["phase"] == "case":
            raise rule_error_cls("Open final voting with !openvote first.")

        prev_row = store.db.execute(
            "SELECT vote FROM votes WHERE case_id=? AND sender_id=?",
            (cur["id"], request.actor.id),
        ).fetchone()
        prev_vote = prev_row[0] if prev_row else None
        messages_sent = cur["messages_sent"]

        # Silent if clearing an empty vote, OR repeating the same vote action after >= 1 message was sent
        if request.vote == "clear" and prev_vote is None:
            return StateCheck(noop=True)
        if request.vote in ("yes", "no") and prev_vote == request.vote and messages_sent >= 1:
            return StateCheck(noop=True)

        votes_in = store.db.execute(
            "SELECT COUNT(*) FROM votes WHERE case_id=?", (cur["id"],)
        ).fetchone()[0]
        nominator_name = store.config.roster[cur["nominator"]]["name"]

        if cur["all_votes_at"] is not None or votes_in >= 3:
            raise rule_error_cls(
                f"Voting is locked with 3/3 votes in on {cur['name']} — finalizing in under a minute "
                f"(or @{nominator_name} can send !final)."
            )

        return StateCheck(
            nominee_id=cur["nominee_id"],
            canonical_name=cur["name"],
            case_id=cur["id"],
            votes_in=votes_in,
            prev_vote=prev_vote,
            nominator_name=nominator_name,
        )

    if request.kind == "finalize":
        cur = store.current()
        if cur is None or cur["phase"] in ("certified", "denied"):
            return StateCheck(noop=True)
        votes_in = store.db.execute(
            "SELECT COUNT(*) FROM votes WHERE case_id=?", (cur["id"],)
        ).fetchone()[0]
        if cur["all_votes_at"] is None or votes_in < 3:
            raise rule_error_cls(
                f"All 3 votes must be in before finalizing ({votes_in}/3 votes submitted)."
            )
        if request.actor.id != cur["nominator"]:
            nominator_name = store.config.roster[cur["nominator"]]["name"]
            raise rule_error_cls(
                f"Only @{nominator_name} (the initial nominator) can finalize early with !final."
            )
        return StateCheck(
            nominee_id=cur["nominee_id"],
            canonical_name=cur["name"],
            case_id=cur["id"],
            votes_in=votes_in,
        )

    if request.kind == "gtlist":
        mode = request.arg.strip().casefold() or "winners"
        if mode not in ("winners", "losers"):
            raise rule_error_cls("Use !gtlist, !gtlist winners, or !gtlist losers.")
        return StateCheck()

    return StateCheck()


def execute_strategy(store, request: ActionRequest, check: StateCheck, normalized_fn) -> None:
    """Step 4: Execute the validated strategy and queue the resulting group reply."""
    if check.noop:
        return
    if request.kind == "nominate":
        _execute_nominate(store, request, check, normalized_fn)
        return
    if request.kind == "generative_nominate":
        _execute_generative_nominate(store, request, check, normalized_fn)
        return
    if request.kind == "vote":
        _execute_vote(store, request, check)
        return
    if request.kind == "finalize":
        store.finalize_case(check.case_id)
        return
    if request.kind == "help":
        _execute_help(store)
        return
    if request.kind == "gtstatus":
        _execute_gtstatus(store)
        return
    if request.kind == "gtlist":
        _execute_gtlist(store, request)
        return
    if request.kind == "legacy":
        store._command(request.command, request.arg, request.actor.id)
        return
    raise ValueError(f"Unknown action kind: {request.kind}")


def _create_case_and_auto_yes(store, request: ActionRequest, check: StateCheck, normalized_fn, ai_invoked: int) -> int:
    key = normalized_fn(check.canonical_name)
    id_key = normalized_fn(check.nominee_id)
    store.db.execute(
        "INSERT OR IGNORE INTO nominees(id, name) VALUES(?, ?)",
        (check.nominee_id, check.canonical_name),
    )
    store.db.executemany(
        "INSERT OR IGNORE INTO aliases(alias, nominee_id) VALUES(?, ?)",
        [(key, check.nominee_id), (id_key, check.nominee_id)],
    )
    case_id = store.db.execute(
        "INSERT INTO cases(nominee_id, nominator, phase, ai_invoked) VALUES(?, ?, 'voting', ?)",
        (check.nominee_id, request.actor.id, ai_invoked),
    ).lastrowid
    store.db.execute(
        "INSERT INTO votes(case_id, sender_id, vote) VALUES(?, ?, 'yes')",
        (case_id, request.actor.id),
    )
    if request.reason:
        store.db.execute(
            "INSERT INTO exhibits(case_id, text) VALUES(?, ?)",
            (case_id, request.reason[:2000]),
        )
    return case_id


def _nomination_prompt(actor_name: str, canonical_name: str, year: int) -> str:
    return (
        f"@{actor_name} would like to nominate {canonical_name} as a "
        f"{year} Generational Talent Nominee (3/3 YES certifies; any NO denies forever). "
        f"Vote with !!Y, !!N, or !!C (!gthelp for rules). 1/3 votes are in."
    )


def _execute_nominate(store, request: ActionRequest, check: StateCheck, normalized_fn) -> None:
    case_id = _create_case_and_auto_yes(store, request, check, normalized_fn, ai_invoked=0)
    store.queue(
        _nomination_prompt(request.actor.name, check.canonical_name, request.year),
        case_id=case_id,
    )


def _execute_generative_nominate(store, request: ActionRequest, check: StateCheck, normalized_fn) -> None:
    if check.upgrade_existing_case:
        case_id = check.case_id
        store.db.execute("UPDATE cases SET ai_invoked=1 WHERE id=?", (case_id,))
    else:
        case_id = _create_case_and_auto_yes(store, request, check, normalized_fn, ai_invoked=1)

    base_msg = (
        f"Added 3 representative photos for {check.canonical_name}. {check.votes_in}/3 votes submitted."
        if check.upgrade_existing_case
        else _nomination_prompt(request.actor.name, check.canonical_name, request.year)
    )

    try:
        bundle = store.generate_media(check.canonical_name, request.reason)
        store.db.execute(
            "UPDATE cases SET ai_artifact_dir=? WHERE id=?",
            (str(bundle.artifact_dir), case_id),
        )
        exhibit_text = (
            f"AI media ({bundle.provider_used}): {bundle.summary} "
            f"[sources: {bundle.sources_path}]"
        )[:2000]
        store.db.execute(
            "INSERT INTO exhibits(case_id, text) VALUES(?, ?)",
            (case_id, exhibit_text),
        )
        store.queue(base_msg, attachment_path=str(bundle.strip_path), case_id=case_id)
    except Exception as exc:
        fallback = (
            f"Could not generate photos for {check.canonical_name} right now ({exc}). "
            f"{check.votes_in}/3 votes submitted."
            if check.upgrade_existing_case
            else f"{base_msg} (Photo lookup unavailable.)"
        )
        store.queue(fallback[:4000], case_id=case_id)


def _execute_vote(store, request: ActionRequest, check: StateCheck) -> None:
    if request.vote == "clear":
        store.db.execute(
            "DELETE FROM votes WHERE case_id=? AND sender_id=?",
            (check.case_id, request.actor.id),
        )
        total = store.db.execute(
            "SELECT COUNT(*) FROM votes WHERE case_id=?", (check.case_id,)
        ).fetchone()[0]
        store.queue(
            f"@{request.actor.name} cleared their vote on {check.canonical_name}. {total}/3 votes submitted.",
            case_id=check.case_id,
        )
        return

    if check.prev_vote is not None:
        store.db.execute(
            "UPDATE votes SET vote=? WHERE case_id=? AND sender_id=?",
            (request.vote, check.case_id, request.actor.id),
        )
        total = store.db.execute(
            "SELECT COUNT(*) FROM votes WHERE case_id=?", (check.case_id,)
        ).fetchone()[0]
        label = "YES" if request.vote == "yes" else "NO"
        store.queue(
            f"@{request.actor.name} changed their vote to {label} on {check.canonical_name}. "
            f"{total}/3 votes submitted.",
            case_id=check.case_id,
        )
        return

    store.db.execute(
        "INSERT INTO votes(case_id, sender_id, vote) VALUES(?, ?, ?)",
        (check.case_id, request.actor.id, request.vote),
    )
    total = store.db.execute(
        "SELECT COUNT(*) FROM votes WHERE case_id=?", (check.case_id,)
    ).fetchone()[0]

    if request.vote == "yes":
        base_msg = (
            f"@{request.actor.name} has voted for {check.canonical_name} to be a generational talent! "
            f"{total}/3 votes submitted."
        )
    else:
        base_msg = (
            f"@{request.actor.name} has voted against {check.canonical_name} being a generational talent. "
            f"{total}/3 votes submitted."
        )

    if total < 3:
        store.queue(base_msg, case_id=check.case_id)
        return

    store.db.execute(
        "UPDATE cases SET all_votes_at=? WHERE id=?",
        (store.clock(), check.case_id),
    )
    store.queue(
        f"{base_msg} Finalizing in 1 minute (or @{check.nominator_name} can send !final now).",
        case_id=check.case_id,
    )


def _execute_help(store) -> None:
    store.queue(
        "Rules: 3/3 unanimous YES certifies; any NO denies permanently. Change/clear votes with !!Y, !!N, !!C "
        "until the 3rd vote locks a 1m timer (or nominator sends !final). "
        "Commands: !gt Name. Pitch | !!gt Name. Pitch (3 photos) | !!Y / !!N / !!C | !final | !gtstatus | !gtlist [winners|losers]"
    )


def _execute_gtstatus(store) -> None:
    winners_count = store.db.execute("SELECT COUNT(*) FROM cases WHERE phase='certified'").fetchone()[0]
    losers_count = store.db.execute("SELECT COUNT(*) FROM denials").fetchone()[0]
    cur = store.current()
    if cur and cur["phase"] in ("case", "voting"):
        votes = {
            r["sender_id"]: r["vote"].upper()
            for r in store.db.execute("SELECT sender_id, vote FROM votes WHERE case_id=?", (cur["id"],)).fetchall()
        }
        voted_parts = [f"@{m['name']}:{votes[mid]}" for mid, m in store.config.roster.items() if mid in votes]
        waiting_parts = [f"@{m['name']}" for mid, m in store.config.roster.items() if mid not in votes]
        if cur["all_votes_at"] is not None:
            rem = max(0, int(round(60.0 - (store.clock() - float(cur["all_votes_at"])))))
            nom_name = store.config.roster[cur["nominator"]]["name"]
            state_str = f"Active: {cur['name']} (3/3 locked: {', '.join(voted_parts)}; finalizing in ~{rem}s)"
            cmds_str = f"Commands: !final (@{nom_name}) | !gtlist | !gthelp"
        else:
            photos_tag = "photos ready" if cur["ai_invoked"] else f"!!gt {cur['name']} for photos"
            wait_str = f"; waiting on {', '.join(waiting_parts)}" if waiting_parts else ""
            state_str = (
                f"Active: {cur['name']} ({len(votes)}/3 — {', '.join(voted_parts) or 'none'}"
                f"{wait_str}; {photos_tag})"
            )
            cmds_str = "Commands: !!Y | !!N | !!C | !gtlist | !gthelp"
    else:
        last_str = f" (last: {cur['name']} — {cur['phase'].upper()})" if cur else ""
        state_str = f"No active nomination{last_str}"
        cmds_str = "Commands: !gt Name. Pitch | !!gt Name. Pitch | !gtlist [winners|losers] | !gthelp"
    store.queue(f"{state_str} · Record: {winners_count} certified, {losers_count} denied · {cmds_str}")


def _execute_gtlist(store, request: ActionRequest) -> None:
    mode = request.arg.strip().casefold() or "winners"
    cur = store.current()
    if cur and cur["phase"] in ("case", "voting"):
        votes_in = store.db.execute("SELECT COUNT(*) FROM votes WHERE case_id=?", (cur["id"],)).fetchone()[0]
        active_str = f"{cur['name']} ({votes_in}/3 votes in)"
    else:
        active_str = "none"

    if mode == "winners":
        winners = [
            r[0]
            for r in store.db.execute(
                "SELECT n.name FROM cases c JOIN nominees n ON n.id=c.nominee_id "
                "WHERE c.phase='certified' ORDER BY c.id"
            ).fetchall()
        ]
        store.queue(
            f"Winners ({len(winners)}): {', '.join(winners) if winners else 'none yet'} · "
            f"Current nominee: {active_str}"
        )
    else:
        losers = [
            r[0]
            for r in store.db.execute(
                "SELECT n.name FROM denials d JOIN nominees n ON n.id=d.nominee_id ORDER BY d.rowid"
            ).fetchall()
        ]
        store.queue(
            f"Losers ({len(losers)}): {', '.join(losers) if losers else 'none yet'} · "
            f"Current nominee: {active_str}"
        )
