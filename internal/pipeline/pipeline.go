// Package pipeline is the upload orchestrator. It takes a panorama upload
// request and walks the validate → probe → tile → exif → thumbs → upload tiles
// → upload original → insert sequence, with best-effort R2 cleanup on failure.
//
// Side effects (tiling, R2 PUTs, DB writes) are reached through small
// interfaces — Tiler, Uploader, Repo — so the orchestrator can be tested with
// fakes without touching libvips, R2, or Postgres.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/paulmach/orb"

	"github.com/michaelpeterswa/bigpictures.company/internal/db"
	"github.com/michaelpeterswa/bigpictures.company/internal/exif"
	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
	"github.com/michaelpeterswa/bigpictures.company/internal/slug"
	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
)

// Request is the input to a full upload run.
type Request struct {
	Source      string     // local path to the .tiff
	Slug        string     // pre-validated slug (use slug.FromTitle + slug.Validate)
	Title       string
	Description string
	CapturedAt  *time.Time
	Location    *orb.Point
	Tags        []string

	// Reprocess true skips EXIF and DB insert; only re-tiles and re-uploads.
	// The caller (reprocess subcommand) supplies the existing row.
	Reprocess bool
}

// Result is what the orchestrator returns on success.
type Result struct {
	ID           string
	Slug         string
	Width        int
	Height       int
	TileCount    int
	TilePrefix   string // e.g. panos/<slug>
	OriginalKey  string // e.g. originals/<slug>.tiff
	InfoJSONURL  string // full public URL
}

// Tiler is the subset of internal/tile this orchestrator uses.
type Tiler interface {
	Probe(ctx context.Context, src string) (*tile.Dimensions, error)
	Tile(ctx context.Context, src, outDir string, opts tile.Options) (*tile.Result, error)
	Thumbnails(ctx context.Context, src, outDir string, widths []int, q int) ([]tile.Thumb, error)
}

// Uploader is the subset of internal/upload this orchestrator uses.
type Uploader interface {
	UploadDir(ctx context.Context, localDir, keyPrefix string, opts UploadDirOpts) error
	PutOriginal(ctx context.Context, localPath, key string) error
	AssertHeaders(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	DeleteKey(ctx context.Context, key string) error
}

// UploadDirOpts is the orchestrator-facing subset of upload.DirOptions.
type UploadDirOpts struct {
	Progress func(UploadProgress)
}

// UploadProgress is reported during the upload phase.
type UploadProgress struct {
	UploadedFiles int
	TotalFiles    int
}

// Repo is the subset of internal/db this orchestrator uses.
type Repo interface {
	InsertPanorama(ctx context.Context, p *db.Panorama) (string, error)
}

// Options configure orchestrator behavior.
type Options struct {
	// TileBaseURL is the public base URL for tiles (PANO_TILE_BASE_URL).
	TileBaseURL string

	// TmpDir is the directory where intermediate pyramids and thumbnails are
	// written. Empty defaults to os.TempDir().
	TmpDir string

	// KeepTmp leaves intermediate files for debugging.
	KeepTmp bool

	// Tile passes through to internal/tile.Options.
	Tile tile.Options

	// ThumbWidths defaults to 300/600/1200 if zero-length.
	ThumbWidths []int

	// ThumbQuality defaults to 82.
	ThumbQuality int

	// OnPhase is called whenever the orchestrator transitions phases.
	OnPhase func(Phase)
	// OnTileProgress is forwarded from tile.Options.Progress.
	OnTileProgress func(tile.Progress)
	// OnUploadProgress is forwarded from upload progress.
	OnUploadProgress func(UploadProgress)

	// ExtractEXIF overrides the EXIF extractor. nil uses exif.Extract.
	// Exposed primarily so tests can avoid feeding real TIFF bytes.
	ExtractEXIF func(path string) (*exif.Metadata, error)
}

// Phase enumerates the orchestrator stages, in order.
type Phase string

const (
	PhaseValidate     Phase = "validate"
	PhaseTile         Phase = "tile"
	PhaseExif         Phase = "exif"
	PhaseThumbs       Phase = "thumbs"
	PhaseUploadTiles  Phase = "upload-tiles"
	PhaseUploadOriginal Phase = "upload-original"
	PhaseInsert       Phase = "insert"
	PhaseDone         Phase = "done"
)

// Runner glues a Tiler/Uploader/Repo together.
type Runner struct {
	Tiler    Tiler
	Uploader Uploader
	Repo     Repo
}

// Run executes the pipeline for req. On any failure after a successful R2
// upload, the orchestrator attempts to delete any objects it created — the
// DB row is the source of truth for "is this pano viewable," so we'd rather
// leak R2 objects (cheap, sweepable later) than land a DB row pointing at
// half-uploaded tiles.
func (r *Runner) Run(ctx context.Context, req Request, opts Options) (*Result, error) {
	if opts.ThumbQuality == 0 {
		opts.ThumbQuality = 82
	}
	if len(opts.ThumbWidths) == 0 {
		opts.ThumbWidths = []int{300, 600, 1200}
	}

	emit := func(p Phase) {
		if opts.OnPhase != nil {
			opts.OnPhase(p)
		}
	}

	// --- Validate ---
	emit(PhaseValidate)
	if err := slug.Validate(req.Slug); err != nil {
		return nil, err
	}
	if _, err := os.Stat(req.Source); err != nil {
		return nil, problems.New(
			"pipeline/source-not-found",
			"Source file is not readable",
			err.Error(),
			problems.WithExt(problems.Ext("path", req.Source)),
		)
	}

	// --- Tile ---
	workDir := opts.TmpDir
	if workDir == "" {
		workDir = os.TempDir()
	}
	pyramidDir := filepath.Join(workDir, "pano-"+req.Slug+"-pyramid")
	thumbsDir := filepath.Join(workDir, "pano-"+req.Slug+"-thumbs")
	if !opts.KeepTmp {
		defer func() {
			_ = os.RemoveAll(pyramidDir)
			_ = os.RemoveAll(thumbsDir)
		}()
	}
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		return nil, problems.New("pipeline/tmp-dir-failed", "Could not create temp directory", err.Error())
	}

	emit(PhaseTile)
	tileOpts := opts.Tile
	if opts.OnTileProgress != nil {
		tileOpts.Progress = opts.OnTileProgress
	}
	// info.json id should resolve to <tileBase>/panos/<slug>; dzsave appends
	// the basename, so we pass the parent URL and let it append <slug>.
	// String-concat rather than path.Join, which collapses "https://" to
	// "https:/" and breaks the resulting URL.
	tileOpts.ID = strings.TrimRight(opts.TileBaseURL, "/") + "/panos"
	tileResult, err := r.Tiler.Tile(ctx, req.Source, filepath.Join(pyramidDir, req.Slug), tileOpts)
	if err != nil {
		return nil, err
	}

	// --- EXIF ---
	var exifJSON []byte
	if !req.Reprocess {
		emit(PhaseExif)
		extract := opts.ExtractEXIF
		if extract == nil {
			extract = exif.Extract
		}
		md, err := extract(req.Source)
		if err != nil {
			return nil, err
		}
		// Fall back to EXIF capture date if the caller didn't supply one.
		if req.CapturedAt == nil && md.CapturedAt != nil {
			req.CapturedAt = md.CapturedAt
		}
		exifJSON, err = json.Marshal(md)
		if err != nil {
			return nil, problems.New("pipeline/exif-marshal-failed", "Could not marshal EXIF", err.Error())
		}
	}

	// --- Thumbnails ---
	emit(PhaseThumbs)
	if _, err := r.Tiler.Thumbnails(ctx, req.Source, thumbsDir, opts.ThumbWidths, opts.ThumbQuality); err != nil {
		return nil, err
	}

	// --- Upload tiles ---
	tilePrefix := "panos/" + req.Slug
	emit(PhaseUploadTiles)
	if err := r.Uploader.UploadDir(ctx, tileResult.OutDir, tilePrefix, UploadDirOpts{
		Progress: opts.OnUploadProgress,
	}); err != nil {
		return nil, err
	}
	// Upload thumbnails under the same prefix so the gallery can find them
	// via <tilePrefix>/thumb-<W>.webp.
	if err := r.Uploader.UploadDir(ctx, thumbsDir, tilePrefix, UploadDirOpts{}); err != nil {
		r.cleanupTilePrefix(ctx, tilePrefix)
		return nil, err
	}
	if err := r.Uploader.AssertHeaders(ctx, tilePrefix+"/info.json"); err != nil {
		r.cleanupTilePrefix(ctx, tilePrefix)
		return nil, err
	}

	// --- Upload original ---
	originalKey := "originals/" + req.Slug + filepath.Ext(req.Source)
	emit(PhaseUploadOriginal)
	if err := r.Uploader.PutOriginal(ctx, req.Source, originalKey); err != nil {
		r.cleanupTilePrefix(ctx, tilePrefix)
		return nil, err
	}

	// --- Insert (skipped on reprocess) ---
	//
	// The upload phase can keep the DB pool idle for many minutes. We bound
	// the insert with a short timeout so a wedged reconnect fails fast and
	// surfaces in the TUI rather than appearing as a hung [insert] phase.
	var insertedID string
	if !req.Reprocess {
		emit(PhaseInsert)
		row := &db.Panorama{
			Slug:         req.Slug,
			Title:        req.Title,
			Description:  req.Description,
			Width:        tileResult.Width,
			Height:       tileResult.Height,
			TilePath:     tilePrefix,
			ThumbPrefix:  tilePrefix,
			OriginalPath: originalKey,
			EXIF:         exifJSON,
			Tags:         req.Tags,
		}
		if req.CapturedAt != nil {
			row.CapturedAt = pgtype.Timestamptz{Time: *req.CapturedAt, Valid: true}
		}
		if req.Location != nil {
			row.Location = req.Location
		}
		insertCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		id, err := r.Repo.InsertPanorama(insertCtx, row)
		cancel()
		if err != nil {
			r.cleanupTilePrefix(ctx, tilePrefix)
			r.cleanupKey(ctx, originalKey)
			return nil, err
		}
		insertedID = id
	}

	emit(PhaseDone)
	return &Result{
		ID:          insertedID,
		Slug:        req.Slug,
		Width:       tileResult.Width,
		Height:      tileResult.Height,
		TileCount:   tileResult.TileCount,
		TilePrefix:  tilePrefix,
		OriginalKey: originalKey,
		InfoJSONURL: fmt.Sprintf("%s/%s/info.json", opts.TileBaseURL, tilePrefix),
	}, nil
}

// cleanupTilePrefix tries to remove the tile prefix on failure. Any error is
// swallowed and reported as a warning-class Problem to a logger if one were
// wired in. For now we emit nothing — the upcoming `pano gc` will sweep
// orphans against the DB.
func (r *Runner) cleanupTilePrefix(ctx context.Context, prefix string) {
	if r.Uploader == nil {
		return
	}
	_ = r.Uploader.DeletePrefix(ctx, prefix)
}

func (r *Runner) cleanupKey(ctx context.Context, key string) {
	if r.Uploader == nil {
		return
	}
	_ = r.Uploader.DeleteKey(ctx, key)
}

// SourceNotFound reports whether err is a pipeline/source-not-found Problem.
func SourceNotFound(err error) bool {
	if p, ok := problems.As(err); ok {
		return p.Type == problems.Base+"/pipeline/source-not-found"
	}
	return false
}

