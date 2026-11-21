package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"gentalent/internal/messaging/domain"
)

var platformSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type PlatformStateStore struct {
	dir string
}

func NewPlatformStateStore(dir string) *PlatformStateStore {
	return &PlatformStateStore{dir: dir}
}

type platformStateRecord struct {
	Platform                 string                 `json:"platform"`
	LastKnownVersion         string                 `json:"last_known_version,omitempty"`
	LastKnownSpecVersion     string                 `json:"last_known_spec_version,omitempty"`
	LastManagedVersion       string                 `json:"last_managed_version,omitempty"`
	LastExternalDriftVersion string                 `json:"last_external_drift_version,omitempty"`
	LastCheckedAt            string                 `json:"last_checked_at,omitempty"`
	BlockedVersions          []blockedVersionRecord `json:"blocked_versions,omitempty"`
}

type blockedVersionRecord struct {
	Version         string   `json:"version"`
	AdapterRevision string   `json:"adapter_revision"`
	SpecVersion     string   `json:"spec_version,omitempty"`
	Source          string   `json:"source,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
	MarkedAt        string   `json:"marked_at,omitempty"`
}

func (s *PlatformStateStore) Load(ctx context.Context, platform string) (domain.PlatformUpdateState, error) {
	if err := ctx.Err(); err != nil {
		return domain.PlatformUpdateState{}, err
	}
	path, err := s.filePath(platform)
	if err != nil {
		return domain.PlatformUpdateState{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.PlatformUpdateState{Platform: platform}, nil
	}
	if err != nil {
		return domain.PlatformUpdateState{}, fmt.Errorf("read platform state %s: %w", path, err)
	}
	var rec platformStateRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return domain.PlatformUpdateState{}, fmt.Errorf("decode platform state %s: %w", path, err)
	}
	return fromRecord(platform, rec)
}

func (s *PlatformStateStore) Save(ctx context.Context, state domain.PlatformUpdateState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.filePath(state.Platform)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create platform state dir: %w", err)
	}
	data, err := json.MarshalIndent(toRecord(state), "", "  ")
	if err != nil {
		return fmt.Errorf("encode platform state: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, "."+state.Platform+".*.json")
	if err != nil {
		return fmt.Errorf("create temp platform state: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (s *PlatformStateStore) filePath(platform string) (string, error) {
	if !platformSlugPattern.MatchString(platform) {
		return "", fmt.Errorf("invalid platform name %q", platform)
	}
	if s.dir == "" {
		return "", errors.New("platform state directory is empty")
	}
	return filepath.Join(s.dir, platform+".json"), nil
}

func toRecord(state domain.PlatformUpdateState) platformStateRecord {
	rec := platformStateRecord{
		Platform:                 state.Platform,
		LastKnownVersion:         state.LastKnownVersion,
		LastKnownSpecVersion:     state.LastKnownSpecVersion,
		LastManagedVersion:       state.LastManagedVersion,
		LastExternalDriftVersion: state.LastExternalDriftVersion,
	}
	if !state.LastCheckedAt.IsZero() {
		rec.LastCheckedAt = state.LastCheckedAt.UTC().Format(time.RFC3339Nano)
	}
	for _, blocked := range state.BlockedVersions {
		item := blockedVersionRecord{
			Version:         blocked.Version,
			AdapterRevision: blocked.AdapterRevision,
			SpecVersion:     blocked.SpecVersion,
			Source:          blocked.Source,
			Reasons:         blocked.Reasons,
		}
		if !blocked.MarkedAt.IsZero() {
			item.MarkedAt = blocked.MarkedAt.UTC().Format(time.RFC3339Nano)
		}
		rec.BlockedVersions = append(rec.BlockedVersions, item)
	}
	return rec
}

func fromRecord(platform string, rec platformStateRecord) (domain.PlatformUpdateState, error) {
	state := domain.PlatformUpdateState{
		Platform:                 platform,
		LastKnownVersion:         rec.LastKnownVersion,
		LastKnownSpecVersion:     rec.LastKnownSpecVersion,
		LastManagedVersion:       rec.LastManagedVersion,
		LastExternalDriftVersion: rec.LastExternalDriftVersion,
	}
	if rec.Platform != "" {
		state.Platform = rec.Platform
	}
	if rec.LastCheckedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rec.LastCheckedAt)
		if err != nil {
			return domain.PlatformUpdateState{}, fmt.Errorf("parse last_checked_at: %w", err)
		}
		state.LastCheckedAt = parsed
	}
	for _, item := range rec.BlockedVersions {
		blocked := domain.BlockedVersion{
			Version:         item.Version,
			AdapterRevision: item.AdapterRevision,
			SpecVersion:     item.SpecVersion,
			Source:          item.Source,
			Reasons:         item.Reasons,
		}
		if item.MarkedAt != "" {
			parsed, err := time.Parse(time.RFC3339Nano, item.MarkedAt)
			if err != nil {
				return domain.PlatformUpdateState{}, fmt.Errorf("parse marked_at: %w", err)
			}
			blocked.MarkedAt = parsed
		}
		state.BlockedVersions = append(state.BlockedVersions, blocked)
	}
	return state, nil
}
