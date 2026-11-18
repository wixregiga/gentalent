package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, dir string, change func(map[string]any)) string {
	t.Helper()
	values := map[string]any{
		"beeper_url": "http://localhost:23373",
		"chat_id":    "group-chat", "account_id": "test-account", "owner_id": alexID,
		"members": []map[string]string{
			{"id": alexID, "name": "Alex", "platform": "android"},
			{"id": blairID, "name": "Blair", "platform": "iphone"},
			{"id": caseyID, "name": "Casey", "platform": "unknown"},
		},
		"database":     ".state/gentalent.sqlite3",
		"token_file":   "secret/beeper.token",
		"media_dir":    ".state/media",
		"ai_providers": []map[string]string{{"cli": "codex", "model": "test"}},
	}
	if change != nil {
		change(values)
	}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigResolvesPathsAgainstItsFolderAndFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	cfg, err := loadConfig(writeConfig(t, dir, func(values map[string]any) {
		values["token_file"] = "~/.config/gentalent/beeper.token"
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database != filepath.Join(dir, ".state", "gentalent.sqlite3") || cfg.MediaDir != filepath.Join(dir, ".state", "media") {
		t.Fatalf("B205 relative paths resolve against the config folder: %+v", cfg)
	}
	if cfg.TokenFile != filepath.Join(dir, "home", ".config", "gentalent", "beeper.token") {
		t.Fatalf("B205 ~ expands: %s", cfg.TokenFile)
	}
	if cfg.NominationsDatabase != filepath.Join(dir, ".state", "nominations.sqlite3") || cfg.MessagingDatabase != filepath.Join(dir, ".state", "messaging.sqlite3") {
		t.Fatalf("Go databases default beside the Python one: %s %s", cfg.NominationsDatabase, cfg.MessagingDatabase)
	}
	settings := cfg.settings()
	if settings.GroupOpenNominationLimit != 1 || settings.MemberOpenNominationLimit != 1 || settings.SingleMessageNominationLimit != 3 || settings.AllowRenomination {
		t.Fatalf("D8 limit defaults: %+v", settings)
	}
	if cfg.PhotoSearchWorkers != 3 || cfg.pollEvery() != 3*time.Second || cfg.PhotoLayout != "individual" || cfg.AnnounceReactionVotes {
		t.Fatalf("defaults: %+v", cfg)
	}
	if err := cfg.validateGroup(); err != nil {
		t.Fatalf("a complete config is valid: %v", err)
	}
}

func TestEveryInvalidSettingNamesItsKey(t *testing.T) {
	for _, tc := range []struct {
		key    string
		change func(map[string]any)
	}{
		{"chat_id", func(v map[string]any) { v["chat_id"] = "REPLACE_WITH_UNMERGED_CHAT_ID" }},
		{"account_id", func(v map[string]any) { delete(v, "account_id") }},
		{"owner_id", func(v map[string]any) { v["owner_id"] = "member-outsider" }},
		{"members", func(v map[string]any) { v["members"] = v["members"].([]map[string]string)[:2] }},
		{"members", func(v map[string]any) { v["members"].([]map[string]string)[1]["platform"] = "pager" }},
		{"members", func(v map[string]any) { v["members"].([]map[string]string)[2]["name"] = " " }},
		{"photo_layout", func(v map[string]any) { v["photo_layout"] = "grid" }},
		{"poll_seconds", func(v map[string]any) { v["poll_seconds"] = 0.5 }},
		{"announce_reaction_votes", func(v map[string]any) { v["announce_reaction_votes"] = "yes" }},
		{"media_dir", func(v map[string]any) { delete(v, "media_dir") }},
		{"photo_search_workers", func(v map[string]any) { v["photo_search_workers"] = 0 }},
		{"ai_providers", func(v map[string]any) { v["ai_providers"] = []map[string]string{{"cli": "chatbot"}} }},
		{"ai_providers", func(v map[string]any) { delete(v, "ai_providers") }},
	} {
		t.Run(tc.key, func(t *testing.T) {
			cfg, err := loadConfig(writeConfig(t, t.TempDir(), tc.change))
			if err == nil {
				err = cfg.validateGroup()
			}
			if err == nil {
				_, err = photoFinder(cfg, "")
			}
			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("D25 want an error naming %s, got %v", tc.key, err)
			}
		})
	}
}

func TestTheExampleConfigLoadsAndRefusesItsPlaceholders(t *testing.T) {
	cfg, err := loadConfig(filepath.Join("..", "..", exampleConfig))
	if err != nil {
		t.Fatalf("init copies an example that loads: %v", err)
	}
	if err := cfg.validateGroup(); err == nil || !strings.Contains(err.Error(), "chat_id") {
		t.Fatalf("B208 the example's placeholder IDs are refused: %v", err)
	}
	if cfg.settings().GroupOpenNominationLimit != 1 || cfg.PhotoSearchWorkers != 3 || len(cfg.AIProviders) != 3 {
		t.Fatalf("the example shows the defaults: %+v", cfg)
	}
}

func TestALimitBelowOneRefusesToStartAndNamesTheSetting(t *testing.T) {
	cfg := testConfig(t)
	cfg.SingleMessageNominationLimit = 0
	_, err := openWorker(t.Context(), cfg, true, time.Now)
	if err == nil || !strings.Contains(err.Error(), "single_message_nomination_limit is 0") {
		t.Fatalf("R9 nomination-limit-not-less-than-one names the setting, got %v", err)
	}
}
