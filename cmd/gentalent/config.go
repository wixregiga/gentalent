package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	aidomain "gentalent/internal/ai/domain"
	mdomain "gentalent/internal/messaging/domain"
	"gentalent/internal/nominations/delivery/chat"
	"gentalent/internal/nominations/domain"
)

type config struct {
	BeeperURL                    string                `json:"beeper_url"`
	ChatID                       string                `json:"chat_id"`
	AccountID                    string                `json:"account_id"`
	OwnerID                      string                `json:"owner_id"`
	Members                      []configMember        `json:"members"`
	Groups                       []configGroupOverride `json:"groups"`
	NominationsDatabase          string                `json:"nominations_database"`
	MessagingDatabase            string                `json:"messaging_database"`
	TokenFile                    string                `json:"token_file"`
	PollSeconds                  float64               `json:"poll_seconds"`
	MediaDir                     string                `json:"media_dir"`
	PhotoLayout                  chat.PhotoLayout      `json:"photo_layout"`
	AnnounceReactionVotes        bool                  `json:"announce_reaction_votes"`
	AIProviders                  []configProvider      `json:"ai_providers"`
	PhotoSearchWorkers           int                   `json:"photo_search_workers"`
	GroupOpenNominationLimit     int                   `json:"group_open_nomination_limit"`
	MemberOpenNominationLimit    int                   `json:"member_open_nomination_limit"`
	SingleMessageNominationLimit int                   `json:"single_message_nomination_limit"`
	AllowRenomination            bool                  `json:"allow_renomination"`
	AllowNameFixes               bool                  `json:"allow_name_fixes"`
	NameFixAgreementPercent      int                   `json:"name_fix_agreement_percent"`
	NameFixHours                 int                   `json:"name_fix_hours"`
	WhoSearchDailyLimit          int                   `json:"who_search_daily_limit"`
	folder                       string
}

type configGroupOverride struct {
	Name                         string            `json:"name"`
	ChatID                       string            `json:"chat_id"`
	AccountID                    string            `json:"account_id"`
	OwnerID                      string            `json:"owner_id"`
	Members                      []configMember    `json:"members"`
	NominationsDatabase          string            `json:"nominations_database"`
	MessagingDatabase            string            `json:"messaging_database"`
	MediaDir                     string            `json:"media_dir"`
	PhotoLayout                  *chat.PhotoLayout `json:"photo_layout"`
	AnnounceReactionVotes        *bool             `json:"announce_reaction_votes"`
	GroupOpenNominationLimit     *int              `json:"group_open_nomination_limit"`
	MemberOpenNominationLimit    *int              `json:"member_open_nomination_limit"`
	SingleMessageNominationLimit *int              `json:"single_message_nomination_limit"`
	AllowRenomination            *bool             `json:"allow_renomination"`
	AllowNameFixes               *bool             `json:"allow_name_fixes"`
	NameFixAgreementPercent      *int              `json:"name_fix_agreement_percent"`
	NameFixHours                 *int              `json:"name_fix_hours"`
	WhoSearchDailyLimit          *int              `json:"who_search_daily_limit"`
}

type groupConfig struct {
	Name                         string
	BeeperURL                    string
	TokenFile                    string
	ChatID                       string
	AccountID                    string
	OwnerID                      string
	Members                      []configMember
	NominationsDatabase          string
	MessagingDatabase            string
	MediaDir                     string
	GifDir                       string
	PollSeconds                  float64
	PhotoLayout                  chat.PhotoLayout
	AnnounceReactionVotes        bool
	PhotoSearchWorkers           int
	GroupOpenNominationLimit     int
	MemberOpenNominationLimit    int
	SingleMessageNominationLimit int
	AllowRenomination            bool
	AllowNameFixes               bool
	NameFixAgreementPercent      int
	NameFixHours                 int
	WhoSearchDailyLimit          int
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

var groupNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

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
		NameFixAgreementPercent:      100,
		NameFixHours:                 24,
		WhoSearchDailyLimit:          5,
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		var wrongType *json.UnmarshalTypeError
		if errors.As(err, &wrongType) {
			return config{}, fmt.Errorf("%s: %s must be a JSON %s", absolute, wrongType.Field, wrongType.Value)
		}
		return config{}, fmt.Errorf("%s: %w", absolute, err)
	}
	cfg.folder = filepath.Dir(absolute)
	if cfg.Groups == nil {
		if cfg.NominationsDatabase == "" {
			cfg.NominationsDatabase = ".state/nominations.sqlite3"
		}
		if cfg.MessagingDatabase == "" {
			cfg.MessagingDatabase = ".state/messaging.sqlite3"
		}
	}
	for _, key := range []*string{&cfg.NominationsDatabase, &cfg.MessagingDatabase, &cfg.TokenFile, &cfg.MediaDir} {
		*key = resolvePath(cfg.folder, *key)
	}
	for i := range cfg.Groups {
		for _, key := range []*string{&cfg.Groups[i].NominationsDatabase, &cfg.Groups[i].MessagingDatabase, &cfg.Groups[i].MediaDir} {
			*key = resolvePath(cfg.folder, *key)
		}
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
	if folder == "" {
		return path
	}
	return filepath.Join(folder, path)
}

func orDefault[T any](override *T, fallback T) T {
	if override != nil {
		return *override
	}
	return fallback
}

func prefixGroupError(name string, err error) error {
	if name == "" || err == nil {
		return err
	}
	return fmt.Errorf("group %s: %w", name, err)
}

func (c config) flatGroup() groupConfig {
	gifDir := ""
	if c.MediaDir != "" {
		gifDir = filepath.Join(c.MediaDir, "shame-gifs")
	}
	return groupConfig{
		BeeperURL:                    c.BeeperURL,
		TokenFile:                    c.TokenFile,
		ChatID:                       c.ChatID,
		AccountID:                    c.AccountID,
		OwnerID:                      c.OwnerID,
		Members:                      c.Members,
		NominationsDatabase:          c.NominationsDatabase,
		MessagingDatabase:            c.MessagingDatabase,
		MediaDir:                     c.MediaDir,
		GifDir:                       gifDir,
		PollSeconds:                  c.PollSeconds,
		PhotoLayout:                  c.PhotoLayout,
		AnnounceReactionVotes:        c.AnnounceReactionVotes,
		PhotoSearchWorkers:           c.PhotoSearchWorkers,
		GroupOpenNominationLimit:     c.GroupOpenNominationLimit,
		MemberOpenNominationLimit:    c.MemberOpenNominationLimit,
		SingleMessageNominationLimit: c.SingleMessageNominationLimit,
		AllowRenomination:            c.AllowRenomination,
		AllowNameFixes:               c.AllowNameFixes,
		NameFixAgreementPercent:      c.NameFixAgreementPercent,
		NameFixHours:                 c.NameFixHours,
		WhoSearchDailyLimit:          c.WhoSearchDailyLimit,
	}
}

func (c config) resolveGroups() ([]groupConfig, error) {
	if c.Groups == nil {
		return []groupConfig{c.flatGroup()}, nil
	}
	if len(c.Groups) == 0 {
		return nil, errors.New("groups: configure at least one group")
	}
	if c.ChatID != "" || len(c.Members) > 0 {
		return nil, errors.New("groups: move chat_id and members into groups entries")
	}
	seenNames := map[string]bool{}
	seenChats := map[string]bool{}
	seenDBs := map[string]string{}
	resolved := make([]groupConfig, 0, len(c.Groups))
	for i, g := range c.Groups {
		if !groupNamePattern.MatchString(g.Name) {
			return nil, fmt.Errorf("groups[%d].name %q must use letters, digits, '_' or '-', starting with a letter or digit", i, g.Name)
		}
		lower := strings.ToLower(g.Name)
		if seenNames[lower] {
			return nil, fmt.Errorf("groups: duplicate group name %q", g.Name)
		}
		seenNames[lower] = true
		if g.ChatID != "" && !strings.Contains(g.ChatID, "REPLACE") {
			if seenChats[g.ChatID] {
				return nil, fmt.Errorf("groups: duplicate chat_id %q", g.ChatID)
			}
			seenChats[g.ChatID] = true
		}
		nomDB := g.NominationsDatabase
		if nomDB == "" {
			nomDB = resolvePath(c.folder, filepath.Join(".state", "groups", g.Name, "nominations.sqlite3"))
		}
		msgDB := g.MessagingDatabase
		if msgDB == "" {
			msgDB = resolvePath(c.folder, filepath.Join(".state", "groups", g.Name, "messaging.sqlite3"))
		}
		cleanNom, cleanMsg := filepath.Clean(nomDB), filepath.Clean(msgDB)
		if cleanNom == cleanMsg {
			return nil, prefixGroupError(g.Name, fmt.Errorf("nominations_database and messaging_database must be distinct (%s)", cleanNom))
		}
		for _, path := range []string{cleanNom, cleanMsg} {
			if other, taken := seenDBs[path]; taken {
				return nil, fmt.Errorf("groups: database path %s is used by both %s and %s", path, other, g.Name)
			}
			seenDBs[path] = g.Name
		}
		accountID := g.AccountID
		if accountID == "" {
			accountID = c.AccountID
		}
		ownerID := g.OwnerID
		if ownerID == "" {
			ownerID = c.OwnerID
		}
		mediaDir := g.MediaDir
		if mediaDir == "" && c.MediaDir != "" {
			mediaDir = filepath.Join(c.MediaDir, "groups", g.Name)
		}
		gifDir := ""
		if c.MediaDir != "" {
			gifDir = filepath.Join(c.MediaDir, "shame-gifs")
		} else if mediaDir != "" {
			gifDir = filepath.Join(mediaDir, "shame-gifs")
		}
		resolved = append(resolved, groupConfig{
			Name:                         g.Name,
			BeeperURL:                    c.BeeperURL,
			TokenFile:                    c.TokenFile,
			ChatID:                       g.ChatID,
			AccountID:                    accountID,
			OwnerID:                      ownerID,
			Members:                      g.Members,
			NominationsDatabase:          nomDB,
			MessagingDatabase:            msgDB,
			MediaDir:                     mediaDir,
			GifDir:                       gifDir,
			PollSeconds:                  c.PollSeconds,
			PhotoLayout:                  orDefault(g.PhotoLayout, c.PhotoLayout),
			AnnounceReactionVotes:        orDefault(g.AnnounceReactionVotes, c.AnnounceReactionVotes),
			PhotoSearchWorkers:           c.PhotoSearchWorkers,
			GroupOpenNominationLimit:     orDefault(g.GroupOpenNominationLimit, c.GroupOpenNominationLimit),
			MemberOpenNominationLimit:    orDefault(g.MemberOpenNominationLimit, c.MemberOpenNominationLimit),
			SingleMessageNominationLimit: orDefault(g.SingleMessageNominationLimit, c.SingleMessageNominationLimit),
			AllowRenomination:            orDefault(g.AllowRenomination, c.AllowRenomination),
			AllowNameFixes:               orDefault(g.AllowNameFixes, c.AllowNameFixes),
			NameFixAgreementPercent:      orDefault(g.NameFixAgreementPercent, c.NameFixAgreementPercent),
			NameFixHours:                 orDefault(g.NameFixHours, c.NameFixHours),
			WhoSearchDailyLimit:          orDefault(g.WhoSearchDailyLimit, c.WhoSearchDailyLimit),
		})
	}
	return resolved, nil
}

func (c config) selectResolvedGroups(name string) ([]groupConfig, bool, error) {
	groups, err := c.resolveGroups()
	if err != nil {
		return nil, false, err
	}
	if name == "" {
		return groups, len(groups) > 1, nil
	}
	for _, g := range groups {
		if g.Name == name {
			return []groupConfig{g}, false, nil
		}
	}
	return nil, false, fmt.Errorf("unknown group %q", name)
}

func (c config) validateGroups() ([]groupConfig, error) {
	groups, err := c.resolveGroups()
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if err := g.validateGroup(); err != nil {
			return nil, prefixGroupError(g.Name, err)
		}
	}
	return groups, nil
}

func (c config) selectGroups(name string) ([]groupConfig, bool, error) {
	groups, multi, err := c.selectResolvedGroups(name)
	if err != nil {
		return nil, false, err
	}
	for _, g := range groups {
		if err := g.validateGroup(); err != nil {
			return nil, false, prefixGroupError(g.Name, err)
		}
	}
	return groups, multi, nil
}

func (c config) validateGroup() error {
	_, err := c.validateGroups()
	return err
}

func (c config) validateMedia() error {
	return c.flatGroup().validateMedia()
}

func (c config) validateAllMedia(groups []groupConfig) error {
	if c.PhotoSearchWorkers < 1 {
		return fmt.Errorf("photo_search_workers is %d; it must be at least 1", c.PhotoSearchWorkers)
	}
	for _, g := range groups {
		if err := g.validateMedia(); err != nil {
			return prefixGroupError(g.Name, err)
		}
	}
	return nil
}

func (g groupConfig) validateGroup() error {
	for _, id := range []struct{ key, value string }{{"chat_id", g.ChatID}, {"account_id", g.AccountID}, {"owner_id", g.OwnerID}} {
		if id.value == "" || strings.Contains(id.value, "REPLACE") {
			return fmt.Errorf("%s: configure the exact chat, account, and owner IDs first", id.key)
		}
	}
	ids := map[string]bool{}
	for _, member := range g.Members {
		ids[member.ID] = true
	}
	if len(g.Members) < 2 || len(ids) != len(g.Members) {
		return errors.New("members: at least two distinct eligible sender IDs are required")
	}
	for _, member := range g.Members {
		switch {
		case member.ID == "" || strings.Contains(member.ID, "REPLACE"):
			return errors.New("members: replace the placeholder roster sender IDs")
		case strings.TrimSpace(member.Name) == "":
			return fmt.Errorf("members: member %s needs a name", member.ID)
		case !slices.Contains([]string{"android", "iphone", "unknown"}, member.Platform):
			return errors.New("members: confirm each member's platform: android, iphone, or unknown")
		}
	}
	if !ids[g.OwnerID] {
		return errors.New("owner_id: the owner must be in the roster")
	}
	if g.PhotoLayout != chat.Individual && g.PhotoLayout != chat.Stitched {
		return errors.New("photo_layout must be one of: individual, stitched")
	}
	if g.PollSeconds < 1 || g.PollSeconds > 3600 {
		return errors.New("poll_seconds must be between 1 and 3600")
	}
	if g.SingleMessageNominationLimit > mostNominationsPerMessage {
		return fmt.Errorf("single_message_nomination_limit is %d; it must be at most %d", g.SingleMessageNominationLimit, mostNominationsPerMessage)
	}
	return nil
}

func (g groupConfig) validateMedia() error {
	if g.MediaDir == "" {
		return errors.New("media_dir is required")
	}
	if g.PhotoSearchWorkers < 1 {
		return fmt.Errorf("photo_search_workers is %d; it must be at least 1", g.PhotoSearchWorkers)
	}
	if g.WhoSearchDailyLimit < 0 {
		return fmt.Errorf("who_search_daily_limit is %d; it must be 0 or more", g.WhoSearchDailyLimit)
	}
	return nil
}

func (c config) tokenFile() (string, error) {
	if c.TokenFile == "" {
		return "", errors.New("token_file is required")
	}
	return c.TokenFile, nil
}

func (c config) platformStateDir() string {
	if dir := strings.TrimSpace(os.Getenv("GENTALENT_PLATFORM_STATE_DIR")); dir != "" {
		return expandHome(dir)
	}
	if c.folder == "" {
		return filepath.Join(".state", "platforms")
	}
	return filepath.Join(c.folder, ".state", "platforms")
}

func (c config) group() mdomain.Group {
	return c.flatGroup().group()
}

func (g groupConfig) group() mdomain.Group {
	members := make([]string, len(g.Members))
	for i, member := range g.Members {
		members[i] = member.ID
	}
	return mdomain.Group{ChatID: g.ChatID, AccountID: g.AccountID, Owner: g.OwnerID, Members: members}
}

func (g groupConfig) memberIDs() []domain.MemberID {
	ids := make([]domain.MemberID, len(g.Members))
	for i, member := range g.Members {
		ids[i] = domain.MemberID(member.ID)
	}
	return ids
}

func (c config) settings() domain.Settings {
	return c.flatGroup().settings()
}

func (g groupConfig) settings() domain.Settings {
	return domain.Settings{
		GroupOpenNominationLimit:     g.GroupOpenNominationLimit,
		MemberOpenNominationLimit:    g.MemberOpenNominationLimit,
		SingleMessageNominationLimit: g.SingleMessageNominationLimit,
		AllowRenomination:            g.AllowRenomination,
		AllowNameFixes:               g.AllowNameFixes,
		NameFixAgreementPercentage:   g.NameFixAgreementPercent,
		NameFixWindow:                time.Duration(g.NameFixHours) * time.Hour,
		WhoSearchDailyLimit:          g.WhoSearchDailyLimit,
	}
}

func (g groupConfig) chat(now func() time.Time) chat.Config {
	members := make([]chat.Member, len(g.Members))
	for i, member := range g.Members {
		members[i] = chat.Member{ID: domain.MemberID(member.ID), Name: member.Name}
	}
	return chat.Config{
		Members:               members,
		Settings:              g.settings(),
		PhotoLayout:           g.PhotoLayout,
		MediaDir:              g.MediaDir,
		AnnounceReactionVotes: g.AnnounceReactionVotes,
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

func (g groupConfig) pollEvery() time.Duration {
	return time.Duration(g.PollSeconds * float64(time.Second))
}

func (c config) pollEvery() time.Duration {
	return c.flatGroup().pollEvery()
}

func settingsError(err error) error {
	var refused domain.Refusal
	if errors.As(err, &refused) && refused.Rule == domain.GroupHasAtLeastTwoMembers {
		return errors.New("members: at least two distinct eligible sender IDs are required")
	}
	var below domain.LimitBelowOne
	if errors.As(err, &below) {
		return fmt.Errorf("%s is %d; it must be at least 1", limitKeys[below.Limit], below.Value)
	}
	var percentage domain.AgreementPercentageOutOfRange
	if errors.As(err, &percentage) {
		return fmt.Errorf("name_fix_agreement_percent is %d; it must be more than 0 and at most 100", percentage.Value)
	}
	var window domain.NameFixWindowBelowAnHour
	if errors.As(err, &window) {
		return fmt.Errorf("name_fix_hours is %d; it must be at least 1", int(window.Value/time.Hour))
	}
	return err
}
