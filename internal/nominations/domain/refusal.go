package domain

type Rule string

const (
	OneOpenNominationPerPerson              Rule = "one-open-nomination-per-person"
	CertifiedByUnanimousYes                 Rule = "certified-by-unanimous-yes"
	OpenNominationsWithinGroupLimit         Rule = "open-nominations-within-group-limit"
	OpenNominationsWithinMemberLimit        Rule = "open-nominations-within-member-limit"
	NominationsPerMessageWithinLimit        Rule = "nominations-per-message-within-limit"
	MessageNominatesAllOrNone               Rule = "message-nominates-all-or-none"
	RenominatedOnlyThroughRenomination      Rule = "renominated-only-through-renomination"
	RenominatedOnlyWhenRejected             Rule = "renominated-only-when-rejected"
	RenominatedOnlyWhenGroupAllows          Rule = "renominated-only-when-group-allows"
	PhotosSearchedAtMostOncePerNomination   Rule = "photos-searched-at-most-once-per-nomination"
	VoteNamesItsNomination                  Rule = "vote-names-its-nomination"
	VotesChangeUntilTheThirdVote            Rule = "votes-change-until-the-third-vote"
	RemovedReactionClearsOnlyItsVote        Rule = "removed-reaction-clears-only-its-vote"
	FinalizedOnlyWhenAllVotesAreIn          Rule = "finalized-only-when-all-votes-are-in"
	FinalizedEarlyOnlyByNominator           Rule = "finalized-early-only-by-nominator"
	FinalizedAMinuteAfterTheLastVote        Rule = "finalized-a-minute-after-the-last-vote"
	OneShamePerMemberPerNomination          Rule = "one-shame-per-member-per-nomination"
	ShameNamesItsNomination                 Rule = "shame-names-its-nomination"
	ShameOnlyMembers                        Rule = "shame-only-members"
	NoSelfShame                             Rule = "no-self-shame"
	NothingToShame                          Rule = "nothing-to-shame"
	UnknownNomination                       Rule = "unknown-nomination"
	NominationClosed                        Rule = "nomination-closed"
	NoOpenNomination                        Rule = "no-open-nomination"
	CloseSpellingNamesOnlyAnOpenNomination  Rule = "close-spelling-names-only-an-open-nomination"
	CloseSpellingNamesOneNomination         Rule = "close-spelling-names-one-nomination"
	OnePendingNameFixPerPerson              Rule = "one-pending-name-fix-per-person"
	NameFixedOnlyWhenGroupAllows            Rule = "name-fixed-only-when-group-allows"
	NameFixedOnlyWhenEnoughMembersAgree     Rule = "name-fixed-only-when-enough-members-agree"
	NameFixedOncePerPerson                  Rule = "name-fixed-once-per-person"
	NameFixNeverTakesAnotherPersonsSpelling Rule = "name-fix-never-takes-another-persons-spelling"
	EarlierSpellingsStillNameThePerson      Rule = "earlier-spellings-still-name-the-person"
	NameFixStoppedWhenAgreementIsOutOfReach Rule = "name-fix-stopped-when-agreement-is-out-of-reach"
	NameFixLapsesAfterTheNameFixWindow      Rule = "name-fix-lapses-after-the-name-fix-window"
	NameFixWithdrawnByItsProposer           Rule = "name-fix-withdrawn-by-its-proposer"
	NameUnchanged                           Rule = "name-unchanged"
	UnknownNameFix                          Rule = "unknown-name-fix"
	NameFixOver                             Rule = "name-fix-over"
)

type Refusal struct {
	Rule        Rule
	Numbers     []NominationNumber
	Ref         Ref
	Limit       int
	Room        int
	Renominable bool
	Fix         NameFixNumber
}

func (r Refusal) Error() string {
	return "refused by " + string(r.Rule)
}
