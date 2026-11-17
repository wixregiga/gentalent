"""Reply text layout shared by every command.

Beeper converts sent text as Markdown and stores it as HTML (doc/beeper-api.md item 7).
Lines inside a section end with a Markdown hard break (two trailing spaces, invisible as
plain text); sections are separated by a blank line. No line may start with Markdown
syntax (-, *, +, #, >, "1."), so content that starts a line must not either.
"""

VOTE_KEYS = "Vote: !!Y yes · !!N no · !!C clear"


def lines(*items: str) -> str:
    """One section: the non-empty items, one per line."""
    return "  \n".join(item for item in items if item)


def sections(*blocks: str) -> str:
    """Sections separated by a blank line; empty sections are dropped."""
    return "\n\n".join(block for block in blocks if block)


MARKDOWN_SPECIALS = "\\`*_[]<>~|"


def literal(text: str) -> str:
    """Backslash-escape Markdown so user text renders as typed and cannot join lines or links."""
    return "".join("\\" + c if c in MARKDOWN_SPECIALS else c for c in text)


def inline(text: str, limit: int) -> str:
    """User-supplied text squeezed onto one line, so it cannot add lines or sections."""
    flat = " ".join((text or "").split())
    return flat if len(flat) <= limit else flat[: limit - 1].rstrip() + "…"
