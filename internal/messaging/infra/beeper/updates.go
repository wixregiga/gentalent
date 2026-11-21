package beeper

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/beeper/desktop-api-go/v6/option"

	"gentalent/internal/messaging/domain"
)

const (
	PlatformName        = "beeper"
	AdapterRevision     = "beeper-api-v5-r1"
	ExpectedSpecMajor   = 5
	ExpectedSpecVersion = "5.0.0"
)

type UpdateObserver struct {
	client   *Client
	cacheDir string
	now      func() time.Time
}

func NewUpdateObserver(client *Client, cacheDir string) *UpdateObserver {
	return &UpdateObserver{
		client:   client,
		cacheDir: cacheDir,
		now:      time.Now,
	}
}

func DefaultUpdaterCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "beepertexts-updater", "pending")
}

func (o *UpdateObserver) PlatformName() string        { return PlatformName }
func (o *UpdateObserver) AdapterRevision() string     { return AdapterRevision }
func (o *UpdateObserver) ExpectedSpecMajor() int      { return ExpectedSpecMajor }
func (o *UpdateObserver) ExpectedSpecVersion() string { return ExpectedSpecVersion }

func (o *UpdateObserver) ObservePlatform(ctx context.Context) (domain.PlatformObservation, error) {
	obs := domain.PlatformObservation{
		Platform:   PlatformName,
		ObservedAt: o.observedAt(),
	}
	pending, err := readPendingUpdate(o.cacheDir)
	if err != nil {
		return domain.PlatformObservation{}, err
	}
	obs.Pending = pending

	var rawResp *http.Response
	info, err := o.client.api.Info.Get(ctx, option.WithResponseInto(&rawResp))
	if err != nil || info == nil {
		obs.ConnectionError = apiError(err).Error()
		obs.HeaderVersion = o.client.LastObservedVersion()
		return obs, nil
	}
	if rawResp != nil {
		obs.HeaderVersion = strings.TrimSpace(rawResp.Header.Get("X-Beeper-Desktop-Version"))
	}
	if obs.HeaderVersion == "" {
		obs.HeaderVersion = o.client.LastObservedVersion()
	}
	obs.CurrentVersion = strings.TrimSpace(info.App.Version)
	if obs.CurrentVersion == "" {
		obs.CurrentVersion = obs.HeaderVersion
	}

	specVersion, issues, err := o.client.inspectSpec(ctx)
	if err != nil {
		obs.ConnectionError = err.Error()
		return obs, nil
	}
	obs.SpecVersion = specVersion
	obs.ContractIssues = issues
	return obs, nil
}

func (o *UpdateObserver) observedAt() time.Time {
	if o.now != nil {
		return o.now().UTC()
	}
	return time.Now().UTC()
}

type updaterMetadata struct {
	Version  string `json:"version"`
	FileName string `json:"fileName"`
	SHA512   string `json:"sha512"`
}

func readPendingUpdate(cacheDir string) (*domain.PendingUpdate, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return nil, nil
	}
	metaPath := filepath.Join(cacheDir, "update-info.json")
	data, err := os.ReadFile(metaPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return &domain.PendingUpdate{
			Source:            domain.PendingSourceCache,
			VerificationError: fmt.Sprintf("read updater metadata: %v", err),
		}, nil
	}

	var meta updaterMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return &domain.PendingUpdate{
			Source:            domain.PendingSourceCache,
			VerificationError: fmt.Sprintf("parse updater metadata: %v", err),
		}, nil
	}

	pending := &domain.PendingUpdate{
		Version:  strings.TrimSpace(meta.Version),
		FileName: strings.TrimSpace(meta.FileName),
		SHA512:   strings.TrimSpace(meta.SHA512),
		Source:   domain.PendingSourceCache,
	}
	if pending.Version == "" {
		pending.Version = ParseAppImageVersion(pending.FileName)
	}
	if !safeFileName(pending.FileName) {
		pending.VerificationError = fmt.Sprintf("pending update metadata has invalid fileName %q", meta.FileName)
		return pending, nil
	}

	candidatePath := filepath.Join(cacheDir, pending.FileName)
	pending.FilePath = candidatePath
	if pending.Version == "" {
		pending.VerificationError = fmt.Sprintf("could not extract version from pending update %q", pending.FileName)
		return pending, nil
	}
	if pending.SHA512 == "" {
		pending.VerificationError = fmt.Sprintf("pending update %s metadata is missing sha512", pending.FileName)
		return pending, nil
	}
	if err := verifyFileSHA512(candidatePath, pending.SHA512); err != nil {
		pending.VerificationError = err.Error()
		return pending, nil
	}
	pending.Verified = true
	return pending, nil
}

func ParseAppImageVersion(fileName string) string {
	base := filepath.Base(strings.TrimSpace(fileName))
	if !strings.HasPrefix(base, "Beeper-") || !strings.HasSuffix(base, ".AppImage") {
		return ""
	}
	trimmed := strings.TrimSuffix(strings.TrimPrefix(base, "Beeper-"), ".AppImage")
	for _, archSuffix := range []string{"-x86_64", "-aarch64", "-arm64", "-amd64", "-x64"} {
		if strings.HasSuffix(trimmed, archSuffix) {
			return strings.TrimSuffix(trimmed, archSuffix)
		}
	}
	return trimmed
}

func safeFileName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return filepath.Base(name) == name
}

func verifyFileSHA512(path, expected string) error {
	expectedBytes, err := decodeSHA512(expected)
	if err != nil {
		return fmt.Errorf("pending update sha512 is invalid: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open pending update %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	hasher := sha512.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("hash pending update %s: %w", filepath.Base(path), err)
	}
	actual := hasher.Sum(nil)
	if !bytes.Equal(actual, expectedBytes) {
		return fmt.Errorf("pending update %s failed SHA-512 verification", filepath.Base(path))
	}
	return nil
}

func decodeSHA512(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == sha512.Size*2 {
		if decoded, err := hex.DecodeString(trimmed); err == nil {
			return decoded, nil
		}
	}
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == sha512.Size {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(decoded) == sha512.Size {
		return decoded, nil
	}
	return nil, errors.New("expected 64-byte base64 or 128-char hex digest")
}
