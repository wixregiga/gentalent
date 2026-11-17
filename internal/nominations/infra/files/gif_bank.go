package files

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
)

type GifBank struct {
	dir string
}

func NewGifBank(dir string) GifBank {
	return GifBank{dir: dir}
}

func (b GifBank) Pick(uses map[string]int) (string, error) {
	entries, err := os.ReadDir(b.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the shame GIF bank: %w", err)
	}
	var least []string
	fewest := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".gif") {
			continue
		}
		switch count := uses[name]; {
		case len(least) == 0 || count < fewest:
			least, fewest = []string{name}, count
		case count == fewest:
			least = append(least, name)
		}
	}
	if len(least) == 0 {
		return "", nil
	}
	return filepath.Join(b.dir, least[rand.IntN(len(least))]), nil
}
