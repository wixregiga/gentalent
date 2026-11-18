package chat_test

import (
	"context"
	"slices"
	"testing"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type fakePorts struct {
	t           *testing.T
	members     []domain.MemberID
	settings    domain.Settings
	nominations []domain.Nomination
	shames      []domain.Shame
	handled     map[string]bool
	outbox      []port.Reply
	relayed     map[string]bool
	feedback    []port.Feedback
	gifs        []string
}

func newFakePorts(t *testing.T, members []domain.MemberID, settings domain.Settings) *fakePorts {
	return &fakePorts{t: t, members: members, settings: settings, handled: map[string]bool{}, relayed: map[string]bool{}}
}

func (f *fakePorts) group() *domain.Group {
	f.t.Helper()
	group, err := domain.NewGroup("group-test", f.members, f.settings, f.nominations, f.shames)
	if err != nil {
		f.t.Fatalf("load the Group: %v", err)
	}
	return group
}

func (f *fakePorts) claim(messageID string) bool {
	if messageID == "" {
		return false
	}
	if f.handled[messageID] {
		return true
	}
	f.handled[messageID] = true
	return false
}

func (f *fakePorts) queue(replies []port.Reply) {
	for _, reply := range replies {
		if slices.ContainsFunc(f.outbox, func(queued port.Reply) bool { return queued.Key == reply.Key }) {
			f.t.Fatalf("reply key %s queued twice", reply.Key)
		}
		f.outbox = append(f.outbox, reply)
	}
}

func (f *fakePorts) Apply(_ context.Context, messageID string, change port.Change) error {
	if f.handled[messageID] {
		return nil
	}
	group := f.group()
	replies, err := change(group)
	if err != nil {
		return err
	}
	f.claim(messageID)
	f.nominations, f.shames = group.Nominations(), group.Shames()
	f.queue(replies)
	return nil
}

func (f *fakePorts) Answer(_ context.Context, messageID string, replies []port.Reply) error {
	if !f.claim(messageID) {
		f.queue(replies)
	}
	return nil
}

func (f *fakePorts) Log(_ context.Context, entry port.Feedback, replies []port.Reply) error {
	if !f.claim(entry.MessageID) {
		f.feedback = append(f.feedback, entry)
		f.queue(replies)
	}
	return nil
}

func (f *fakePorts) MarkRelayed(_ context.Context, keys []string) error {
	for _, key := range keys {
		f.relayed[key] = true
	}
	return nil
}

func (f *fakePorts) Unrelayed(context.Context) ([]port.Reply, error) {
	var replies []port.Reply
	for _, reply := range f.outbox {
		if !f.relayed[reply.Key] {
			replies = append(replies, reply)
		}
	}
	return replies, nil
}

func (f *fakePorts) Feedback(context.Context) ([]port.Feedback, error) {
	return f.feedback, nil
}

func (f *fakePorts) Pick(uses map[string]int) (string, error) {
	for _, gif := range f.gifs {
		if uses[gif] == 0 {
			return "/bank/" + gif, nil
		}
	}
	return "", nil
}

func (f *fakePorts) Standing(context.Context) (port.Standing, error) {
	var standing port.Standing
	latest := map[domain.PersonID]domain.Nomination{}
	var order []domain.PersonID
	for _, n := range f.nominations {
		view := port.NominationView{Number: n.Number, Person: n.Person.Name, Nominator: n.Nominator, Status: n.Status, Votes: n.Votes, LockedAt: n.LockedAt, Photos: n.Photos}
		if n.Open() {
			standing.Open = append(standing.Open, view)
		}
		if n.Photos == domain.PhotosSearching {
			standing.Searching = append(standing.Searching, n.Number)
		}
		standing.Latest = view
		if n.Open() {
			continue
		}
		if _, seen := latest[n.Person.ID]; !seen {
			order = append(order, n.Person.ID)
		}
		latest[n.Person.ID] = n
	}
	slices.SortFunc(order, func(a, b domain.PersonID) int { return int(latest[a].Number - latest[b].Number) })
	for _, id := range order {
		switch latest[id].Status {
		case domain.Certified:
			standing.Certified = append(standing.Certified, latest[id].Person.Name)
		case domain.Rejected:
			standing.Rejected = append(standing.Rejected, latest[id].Person.Name)
		}
	}
	return standing, nil
}

func (f *fakePorts) Names(context.Context) ([]port.Name, error) {
	var names []port.Name
	seen := map[domain.PersonID]bool{}
	for i := len(f.nominations) - 1; i >= 0; i-- {
		person := f.nominations[i].Person
		if !seen[person.ID] {
			seen[person.ID] = true
			names = append(names, port.Name{Alias: string(person.ID), Person: person.ID, Display: person.Name})
		}
	}
	return names, nil
}
