// Package tile shells out to libvips to generate IIIF Image API 3.0 level-0
// tile pyramids and pre-sized thumbnails. We don't use cgo bindings (govips/
// bimg) so the resulting binary stays pure-Go and we inherit dzsave's
// streaming, memory-mapped pipeline for gigapixel TIFFs for free.
package tile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// MinVipsMajor and MinVipsMinor describe the minimum libvips version that
// supports `dzsave --layout iiif3`. Older versions silently fail or produce a
// different output structure.
const (
	MinVipsMajor = 8
	MinVipsMinor = 11
)

// Options configure a Tile call. Zero values pick sane defaults.
type Options struct {
	// ID is the parent URL dzsave appends the output basename onto when
	// writing info.json's "id" field. To produce
	// "https://tiles.bigpictures.company/panos/mt-rainier", pass
	// "https://tiles.bigpictures.company/panos" and an outDir basename of
	// "mt-rainier". If empty, dzsave's default ("https://example.com/iiif")
	// is kept; callers can rewrite the file later if needed.
	ID string

	// Quality is the WebP quality factor (1-100). Default 82.
	Quality int

	// TileSize is the IIIF tile edge length in pixels. Default 256.
	TileSize int

	// Concurrency sets VIPS_CONCURRENCY for the dzsave invocation. Zero
	// inherits vips's default (NCPU); set to a lower value when the upload
	// stage is competing for cores.
	Concurrency int

	// Progress, if non-nil, is called for each parsed progress event from
	// dzsave's --vips-progress stderr.
	Progress func(Progress)
}

// Result describes a completed tile pyramid on disk.
type Result struct {
	// OutDir is the directory containing info.json plus the tile subdirs.
	OutDir string

	// InfoJSONPath is filepath.Join(OutDir, "info.json").
	InfoJSONPath string

	// Width and Height are pulled from info.json.
	Width  int
	Height int

	// TileCount is the count of files in OutDir excluding info.json. Useful
	// for sizing upload progress.
	TileCount int
}

// Tile runs `vips dzsave src outDir --layout iiif3 ...`. The output directory
// is created if necessary. Returns a Result describing the produced pyramid.
//
// dzsave's stderr is consumed in real time; --vips-progress lines are parsed
// and forwarded to opts.Progress.
func Tile(ctx context.Context, src, outDir string, opts Options) (*Result, error) {
	if opts.Quality == 0 {
		opts.Quality = 82
	}
	if opts.TileSize == 0 {
		opts.TileSize = 256
	}

	if err := ensureVipsAvailable(ctx); err != nil {
		return nil, err
	}
	if _, err := os.Stat(src); err != nil {
		return nil, problems.New(
			"tile/invalid-source",
			"Source image is not readable",
			err.Error(),
			problems.WithExt(problems.Ext("path", src)),
		)
	}
	if err := os.MkdirAll(filepath.Dir(outDir), 0o755); err != nil {
		return nil, problems.New("tile/dzsave-failed", "Could not create output directory", err.Error())
	}

	args := []string{
		"dzsave", src, outDir,
		"--layout", "iiif3",
		"--suffix", fmt.Sprintf(".webp[Q=%d]", opts.Quality),
		"--tile-size", strconv.Itoa(opts.TileSize),
		"--vips-progress",
		// Force complete tile coverage. dzsave's default --skip-blanks
		// silently drops tiles that are "nearly equal to the background"
		// — fine for some use cases, but for panoramas with uniform sky
		// or water it leaves OSD-visible gaps. -1 disables the heuristic.
		"--skip-blanks", "-1",
	}
	if opts.ID != "" {
		args = append(args, "--id", opts.ID)
	}

	cmd := exec.CommandContext(ctx, "vips", args...)
	if opts.Concurrency > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("VIPS_CONCURRENCY=%d", opts.Concurrency))
	}

	// vips --vips-progress writes to stdout (not stderr) and uses CR-only
	// separators between updates. We also capture stderr for diagnostics.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, problems.New("tile/dzsave-failed", "Could not attach stdout pipe", err.Error())
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, problems.New("tile/dzsave-failed", "Could not attach stderr pipe", err.Error())
	}
	if err := cmd.Start(); err != nil {
		return nil, problems.New("tile/dzsave-failed", "Could not start vips", err.Error())
	}

	progressTail := streamProgress(stdout, opts.Progress)
	errTail := streamProgress(stderr, nil)
	// All reads must finish before cmd.Wait, per os/exec docs.
	progressTail.Wait()
	errTail.Wait()

	if err := cmd.Wait(); err != nil {
		// Surface both streams' last lines — stderr is where errors land,
		// stdout is where progress lands; users want both for diagnosis.
		return nil, problems.New(
			"tile/dzsave-failed",
			"vips dzsave failed",
			err.Error(),
			problems.WithExt(
				problems.Ext("stderr", errTail.lines()),
				problems.Ext("stdout_tail", progressTail.lines()),
			),
		)
	}

	return summarize(outDir)
}

// ensureVipsAvailable checks that `vips` is on PATH and meets the minimum
// version. Errors map to typed Problems so the CLI can render a clear message.
func ensureVipsAvailable(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "vips", "--version").CombinedOutput()
	if err != nil {
		return problems.New(
			"tile/vips-not-found",
			"libvips is not installed",
			"the `vips` command is required for tiling but was not found on PATH",
		)
	}
	major, minor, ok := parseVipsVersion(string(out))
	if !ok {
		// Couldn't parse — let it proceed; dzsave will fail loudly if the
		// installed version is wrong.
		return nil
	}
	if major < MinVipsMajor || (major == MinVipsMajor && minor < MinVipsMinor) {
		return problems.New(
			"tile/vips-too-old",
			"libvips is too old",
			fmt.Sprintf("detected vips %d.%d; need >= %d.%d for --layout iiif3", major, minor, MinVipsMajor, MinVipsMinor),
			problems.WithExt(problems.Ext("detected", fmt.Sprintf("%d.%d", major, minor))),
		)
	}
	return nil
}

// parseVipsVersion extracts (major, minor) from "vips-8.18.2" / "8.18.2" /
// "libvips 8.18.2" style output.
func parseVipsVersion(s string) (major, minor int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "vips-")
	s = strings.TrimPrefix(s, "libvips ")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return maj, min, true
}

// summarize reads info.json, patches it to declare webp as the preferred
// format (dzsave doesn't), counts the tiles produced, and writes info.json
// back so the upload picks up the patched version.
//
// Without preferredFormats/extraFormats, OpenSeadragon defaults to .jpg per
// the IIIF level-0 spec — and our tiles are .webp, so every request 404s.
func summarize(outDir string) (*Result, error) {
	infoPath := filepath.Join(outDir, "info.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, problems.New(
			"tile/dzsave-failed",
			"dzsave did not produce info.json",
			err.Error(),
			problems.WithExt(problems.Ext("path", infoPath)),
		)
	}
	// Decode into a map so we preserve unknown fields when re-encoding.
	var info map[string]any
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, problems.New(
			"tile/dzsave-failed",
			"info.json was malformed",
			err.Error(),
		)
	}
	info["extraFormats"] = []string{"webp"}
	info["preferredFormats"] = []string{"webp"}
	patched, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, problems.New("tile/dzsave-failed", "Could not re-encode info.json", err.Error())
	}
	if err := os.WriteFile(infoPath, patched, 0o644); err != nil {
		return nil, problems.New("tile/dzsave-failed", "Could not write patched info.json", err.Error())
	}
	width, _ := info["width"].(float64)
	height, _ := info["height"].(float64)

	tileCount := 0
	walkErr := filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "info.json" {
			return nil
		}
		tileCount++
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, problems.New("tile/dzsave-failed", "Could not enumerate tiles", walkErr.Error())
	}

	return &Result{
		OutDir:       outDir,
		InfoJSONPath: infoPath,
		Width:        int(width),
		Height:       int(height),
		TileCount:    tileCount,
	}, nil
}
