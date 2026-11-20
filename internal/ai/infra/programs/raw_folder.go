package programs

import (
	"fmt"
	"os"
	"path/filepath"
)

func emptyRawFolder(raw string) error {
	if err := os.MkdirAll(raw, 0o755); err != nil {
		return fmt.Errorf("creating the raw folder: %w", err)
	}
	entries, err := os.ReadDir(raw)
	if err != nil {
		return fmt.Errorf("reading the raw folder: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(raw, entry.Name())); err != nil {
			return fmt.Errorf("clearing the raw folder: %w", err)
		}
	}
	return nil
}

func writeSchema(folder string, j job) (string, error) {
	path := filepath.Join(folder, j.schemaFile)
	if err := os.WriteFile(path, []byte(j.schema), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", j.schemaFile, err)
	}
	return path, nil
}
