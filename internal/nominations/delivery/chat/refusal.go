package chat

import (
	"fmt"
	"strconv"

	"gentalent/internal/nominations/domain"
)

func (w wording) refusal(r domain.Refusal) (string, bool) {
	first := domain.NominationNumber(0)
	if len(r.Numbers) > 0 {
		first = r.Numbers[0]
	}
	members := len(w.bot.config.Members)
	switch r.Rule {
	case domain.OneOpenNominationPerPerson:
		if r.Ref.Person != "" {
			return "Named twice in this message: " + w.named(r.Ref) + ".", true
		}
		return lines("Already nominated: "+w.name(first)+", "+w.votesIn(w.nomination(first)), "Vote with !!Y or !!N."), true
	case domain.OpenNominationsWithinGroupLimit:
		if len(r.Numbers) == 1 {
			nomination := w.nomination(first)
			return fmt.Sprintf("Finish voting on %s first (%d/%d votes in). Respond with !!Y or !!N.", w.name(first), len(nomination.Votes), members), true
		}
		return sections(lines(append([]string{fmt.Sprintf("Finish voting first: the Group's limit is %d open Nominations.", r.Limit)}, w.listed(r.Numbers)...)...),
			fmt.Sprintf("Vote by number: !!Y %d or !!N %d", first, first)), true
	case domain.OpenNominationsWithinMemberLimit:
		return lines(append([]string{fmt.Sprintf("You already have %d open Nominations, your limit.", len(r.Numbers))}, w.listed(r.Numbers)...)...), true
	case domain.NominationsPerMessageWithinLimit:
		return fmt.Sprintf("At most %d people can be nominated in one message.", r.Limit), true
	case domain.MessageNominatesAllOrNone:
		return fmt.Sprintf("Only %d more can be nominated right now, so nobody was.", r.Room), true
	case domain.RenominatedOnlyThroughRenomination:
		return w.alreadyDecided(w.nomination(first), r.Renominable), true
	case domain.RenominatedOnlyWhenRejected:
		if len(r.Numbers) == 0 {
			name := w.named(r.Ref)
			return "Never nominated: " + name + ". Nominate: !gt " + name + ". Pitch", true
		}
		if nomination := w.nomination(first); nomination.Open() {
			return lines("Already nominated: "+w.name(first)+", "+w.votesIn(nomination), "Vote with !!Y or !!N."), true
		}
		return "Already a Generational Talent: " + w.name(first) + ".", true
	case domain.RenominatedOnlyWhenGroupAllows:
		return "Renomination is turned off for this Group.", true
	case domain.PhotosSearchedAtMostOncePerNomination:
		nomination := w.nomination(first)
		return fmt.Sprintf("Photos were already requested for %s (%d/%d votes in). Respond with !!Y or !!N.", w.name(first), len(nomination.Votes), members), true
	case domain.VoteNamesItsNomination, domain.CloseSpellingNamesOneNomination:
		return lines(append([]string{fmt.Sprintf("Several Nominations are open. Name one, e.g. !!Y %d", first)}, w.listed(r.Numbers)...)...), true
	case domain.VotesChangeUntilTheLastVote:
		nomination := w.nomination(first)
		return lines(fmt.Sprintf("Voting is locked: %d/%d votes are in on %s.", members, members, w.name(first)),
			"Result in under a minute, or "+w.member(nomination.Nominator)+" can send !final."), true
	case domain.FinalizedOnlyWhenAllVotesAreIn:
		return lines(append([]string{fmt.Sprintf("All %d votes must be in before finalizing.", members)}, w.listed(r.Numbers)...)...), true
	case domain.FinalizedEarlyOnlyByNominator:
		nominator := w.nomination(first).Nominator
		return fmt.Sprintf("Only %s (the initial nominator) can finalize %s early with !final.", w.member(nominator), w.name(first)), true
	case domain.OneShamePerMemberPerNomination:
		return "You already used your shame for " + w.name(first) + ".", true
	case domain.ShameNamesItsNomination:
		return lines(append([]string{fmt.Sprintf("Several Nominations are open. Name one, e.g. !gtshame @Name %d reason", first)}, w.listed(r.Numbers)...)...), true
	case domain.ShameOnlyMembers:
		return "You can only shame someone in this group.", true
	case domain.NoSelfShame:
		return "You can't shame yourself.", true
	case domain.NothingToShame:
		return "There's no nominee to shame anyone over yet.", true
	case domain.UnknownNomination:
		return noNomination(strconv.Itoa(int(r.Ref.Number))), true
	case domain.NominationClosed:
		return "Finished already: " + w.name(first) + ".", true
	case domain.NoOpenNomination:
		return "No open Nomination for " + w.person(r.Ref.Person) + ".", true
	case domain.CloseSpellingNamesOnlyAnOpenNomination:
		if len(r.Ref.Close) == 0 {
			return "", false
		}
		return "No open Nomination for " + w.person(r.Ref.Close[0]) + ".", true
	case domain.NameFixedOnlyWhenGroupAllows:
		return "Name fixes are turned off for this Group.", true
	case domain.NameFixedOncePerPerson:
		return w.name(first) + " already had their one name fix.", true
	case domain.NameFixNeverTakesAnotherPersonsSpelling:
		if len(r.Numbers) == 0 {
			return "That spelling already names someone else. Pick a different spelling.", true
		}
		return "That spelling already names " + w.name(first) + ". Pick a different spelling.", true
	case domain.OnePendingNameFixPerPerson:
		return w.pendingNameFix(r.Fix), true
	case domain.NameUnchanged:
		return w.name(first) + " is already spelled that way.", true
	case domain.WhoSearchesWithinDailyLimit:
		return w.whoLimitReached(r), true
	case domain.NoPendingNameFix:
		return "No fix is waiting for " + w.name(first) + ". Propose one: !gtfix " + strconv.Itoa(int(first)) + " Correct Name", true
	}
	return "", false
}

func (w wording) alreadyDecided(latest domain.Nomination, renominable bool) string {
	if (domain.GenerationalTalent{}).SatisfiedBy(latest.Status) {
		return "Already a Generational Talent: " + w.name(latest.Number) + "."
	}
	if renominable {
		return lines("Rejected before: "+w.name(latest.Number)+".", "Renominate: !gtrenominate "+w.person(latest.Person)+". Pitch")
	}
	return "Rejected before: " + w.name(latest.Number) + ". They can't be nominated again."
}
