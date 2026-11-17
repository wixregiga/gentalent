package sqlitedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func openTemp(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sqlite3")
	db, err := Open(context.Background(), path, ReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func exec(statement string) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, statement)
		return err
	}
}

func TestMigrateAppliesEachMigrationOnceInOrder(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)
	migrations := []Migration{
		{Name: "create", Up: exec("CREATE TABLE seen(step INTEGER)")},
		{Name: "first row", Up: exec("INSERT INTO seen VALUES (1)")},
	}
	for range 2 {
		if err := Migrate(ctx, db, migrations); err != nil {
			t.Fatal(err)
		}
	}
	migrations = append(migrations, Migration{Name: "second row", Up: exec("INSERT INTO seen VALUES (2)")})
	if err := Migrate(ctx, db, migrations); err != nil {
		t.Fatal(err)
	}
	var rows, version int
	db.QueryRow("SELECT COUNT(*) FROM seen").Scan(&rows)
	version, _ = Version(ctx, db)
	if rows != 2 || version != 3 {
		t.Fatalf("rows %d version %d, want 2 rows at version 3", rows, version)
	}
}

func TestFailedMigrationLeavesNoTrace(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)
	failing := []Migration{
		{Name: "create", Up: exec("CREATE TABLE kept(x)")},
		{Name: "half done", Up: func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "CREATE TABLE half(x)"); err != nil {
				return err
			}
			return errors.New("refused")
		}},
	}
	err := Migrate(ctx, db, failing)
	if err == nil || !strings.Contains(err.Error(), "migration 2 (half done)") {
		t.Fatalf("err = %v, want migration 2 named", err)
	}
	version, _ := Version(ctx, db)
	var half int
	db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name = 'half'").Scan(&half)
	if version != 1 || half != 0 {
		t.Fatalf("version %d, half table present %d; want version 1 and no half table", version, half)
	}
}

func TestMigrateRefusesADatabaseNewerThanTheBuild(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)
	db.Exec("PRAGMA user_version = 5")
	if err := Migrate(ctx, db, []Migration{{Name: "one", Up: exec("SELECT 1")}}); err == nil {
		t.Fatal("migrated a database newer than the build")
	}
}

func TestReadOnlyRefusesWrites(t *testing.T) {
	ctx := context.Background()
	db, path := openTemp(t)
	db.Exec("CREATE TABLE t(x)")
	reader, err := Open(ctx, path, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.Exec("INSERT INTO t VALUES (1)"); err == nil {
		t.Fatal("read-only connection accepted a write")
	}
}

func TestBackupCopiesCommittedRowsAndRefusesToOverwrite(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)
	db.Exec("CREATE TABLE ballot(vote TEXT)")
	db.Exec("INSERT INTO ballot VALUES ('yes')")
	destination := filepath.Join(t.TempDir(), "copy.sqlite3")
	if err := Backup(ctx, db, destination); err != nil {
		t.Fatal(err)
	}
	copied, err := Open(ctx, destination, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	var vote string
	if err := copied.QueryRow("SELECT vote FROM ballot").Scan(&vote); err != nil || vote != "yes" {
		t.Fatalf("copied vote %q err %v", vote, err)
	}
	if err := Backup(ctx, db, destination); err == nil {
		t.Fatal("backup overwrote an existing file")
	}
}
