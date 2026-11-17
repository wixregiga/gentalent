package agents

import (
	"fmt"
	"os"
)

type SourcesFile struct{}

func (SourcesFile) WriteSources(path, text string) error {
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
