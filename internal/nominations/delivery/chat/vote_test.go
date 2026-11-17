package chat_test

import (
	"context"
	"testing"
	"time"

	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

func TestVoteReplies(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	steps := []struct {
		from       chat.Member
		text, want string
	}{
		{blair, "!!N", "@Blair has voted against Juno Vale (#1) being a generational talent.  \n2/3 votes in."},
		{blair, "!!Y", "@Blair changed their vote to YES on Juno Vale (#1).  \n2/3 votes in."},
		{alex, "!!C", "@Alex cleared their vote on Juno Vale (#1).  \n1/3 votes in."},
		{alex, "!!Y", "@Alex has voted for Juno Vale (#1) to be a generational talent!  \n2/3 votes in."},
		{casey, "!!Y", "@Casey has voted for Juno Vale (#1) to be a generational talent!  \n3/3 votes in. Result in 1 minute.  \nFinish now: !final (@Alex)"},
		{casey, "!!N", "Voting is locked: 3/3 votes are in on Juno Vale (#1).  \nResult in under a minute, or @Alex can send !final."},
	}
	for _, step := range steps {
		if got := h.say(step.from, step.text); got != step.want {
			t.Fatalf("B59 to B65 %s %q:\ngot  %q\nwant %q", step.from.Name, step.text, got, step.want)
		}
	}
	h.silent(casey, "!!Y")
}

func TestSilentVotes(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.silent(blair, "!!Y")
	h.send(alex, "!gt Juno Vale. Pitch")
	h.silent(alex, "!!Y")
	h.silent(blair, "!!C")
	if !h.ports.handled["message-1"] {
		t.Fatal("B56 a silent vote is still recorded as handled")
	}
}

func TestTheVerdict(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!Y")
	contains(t, h.say(blair, "!final"), "All 3 votes must be in before finalizing.", "Nomination 1: Juno Vale, 2/3 votes in")
	h.send(casey, "!!Y")
	if got := h.say(casey, "!final"); got != "Only @Alex (the initial nominator) can finalize Juno Vale (#1) early with !final." {
		t.Fatalf("B82, got %q", got)
	}
	if got := h.say(alex, "!final"); got != "Congratulations Juno Vale (#1)! You are officially a Generational Talent!" {
		t.Fatalf("B83 B85, got %q", got)
	}
	h.silent(alex, "!final")
	if votes := h.nomination(1).Votes; len(votes) != 3 {
		t.Fatalf("B88 votes stay after the verdict, got %+v", votes)
	}
}

func TestTheTimerFinalizesEveryDueNominationInOneVerdict(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	h.send(alex, "!gt Juno Vale. Pitch !gt Remy Quill. Pitch")
	h.send(blair, "!!Y 1-2")
	h.send(casey, "!!Y 1 !!N 2")
	ctx := context.Background()

	if err := h.bot.FinalizeDue(ctx, start.Add(59*time.Second)); err != nil || len(h.fresh()) != 0 {
		t.Fatal("B77 nothing is due before the minute")
	}
	if err := h.bot.FinalizeDue(ctx, start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	replies := h.fresh()
	want := "Congratulations Juno Vale (#1)! You are officially a Generational Talent!  \nRejected: Remy Quill (#2) did not get a unanimous vote (2/3 yes)."
	if len(replies) != 1 || replies[0].Text != want {
		t.Fatalf("B84 B86 Q19 one verdict lists each result in number order, got %+v", replies)
	}
}

func TestADueNominationIsFinalizedBeforeTheNextMessage(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!Y")
	h.send(casey, "!!Y")
	h.now = start.Add(2 * time.Minute)
	replies := h.send(casey, "!!N")
	if len(replies) != 1 || replies[0].Key[:8] != "verdict/" {
		t.Fatalf("B78 the verdict comes first and the late vote meets a finished Nomination, got %+v", replies)
	}
}

func TestFinalByNumberAndName(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	h.send(alex, "!gt Juno Vale. Pitch !gt Remy Quill. Pitch")
	h.send(blair, "!!Y 1-2")
	h.send(casey, "!!Y 1-2")
	contains(t, h.say(alex, "!final remy quill"), "Congratulations Remy Quill (#2)!")
	if !h.nomination(1).Open() {
		t.Fatal("Q20 only the named Nomination is finalized")
	}
	contains(t, h.say(alex, "!final 2"), "Finished already: Remy Quill (#2).")
	contains(t, h.say(alex, "!final 9"), "There is no Nomination 9.")
	contains(t, h.say(alex, "!final juno vael"), "Did you mean Juno Vale?")
	contains(t, h.say(alex, "!final"), "Congratulations Juno Vale (#1)!")
}

func TestReactionsVote(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	ctx := context.Background()
	open, err := h.bot.OpenAnnouncements(ctx)
	if err != nil || len(open) != 1 || open[0] != 1 {
		t.Fatalf("B68 the open Nomination's announcement counts, got %v, %v", open, err)
	}
	count := func(reactions map[string][]string) {
		t.Helper()
		if err := h.bot.CountReactions(ctx, 1, reactions); err != nil {
			t.Fatal(err)
		}
	}
	count(map[string][]string{string(blair.ID): {"👍🏿"}, string(casey.ID): {"thumbsup", "🎉"}, "outsider": {"👍"}})
	got := h.nomination(1).Votes
	if got[blair.ID] != domain.Yes || len(got) != 2 || len(h.fresh()) != 0 {
		t.Fatalf("B66 B67 B72 any skin tone counts, other keys and outsiders do not, silently, got %+v", got)
	}
	count(map[string][]string{string(blair.ID): {"👍", "👎️"}})
	if _, voted := h.nomination(1).Votes[blair.ID]; voted {
		t.Fatal("B67 a Member with both a yes and a no counts as none, so the yes is cleared")
	}
}

func TestReactionVotesAreAnnouncedWhenConfigured(t *testing.T) {
	h := newHarness(t, oneAtATime, func(c *chat.Config) { c.AnnounceReactionVotes = true })
	h.send(alex, "!gt Juno Vale. Pitch")
	if err := h.bot.CountReactions(context.Background(), 1, map[string][]string{string(blair.ID): {"👎"}}); err != nil {
		t.Fatal(err)
	}
	replies := h.fresh()
	if len(replies) != 1 || replies[0].Text != "@Blair has voted against Juno Vale (#1) being a generational talent.  \n2/3 votes in." {
		t.Fatalf("B72 the typed-vote text, got %+v", replies)
	}
}

func TestRelaying(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	ctx := context.Background()
	replies, err := h.bot.Unrelayed(ctx)
	if err != nil || len(replies) != 1 || replies[0].Announces != 1 {
		t.Fatalf("the outbox holds the announcement, got %+v, %v", replies, err)
	}
	if err := h.bot.MarkRelayed(ctx, []string{replies[0].Key}); err != nil {
		t.Fatal(err)
	}
	if again, _ := h.bot.Unrelayed(ctx); len(again) != 0 {
		t.Fatalf("a relayed reply is not relayed again, got %+v", again)
	}
	h.handle(chat.Message{ID: "message-1", Sender: string(alex.ID), Text: "!gthelp"})
	if len(h.ports.outbox) != 1 {
		t.Fatal("a handled message ID does nothing")
	}
}
