package beeper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretsArePrivateAndAtomicallyReplaced(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets")
	path := filepath.Join(dir, "token")
	if err := WriteToken(path, "   "); err == nil {
		t.Fatal("wrote empty token")
	}
	if err := WriteToken(path, "  first-token\n"); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := WriteToken(path, "second-token"); err != nil {
		t.Fatal(err)
	}
	oldInfo, err := old.Stat()
	if err != nil {
		t.Fatal(err)
	}
	newInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldInfo, newInfo) || newInfo.Mode().Perm() != 0600 || dirInfo.Mode().Perm() != 0700 {
		t.Fatal("secret was overwritten in place or exposed through permissions")
	}
	if token, err := ReadToken(path); err != nil || token != "second-token" {
		t.Fatalf("read failed: %v", err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadToken(path); err == nil {
		t.Fatal("accepted a group-readable token")
	}
}
