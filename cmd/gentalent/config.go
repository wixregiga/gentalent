package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	aidomain "gentalent/internal/ai/domain"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

type config struct {
	BeeperURL                    string           `json:"beeper_url"`
	ChatID                       string           `json:"chat_id"`
	AccountID                    string           `json:"account_id"`
	OwnerID                      string           `json:"owner_id"`
	Members                      []configMember   `json:"members"`
	NominationsDatabase          string           `json:"nominations_database"`
	MessagingDatabase            string           `json:"messaging_database"`
	TokenFile                    string           `json:"token_file"`
	PollSeconds                  float64          `json:"poll_seconds"`
	MediaDir                     string           `json:"media_dir"`
	PhotoLayout                  chat.PhotoLayout `json:"photo_layout"`
	AnnounceReactionVotes        bool             `json:"announce_reaction_votes"`
	AIProviders                  []configProvider `json:"ai_providers"`
	PhotoSearchWorkers           int              `json:"photo_search_workers"`
	GroupOpenNominationLimit     int              `json:"group_open_nomination_limit"`
	MemberOpenNominationLimit    int              `json:"member_open_nomination_limit"`
	SingleMessageNominationLimit int              `json:"single_message_nomination_limit"`
	AllowRenomination            bool             `json:"allow_renomination"`
}

type configMember struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type configProvider struct {
	CLI   string `json:"cli"`
	Model string `json:"model"`
}

const mostNominationsPerMessage = 10

var limitKeys = map[domain.Limit]string{
	domain.GroupOpenNominationLimit:     "group_open_nomination_limit",
	domain.MemberOpenNominationLimit:    "member_open_nomination_limit",
	domain.SingleMessageNominationLimit: "single_message_nomination_limit",
}

func loadConfig(path string) (config, error) {
	absolute, err := filepath.Abs(expandHome(path))
	if err != nil {
		return config{}, err
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	cfg := config{
		PollSeconds:                  3,
		PhotoLayout:                  chat.Individual,
		PhotoSearchWorkers:           3,
		GroupOpenNominationLimit:     1,
		MemberOpenNominationLimit:    1,
		SingleMessageNominationLimit: 3,
		NominationsDatabase:          ".state/nominations.sqlite3",
		MessagingDatabase:            ".state/messaging.sqlite3",
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		var wrongType *json.UnmarshalTypeError
		if errors.As(err, &wrongType) {
			return config{}, fmt.Errorf("%s: %s must be a JSON %s", absolute, wrongType.Field, wrongType.Value)
		}
		return config{}, fmt.Errorf("%s: %w", absolute, err)
	}
	folder := filepath.Dir(absolute)
	for _, key := range []*string{&cfg.NominationsDatabase, &cfg.MessagingDatabase, &cfg.TokenFile, &cfg.MediaDir} {
		*key = resolvePath(folder, *key)
	}
	return cfg, nil
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

func resolvePath(folder, path string) string {
	if path == "" {
		return ""
	}
	path = expandHome(path)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(folder, path)
}

func (c config) validateGroup() error {
	for _, id := range []struct{ key, value string }{{"chat_id", c.ChatID}, {"account_id", c.AccountID}, {"owner_id", c.OwnerID}} {
		if id.value == "" || strings.Contains(id.value, "REPLACE") {
			return fmt.Errorf("%s: configure the exact chat, account, and owner IDs first", id.key)
		}
	}
	ids := map[string]bool{}
	for _, member := range c.Members {
		ids[member.ID] = true
	}
	if len(c.Members) != 3 || len(ids) != 3 {
		return errors.New("members: exactly three distinct eligible sender IDs are required")
	}
	for _, member := range c.Members {
		switch {
		case member.ID == "" || strings.Contains(member.ID, "REPLACE"):
			return errors.New("members: replace the three roster sender IDs")
		case strings.TrimSpace(member.Name) == "":
			return fmt.Errorf("members: member %s needs a name", member.ID)
		case !slices.Contains([]string{"android", "iphone", "unknown"}, member.Platform):
			return errors.New("members: confirm each member's platform: android, iphone, or unknown")
		}
	}
	if !ids[c.OwnerID] {
		return errors.New("owner_id: the owner must be in the three-person roster")
	}
	if c.PhotoLayout != chat.Individual && c.PhotoLayout != chat.Stitched {
		return errors.New("photo_layout must be one of: individual, stitched")
	}
	if c.PollSeconds < 1 || c.PollSeconds > 3600 {
		return errors.New("poll_seconds must be between 1 and 3600")
	}
	if c.SingleMessageNominationLimit > mostNominationsPerMessage {
		return fmt.Errorf("single_message_nomination_limit is %d; it must be at most %d", c.SingleMessageNominationLimit, mostNominationsPerMessage)
	}
	return nil
}

func (c config) validateMedia() error {
	if c.MediaDir == "" {
		return errors.New("media_dir is required")
	}
	if c.PhotoSearchWorkers < 1 {
		return fmt.Errorf("photo_search_workers is %d; it must be at least 1", c.PhotoSearchWorkers)
	}
	return nil
}

func (c config) tokenFile() (string, error) {
	if c.TokenFile == "" {
		return "", errors.New("token_file is required")
	}
	return c.TokenFile, nil
}

func (c config) group() mdomain.Group {
	members := make([]string, len(c.Members))
	for i, member := range c.Members {
		members[i] = member.ID
	}
	return mdomain.Group{ChatID: c.ChatID, AccountID: c.AccountID, Owner: c.OwnerID, Members: members}
}

func (c config) memberIDs() []domain.MemberID {
	ids := make([]domain.MemberID, len(c.Members))
	for i, member := range c.Members {
		ids[i] = domain.MemberID(member.ID)
	}
	return ids
}

func (c config) settings() domain.Settings {
	return domain.Settings{
		GroupOpenNominationLimit:     c.GroupOpenNominationLimit,
		MemberOpenNominationLimit:    c.MemberOpenNominationLimit,
		SingleMessageNominationLimit: c.SingleMessageNominationLimit,
		AllowRenomination:            c.AllowRenomination,
	}
}

func (c config) chat(now func() time.Time) chat.Config {
	members := make([]chat.Member, len(c.Members))
	for i, member := range c.Members {
		members[i] = chat.Member{ID: domain.MemberID(member.ID), Name: member.Name}
	}
	return chat.Config{
		Members:               members,
		Settings:              c.settings(),
		PhotoLayout:           c.PhotoLayout,
		MediaDir:              c.MediaDir,
		AnnounceReactionVotes: c.AnnounceReactionVotes,
		Now:                   now,
	}
}

func (c config) providers(only string) ([]aidomain.Provider, error) {
	var providers []aidomain.Provider
	for _, provider := range c.AIProviders {
		if only == "" || provider.CLI == only {
			providers = append(providers, aidomain.Provider{CLI: provider.CLI, Model: provider.Model})
		}
	}
	if only != "" && len(providers) == 0 {
		return nil, fmt.Errorf("%s is not listed in ai_providers in config.json", only)
	}
	return providers, nil
}

func (c config) pollEvery() time.Duration {
	return time.Duration(c.PollSeconds * float64(time.Second))
}

func limitError(err error) error {
	var below domain.LimitBelowOne
	if errors.As(err, &below) {
		return fmt.Errorf("%s is %d; it must be at least 1", limitKeys[below.Limit], below.Value)
	}
	return err
}
