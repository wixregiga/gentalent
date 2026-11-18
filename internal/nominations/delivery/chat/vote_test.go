package chat_test

import (
	"context"
	"strings"
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

func TestAVoteHandledAfterDowntimeLocksAtTheBotsClock(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!Y")
	h.now = start.Add(10 * time.Minute)
	replies := h.handle(chat.Message{Sender: string(casey.ID), Text: "!!Y", SentAt: start})
	if len(replies) != 1 || !strings.Contains(replies[0].Text, "Result in 1 minute.") {
		t.Fatalf("B65 the third vote locks, got %+v", replies)
	}
	ctx := context.Background()
	if err := h.bot.FinalizeDue(ctx, h.now.Add(59*time.Second)); err != nil || len(h.fresh()) != 0 {
		t.Fatalf("B65 B77 the minute starts when the bot applies the vote, not when it was sent: %v", err)
	}
	if err := h.bot.FinalizeDue(ctx, h.now.Add(time.Minute)); err != nil || len(h.fresh()) != 1 {
		t.Fatalf("B77 the verdict comes a minute after the bot applied the vote: %v", err)
	}
	if got := h.nomination(1).FinalizedAt; !got.Equal(h.now.Add(time.Minute)) {
		t.Fatalf("B176 a verdict is recorded at the bot's clock, got %v", got)
	}
}

func TestFinalIsRecordedAtTheBotsClock(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!Y")
	h.send(casey, "!!Y")
	h.now = start.Add(30 * time.Second)
	h.handle(chat.Message{Sender: string(alex.ID), Text: "!final", SentAt: start})
	if got := h.nomination(1).FinalizedAt; !got.Equal(h.now) {
		t.Fatalf("B176 !final records the verdict at the bot's clock, got %v", got)
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

func TestAVoteReplyingToAnAnnouncementVotesOnThatNomination(t *testing.T) {
	h := newHarness(t, severalOpen(3, 3))
	h.send(alex, "!gt Juno Vale. Pitch !gt Remy Quill. Pitch")
	replyTo := func(from chat.Member, text string, announces int) string {
		t.Helper()
		replies := h.handle(chat.Message{Sender: string(from.ID), Text: text, ReplyTo: &chat.Quoted{ID: "announcement", FromBot: true, Announces: announces}})
		if len(replies) != 1 {
			t.Fatalf("%q: want one reply, got %+v", text, replies)
		}
		return replies[0].Text
	}
	contains(t, replyTo(blair, "!!Y", 2), "@Blair has voted for Remy Quill (#2)")
	if _, voted := h.nomination(1).Votes[blair.ID]; voted {
		t.Fatal("Q7 the reply names the Nomination, so no other one is voted on")
	}
	contains(t, replyTo(casey, "!!N 1", 2), "@Casey has voted against Juno Vale (#1)")
	if _, voted := h.nomination(2).Votes[casey.ID]; voted {
		t.Fatal("Q7 a reference in the text wins over the reply")
	}
	h.send(blair, "!!Y 1")
	h.send(casey, "!!Y 1")
	h.send(alex, "!final 1")
	if got := replyTo(casey, "!!N", 1); got != "Finished already: Juno Vale (#1)." {
		t.Fatalf("Q7 a reply to a finished Nomination's announcement is refused, got %q", got)
	}
	if _, voted := h.nomination(2).Votes[casey.ID]; voted {
		t.Fatal("Q7 a vote on a finished Nomination never lands on the only open one")
	}
}

func TestAReplyToABotMessageOfUnknownNominationMustNameOne(t *testing.T) {
	h := newHarness(t, oneAtATime)
	h.send(alex, "!gt Juno Vale. Pitch")
	h.send(blair, "!!Y")
	h.send(casey, "!!Y")
	h.send(alex, "!final")
	h.send(blair, "!gt Remy Quill. Pitch")
	unknown := &chat.Quoted{ID: "unreconciled-announcement", FromBot: true}
	want := "Which Nomination? Name one, e.g. !!Y 2  \nNomination 2: Remy Quill, 1/3 votes in"
	replies := h.handle(chat.Message{Sender: string(casey.ID), Text: "!!N", ReplyTo: unknown})
	if len(replies) != 1 || replies[0].Text != want {
		t.Fatalf("Q7 a reply to a bot message of unknown Nomination is refused, got %+v", replies)
	}
	if _, voted := h.nomination(2).Votes[casey.ID]; voted {
		t.Fatal("Q7 a no meant for another Nomination never lands on the only open one")
	}
	member := &chat.Quoted{ID: "member-message", Author: string(blair.ID)}
	replies = h.handle(chat.Message{Sender: string(alex.ID), Text: "!!Y", ReplyTo: member})
	if len(replies) != 1 || !strings.Contains(replies[0].Text, "@Alex has voted for Remy Quill (#2)") {
		t.Fatalf("Q7 a reply to a Member's message keeps the one-open rule, got %+v", replies)
	}
	contains(t, h.say(casey, "!!N 2"), "@Casey has voted against Remy Quill (#2)")

	finalWant := "Which Nomination? Name one, e.g. !final 2  \nNomination 2: Remy Quill, 3/3 votes in"
	replies = h.handle(chat.Message{Sender: string(blair.ID), Text: "!final", ReplyTo: unknown})
	if len(replies) != 1 || replies[0].Text != finalWant || !h.nomination(2).Open() {
		t.Fatalf("Q7 a bare !final replying to a bot message of unknown Nomination is refused, got %+v", replies)
	}
	replies = h.handle(chat.Message{Sender: string(blair.ID), Text: "!final", ReplyTo: &chat.Quoted{ID: "announcement", FromBot: true, Announces: 2}})
	if len(replies) != 1 || !strings.Contains(replies[0].Text, "Rejected: Remy Quill (#2)") {
		t.Fatalf("Q7 a bare !final replying to an announcement finalizes that Nomination, got %+v", replies)
	}
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
