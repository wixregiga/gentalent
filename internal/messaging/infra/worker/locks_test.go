package worker

import (
	"path/filepath"
	"testing"

	"gentalent/internal/messaging/domain"
)

func TestWorkerLocksAreExclusiveAcrossOpenFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	database := filepath.Join(root, "data", "messaging.sqlite3")
	group := domain.Group{ChatID: "group", AccountID: "account", Owner: "alex", Members: []string{"casey", "blair", "alex"}}
	binding := Binding(group)
	first, err := LockDatabase(database)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockDatabase(database); err == nil {
		second.Close()
		t.Fatal("database lock was shared")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := LockDatabase(database)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	live, err := LockLiveGroup(binding)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if another, err := LockLiveGroup(Binding(group)); err == nil {
		another.Close()
		t.Fatal("second live worker acquired the same group")
	}
	if DryRunDatabase(database) == database || filepath.Base(DryRunDatabase(database)) != "dryrun-messaging.sqlite3" {
		t.Fatal("preview shares live database")
	}
}

func TestBindingMatchesImportedMetadataAndLiveLock(t *testing.T) {
	group := domain.Group{ChatID: "group<&>", AccountID: "account😀", Owner: "al\x7fex", Members: []string{"casey", "blåir", "al\x7fex"}}
	const expected = "4c79fb688b13e64f6d1671fe8be1c109925df7e07fdae637de93ceeed5181183"
	if got := Binding(group); got != expected {
		t.Fatalf("binding = %s, want %s", got, expected)
	}
	group.Members[0], group.Members[2] = group.Members[2], group.Members[0]
	if got := Binding(group); got != expected {
		t.Fatalf("reordered scope binding = %s", got)
	}
}
