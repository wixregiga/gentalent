package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gentalent/internal/messaging/domain"
)

type AttachmentReader struct{}

func (AttachmentReader) Attachment(ctx context.Context, path string) (domain.Attachment, error) {
	if err := ctx.Err(); err != nil {
		return domain.Attachment{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return domain.Attachment{}, err
	}
	if !info.Mode().IsRegular() {
		return domain.Attachment{}, fmt.Errorf("attachment is not a regular file: %s", path)
	}
	return domain.Attachment{Name: filepath.Base(path), Bytes: info.Size(), SizeKnown: true}, nil
}
