package beeper

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ReadToken(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open token file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("secret file must be private: chmod 600 %s", path)
	}
	value, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(value))
	if token == "" {
		return "", fmt.Errorf("token was empty")
	}
	return token, nil
}

func WriteToken(path, value string) error {
	token := strings.TrimSpace(value)
	if token == "" {
		return fmt.Errorf("token was empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".new-*")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(token); err != nil {
		file.Close()
		return fmt.Errorf("write token: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync token file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace token file: %w", err)
	}
	return nil
}
