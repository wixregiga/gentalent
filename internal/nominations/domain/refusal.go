package domain

type Rule string

const (
	OneOpenNominationPerPerson         Rule = "one-open-nomination-per-person"
	CertifiedByUnanimousYes            Rule = "certified-by-unanimous-yes"
	OpenNominationsWithinGroupLimit    Rule = "open-nominations-within-group-limit"
	OpenNominationsWithinMemberLimit   Rule = "open-nominations-within-member-limit"
	NominationsPerMessageWithinLimit   Rule = "nominations-per-message-within-limit"
	MessageNominatesAllOrNone          Rule = "message-nominates-all-or-none"
	RenominatedOnlyThroughRenomination Rule = "renominated-only-through-renomination"
	RenominatedOnlyWhenRejected        Rule = "renominated-only-when-rejected"
	RenominatedOnlyWhenGroupAllows     Rule = "renominated-only-when-group-allows"
	AiRunsAtMostOncePerNominee         Rule = "ai-runs-at-most-once-per-nominee"
	VoteNamesItsNomination             Rule = "vote-names-its-nomination"
	VotesChangeUntilTheThirdVote       Rule = "votes-change-until-the-third-vote"
	RemovedReactionClearsOnlyItsVote   Rule = "removed-reaction-clears-only-its-vote"
	FinalizedOnlyWhenAllVotesAreIn     Rule = "finalized-only-when-all-votes-are-in"
	FinalizedEarlyOnlyByNominator      Rule = "finalized-early-only-by-nominator"
	ResultPostsAfterOneMinute          Rule = "result-posts-after-one-minute"
	OneShameGivenPerNominee            Rule = "one-shame-given-per-nominee"
	ShameNamesItsNomination            Rule = "shame-names-its-nomination"
	ShameOnlyMembers                   Rule = "shame-only-members"
	NoSelfShame                        Rule = "no-self-shame"
	NothingToShame                     Rule = "nothing-to-shame"
	UnknownNomination                  Rule = "unknown-nomination"
	NominationClosed                   Rule = "nomination-closed"
	NoOpenNomination                   Rule = "no-open-nomination"
)

type Refusal struct {
	Rule        Rule
	Numbers     []NominationNumber
	Ref         Ref
	Limit       int
	Room        int
	Renominable bool
}

func (r Refusal) Error() string {
	return "refused by " + string(r.Rule)
}
