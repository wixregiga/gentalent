package port

import "context"

type ImageTool interface {
	Check(ctx context.Context, path string) error
	Optimize(ctx context.Context, source, destination string) error
	Stitch(ctx context.Context, photos []string, destination string) error
}
