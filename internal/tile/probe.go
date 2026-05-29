package tile

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Dimensions describes the result of a probe.
type Dimensions struct {
	Width  int
	Height int
	Format string // e.g. "tiffload"
}

// Probe runs `vips header -f <field>` for the fields we care about.
// Returns a tile/invalid-source Problem if any field can't be read.
func Probe(ctx context.Context, src string) (*Dimensions, error) {
	w, err := readHeaderField(ctx, src, "width")
	if err != nil {
		return nil, err
	}
	h, err := readHeaderField(ctx, src, "height")
	if err != nil {
		return nil, err
	}
	wi, err := strconv.Atoi(strings.TrimSpace(w))
	if err != nil {
		return nil, problems.New("tile/invalid-source", "Could not parse width", err.Error())
	}
	hi, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil {
		return nil, problems.New("tile/invalid-source", "Could not parse height", err.Error())
	}
	loader, _ := readHeaderField(ctx, src, "vips-loader") // optional
	return &Dimensions{
		Width:  wi,
		Height: hi,
		Format: strings.TrimSpace(loader),
	}, nil
}

func readHeaderField(ctx context.Context, src, field string) (string, error) {
	out, err := exec.CommandContext(ctx, "vipsheader", "-f", field, src).Output()
	if err != nil {
		return "", problems.New(
			"tile/invalid-source",
			"Could not read "+field+" from image",
			err.Error(),
			problems.WithExt(problems.Ext("path", src), problems.Ext("field", field)),
		)
	}
	return string(out), nil
}
