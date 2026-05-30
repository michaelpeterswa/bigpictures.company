package tile

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Thumb is one rendered thumbnail.
type Thumb struct {
	Width int    // requested width in pixels
	Path  string // absolute path to the .webp on disk
}

// Thumbnails renders src to one .webp per width in widths under outDir, named
// "thumb-<w>.webp". Each call shells out to `vips thumbnail`. Returns the list
// of produced thumbnails in the same order as widths.
//
// Q defaults to 82 when zero is passed.
func Thumbnails(ctx context.Context, src, outDir string, widths []int, q int) ([]Thumb, error) {
	if q == 0 {
		q = 82
	}
	out := make([]Thumb, 0, len(widths))
	for _, w := range widths {
		path := filepath.Join(outDir, fmt.Sprintf("thumb-%d.webp", w))
		args := []string{
			"thumbnail",
			src,
			fmt.Sprintf("%s[Q=%d]", path, q),
			strconv.Itoa(w),
		}
		if cmdOut, err := exec.CommandContext(ctx, "vips", args...).CombinedOutput(); err != nil {
			return nil, problems.New(
				"tile/thumbnail-failed",
				"vips thumbnail failed",
				err.Error(),
				problems.WithExt(
					problems.Ext("width", w),
					problems.Ext("stderr", string(cmdOut)),
				),
			)
		}
		out = append(out, Thumb{Width: w, Path: path})
	}
	return out, nil
}
