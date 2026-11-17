package programs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	identifyTimeout = 15 * time.Second
	convertTimeout  = 30 * time.Second
	fallbackBinDir  = "/usr/bin"
	tileHeight      = "x480"
	tileBorderColor = "#151e2b"
)

type ImageMagick struct {
	identify string
	convert  string
}

func NewImageMagick() (ImageMagick, error) {
	identify, err := resolveTool("identify", fallbackBinDir)
	if err != nil {
		return ImageMagick{}, err
	}
	convert, err := resolveTool("convert", fallbackBinDir)
	if err != nil {
		return ImageMagick{}, err
	}
	return ImageMagick{identify: identify, convert: convert}, nil
}

func resolveTool(name, fallbackDir string) (string, error) {
	if found, err := exec.LookPath(name); err == nil {
		return found, nil
	}
	fallback := filepath.Join(fallbackDir, name)
	if info, err := os.Stat(fallback); err == nil && !info.IsDir() {
		return fallback, nil
	}
	return "", fmt.Errorf("required image tool not found: %s", name)
}

func (m ImageMagick) Check(ctx context.Context, path string) error {
	name := filepath.Base(path)
	if err := directNonEmptyFile(path); err != nil {
		return fmt.Errorf("missing or invalid raw image file %s: %w", name, err)
	}
	if _, err := runProgram(ctx, call{exe: m.identify, args: []string{path}, timeout: identifyTimeout}); err != nil {
		return fmt.Errorf("file is not a valid image: %s: %w", name, err)
	}
	return nil
}

func (m ImageMagick) Optimize(ctx context.Context, source, destination string) error {
	return m.convertImage(ctx, destination, source, "-auto-orient", "-resize", "960x960>", "-colorspace", "sRGB", "-strip", "-sampling-factor", "4:2:0", "-quality", "78", destination)
}

func (m ImageMagick) Stitch(ctx context.Context, photos []string, destination string) error {
	scratch, err := os.MkdirTemp(filepath.Dir(destination), ".strip-")
	if err != nil {
		return fmt.Errorf("creating the strip scratch folder: %w", err)
	}
	defer os.RemoveAll(scratch)

	args := make([]string, 0, len(photos)+9)
	for i, photo := range photos {
		tile := filepath.Join(scratch, fmt.Sprintf("tile_%d.jpg", i+1))
		if err := m.convertImage(ctx, tile, photo, "-resize", tileHeight, "-bordercolor", tileBorderColor, "-border", "6x6", tile); err != nil {
			return fmt.Errorf("preparing strip tile %d: %w", i+1, err)
		}
		args = append(args, tile)
	}
	args = append(args, "+append", "-colorspace", "sRGB", "-strip", "-sampling-factor", "4:2:0", "-quality", "80", destination)
	return m.convertImage(ctx, destination, args...)
}

func (m ImageMagick) convertImage(ctx context.Context, output string, args ...string) error {
	if _, err := runProgram(ctx, call{exe: m.convert, args: args, timeout: convertTimeout}); err != nil {
		return err
	}
	if info, err := os.Stat(output); err != nil || info.IsDir() {
		return fmt.Errorf("convert produced no file at %s", output)
	}
	return nil
}

func directNonEmptyFile(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	folder, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return err
	}
	if filepath.Dir(resolved) != folder {
		return errors.New("resolves outside the raw folder")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("not a non-empty file")
	}
	return nil
}
