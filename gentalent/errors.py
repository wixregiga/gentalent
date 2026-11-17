class RuleError(Exception):
    """A request the rules refuse. The message is sent back to the group as the reply."""
