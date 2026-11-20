package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mdomain "gentalent/internal/messaging/domain"
	msqlite "gentalent/internal/messaging/infra/sqlite"
	"gentalent/internal/shared/sqlitedb"
)

func TestBackupCopiesEachExistingDatabaseBesideIt(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, nil)
	if code, out, errs := gentalent(t, "--config", config, "backup", "--live", "--label", "before-abc1234"); code != 0 || out != "" {
		t.Fatalf("with no database yet, a backup copies nothing and succeeds: %d %q %q", code, out, errs)
	}

	cfg, err := loadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	databases, err := openWorker(t.Context(), cfg.flatGroup(), true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer databases.close()
	if err := databases.inbox.Enqueue(t.Context(), []mdomain.Outgoing{{Key: "reply-1", Text: "committed before the backup"}}); err != nil {
		t.Fatal(err)
	}

	code, out, errs := gentalent(t, "--config", config, "backup", "--live", "--label", "before-abc1234")
	if code != 0 {
		t.Fatalf("a backup while the bot holds the databases open succeeds: %d %q", code, errs)
	}
	backups := filepath.Join(dir, ".state", "backups")
	for _, name := range []string{"nominations", "messaging"} {
		matches, err := filepath.Glob(filepath.Join(backups, name+"-*-before-abc1234.sqlite3"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("one %s backup named by its label: %v %v", name, matches, err)
		}
		if !strings.Contains(out, "Backed up "+name+".sqlite3 to "+matches[0]) {
			t.Fatalf("the backup of %s is reported: %q", name, out)
		}
		info, err := os.Stat(matches[0])
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("a backup holds the ballot, so only its owner reads it: %v %v", info, err)
		}
	}

	matches, _ := filepath.Glob(filepath.Join(backups, "messaging-*-before-abc1234.sqlite3"))
	copied, err := sqlitedb.Open(t.Context(), matches[0], sqlitedb.ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	reply, err := msqlite.New(copied).Reply(t.Context(), "reply-1")
	if err != nil || reply.Text != "committed before the backup" {
		t.Fatalf("the backup holds every committed row: %+v %v", reply, err)
	}

	if code, _, errs := gentalent(t, "--config", config, "backup", "--live", "--label", "../escape"); code != 2 || !strings.Contains(errs, "--label") {
		t.Fatalf("a label that names another folder is refused: %d %q", code, errs)
	}
	if code, _, _ := gentalent(t, "--config", config, "backup", "--live"); code != 2 {
		t.Fatal("a backup without a label is refused")
	}
}
