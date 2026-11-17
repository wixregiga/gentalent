"""Find which nominee people mean from what they type.

People write nominees the way the bot shows them ("Sandra Bullock 👀"), squashed
("sandrabullock"), or with a typo. Matching uses the same normalized key as nominations
(letters and digits only, case-folded), so case, spaces, punctuation and emoji never matter.
A close-but-wrong spelling is never guessed: the caller gets a suggestion to confirm.
"""
from dataclasses import dataclass
from difflib import SequenceMatcher
import re
from typing import Optional
import unicodedata

# How alike two normalized names must be to suggest one. 0.85 accepts one or two typos in a
# full name ("jordanabrewstr" vs "jordanabrewster" is 0.97) and rejects unrelated words.
SUGGEST_RATIO = 0.85
# Shorter keys are too easy to confuse with ordinary words.
SUGGEST_MIN_LENGTH = 5
# Nominee references are at most this many words.
MAX_NAME_WORDS = 6


def normalized(value: str) -> str:
    """The key nominees are stored and matched by: letters and digits only, case-folded."""
    return "".join(c for c in unicodedata.normalize("NFKC", value).casefold() if c.isalnum())


@dataclass(frozen=True)
class Reference:
    case: Optional[object] = None   # the matched case row, joined with the nominee name
    rest: str = ""                  # the words after the nominee reference
    suggestion: str = ""            # a close nominee name to confirm, when nothing matched


def _case_for_key(store, key):
    return store.db.execute(
        "SELECT c.*, n.name FROM aliases a JOIN cases c ON c.nominee_id=a.nominee_id "
        "JOIN nominees n ON n.id=c.nominee_id WHERE a.alias=?",
        (key,),
    ).fetchone()


def leading_reference(store, text: str) -> Reference:
    """Match a nominee named at the start of text, using the longest run of words that matches.

    "Jordana Brewster. She's mid" and "jordanabrewster she's mid" both give Jordana
    Brewster's case with rest "She's mid". With no match, the case is None and rest is the
    whole text, plus a suggestion when the opening words are a near miss of a known nominee.
    """
    words = text.split()
    for count in range(min(len(words), MAX_NAME_WORDS), 0, -1):
        key = normalized(" ".join(words[:count]))
        if key and (case := _case_for_key(store, key)):
            rest = " ".join(words[count:]).lstrip(".:;,-–— ")
            return Reference(case=case, rest=rest)
    return Reference(rest=text.strip(), suggestion=_near_miss(store, words))


def _near_miss(store, words) -> str:
    names = store.db.execute(
        "SELECT a.alias, n.name FROM aliases a JOIN cases c ON c.nominee_id=a.nominee_id "
        "JOIN nominees n ON n.id=c.nominee_id ORDER BY c.id DESC"
    ).fetchall()
    if words:
        # "Sandra ..." or "Jordana's ...": someone's first name, which never falls back silently.
        first = normalized(re.sub(r"['’]s$", "", words[0], flags=re.IGNORECASE))
        for _, name in names:
            if len(first) >= 3 and normalized(name.split()[0]) == first:
                return name
    best, best_ratio = "", SUGGEST_RATIO
    for count in range(1, min(len(words), MAX_NAME_WORDS) + 1):
        key = normalized(" ".join(words[:count]))
        if len(key) < SUGGEST_MIN_LENGTH:
            continue
        for alias, name in names:
            ratio = SequenceMatcher(None, key, alias).ratio()
            if ratio >= best_ratio:
                best, best_ratio = name, ratio
    return best
