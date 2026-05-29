package pipeline

import (
	"context"

	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
	"github.com/michaelpeterswa/bigpictures.company/internal/upload"
)

// TileAdapter wraps the package-level tile helpers so they satisfy Tiler.
// We use a struct (not a thin wrapper of free functions) so future test
// instrumentation has a place to hang.
type TileAdapter struct{}

func (TileAdapter) Probe(ctx context.Context, src string) (*tile.Dimensions, error) {
	return tile.Probe(ctx, src)
}

func (TileAdapter) Tile(ctx context.Context, src, outDir string, opts tile.Options) (*tile.Result, error) {
	return tile.Tile(ctx, src, outDir, opts)
}

func (TileAdapter) Thumbnails(ctx context.Context, src, outDir string, widths []int, q int) ([]tile.Thumb, error) {
	return tile.Thumbnails(ctx, src, outDir, widths, q)
}

// UploadAdapter wraps an *upload.Client so it satisfies the Uploader interface
// with the orchestrator's local UploadDirOpts/UploadProgress types.
type UploadAdapter struct{ Client *upload.Client }

func (a UploadAdapter) UploadDir(ctx context.Context, localDir, keyPrefix string, opts UploadDirOpts) error {
	return a.Client.UploadDir(ctx, localDir, keyPrefix, upload.DirOptions{
		Progress: func(p upload.Progress) {
			if opts.Progress != nil {
				opts.Progress(UploadProgress{
					UploadedFiles: p.UploadedFiles,
					TotalFiles:    p.TotalFiles,
				})
			}
		},
	})
}

func (a UploadAdapter) PutOriginal(ctx context.Context, localPath, key string) error {
	return a.Client.PutOriginal(ctx, localPath, key, upload.MultipartOptions{})
}

func (a UploadAdapter) AssertHeaders(ctx context.Context, key string) error {
	return a.Client.AssertHeaders(ctx, key)
}

func (a UploadAdapter) DeletePrefix(ctx context.Context, prefix string) error {
	return a.Client.DeletePrefix(ctx, prefix)
}

func (a UploadAdapter) DeleteKey(ctx context.Context, key string) error {
	return a.Client.DeleteKey(ctx, key)
}
