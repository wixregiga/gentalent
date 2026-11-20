package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"gentalent/internal/nominations/delivery/chat"
)

func gentalent(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = execute(t.Context(), args, invocation{out: &out, errs: &errs})
	return code, out.String(), errs.String()
}

func TestBusyExitsNonZeroOnlyWhileAPhotoSearchIsStarted(t *testing.T) {
	dir := t.TempDir()
	config := writeConfig(t, dir, func(values map[string]any) { values["group_open_nomination_limit"] = 2 })
	if code, out, errs := gentalent(t, "--config", config, "busy", "--live"); code != 0 || !strings.Contains(out, "No AI search") {
		t.Fatalf("D27 no database means nothing is running: %d %q %q", code, out, errs)
	}

	cfg, err := loadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	databases, err := openWorker(t.Context(), cfg, true, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer databases.close()
	message := chat.Message{ID: "message-1", Sender: blairID, Text: "!!gt Grace Hopper. Wrote the first compiler", SentAt: time.Now()}
	if err := databases.bot.Handle(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := gentalent(t, "--config", config, "busy", "--live"); code != 0 {
		t.Fatal("D27 a requested search that has not started yet is safe to restart")
	}
	if _, err := databases.bot.ClaimPhotoSearches(t.Context()); err != nil {
		t.Fatal(err)
	}
	code, _, errs := gentalent(t, "--config", config, "busy", "--live")
	if code != 1 || !strings.Contains(errs, "nominations 1") {
		t.Fatalf("D27 busy while the search for nomination 1 runs: %d %q", code, errs)
	}
	if code, _, _ := gentalent(t, "--config", config, "busy"); code != 0 {
		t.Fatal("B188 without --live busy reads the dry-run database")
	}
	who := chat.Message{ID: "message-2", Sender: caseyID, Text: "!!gtwho the woman who wrote the first computer program", SentAt: time.Now()}
	if err := databases.bot.Handle(t.Context(), who); err != nil {
		t.Fatal(err)
	}
	if _, err := databases.bot.ClaimWhoSearches(t.Context()); err != nil {
		t.Fatal(err)
	}
	code, _, errs = gentalent(t, "--config", config, "busy", "--live")
	if code != 1 || !strings.Contains(errs, "nominations 1") || !strings.Contains(errs, "who searches 1") {
		t.Fatalf("busy while a photo search and a who search run: %d %q", code, errs)
	}

	code, out, errs := gentalent(t, "--config", config, "status", "--live")
	want := `{
  "nominations": [
    {
      "number": 1,
      "person": "Grace Hopper",
      "status": "nominated",
      "votes": 1
    }
  ],
  "outbox_counts": {},
  "outbox_needing_attention": []
}
`
	if code != 0 || out != want {
		t.Fatalf("B158 status lists every Nomination while the bot holds the databases: %d %q %q", code, out, errs)
	}
}

func TestResolveSendTakesExactlyOneOutcomeAndFlagsAfterTheID(t *testing.T) {
	config := writeConfig(t, t.TempDir(), nil)
	if code, _, _ := gentalent(t, "--config", config, "resolve-send", "reply-key"); code != 2 {
		t.Fatal("B202 an outcome is required")
	}
	if code, _, _ := gentalent(t, "--config", config, "resolve-send", "reply-key", "--message-id", "m", "--not-delivered"); code != 2 {
		t.Fatal("B202 the two outcomes exclude each other")
	}
	code, _, errs := gentalent(t, "--config", config, "resolve-send", "reply-key", "--not-delivered", "--live")
	if code != 1 || !strings.Contains(errs, "no database at") {
		t.Fatalf("B157 flags after the ID parse, and a missing database is named: %d %q", code, errs)
	}
}
