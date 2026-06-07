package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/paulmach/orb"
	"github.com/spf13/cobra"

	"github.com/michaelpeterswa/bigpictures.company/internal/config"
	"github.com/michaelpeterswa/bigpictures.company/internal/db"
	"github.com/michaelpeterswa/bigpictures.company/internal/pipeline"
	"github.com/michaelpeterswa/bigpictures.company/internal/slug"
	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
	"github.com/michaelpeterswa/bigpictures.company/internal/tui"
	"github.com/michaelpeterswa/bigpictures.company/internal/upload"
)

type uploadFlags struct {
	title       string
	description string
	slug        string
	captured    string // YYYY-MM-DD
	location    string // "lat,lon"
	tags        []string
	tmpDir      string
	keepTmp     bool
}

func newUploadCmd() *cobra.Command {
	var f uploadFlags
	cmd := &cobra.Command{
		Use:   "upload <src>",
		Short: "Tile, upload, and register a panorama",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpload(cmd, args[0], f)
		},
	}
	cmd.Flags().StringVar(&f.title, "title", "", "Panorama title (required)")
	cmd.Flags().StringVar(&f.description, "description", "", "Description")
	cmd.Flags().StringVar(&f.slug, "slug", "", "URL slug; defaults to slugified title")
	cmd.Flags().StringVar(&f.captured, "captured", "", "Capture date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.location, "location", "", "Location \"lat,lon\"")
	cmd.Flags().StringSliceVar(&f.tags, "tags", nil, "Tags (comma-separated)")
	cmd.Flags().StringVar(&f.tmpDir, "tmp-dir", "", "Temp dir for intermediate files (defaults to $TMPDIR)")
	cmd.Flags().BoolVar(&f.keepTmp, "keep-tmp", false, "Keep intermediate tile/thumbnail files after upload")
	// --title intentionally not marked required: on a TTY we drop into the
	// interactive form when no flags are present.
	return cmd
}

func runUpload(cmd *cobra.Command, src string, f uploadFlags) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.RequireDatabase(); err != nil {
		return err
	}
	if err := cfg.RequireR2(); err != nil {
		return err
	}

	// Interactive form when no flags were given and stdin is a TTY.
	if f.title == "" && isInteractive() {
		filled, err := tui.RunUploadForm(tui.UploadFormResult{})
		if err != nil {
			return err
		}
		if filled == nil {
			return errors.New("cancelled")
		}
		f.title = filled.Title
		f.description = filled.Description
		f.slug = filled.Slug
		f.captured = filled.Captured
		f.location = filled.Location
		f.tags = filled.Tags
	}

	req, err := buildRequest(src, f)
	if err != nil {
		return err
	}

	runner, closeFn, err := buildRunner(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer closeFn()

	tmp := f.tmpDir
	if tmp == "" {
		tmp = cfg.TmpDir
	}

	opts := pipeline.Options{
		TileBaseURL: cfg.TileBaseURL,
		TmpDir:      tmp,
		KeepTmp:     f.keepTmp,
	}

	// TTY → Bubble Tea progress. Otherwise plain text logs, suitable for CI
	// and pipelines.
	if isInteractive() {
		return runUploadInteractive(cmd.Context(), runner, *req, opts, cmd)
	}
	return runUploadPlain(cmd.Context(), runner, *req, opts, cmd)
}

func runUploadPlain(ctx context.Context, runner *pipeline.Runner, req pipeline.Request, opts pipeline.Options, cmd *cobra.Command) error {
	opts.OnPhase = func(p pipeline.Phase) {
		fmt.Fprintf(cmd.ErrOrStderr(), "[%s]\n", p)
	}
	opts.OnTileProgress = func(p tile.Progress) {
		fmt.Fprintf(cmd.ErrOrStderr(), "\r  tile: %3d%%", p.Percent)
		if p.Done {
			fmt.Fprintln(cmd.ErrOrStderr())
		}
	}
	opts.OnUploadProgress = func(p pipeline.UploadProgress) {
		fmt.Fprintf(cmd.ErrOrStderr(), "\r  upload: %4d/%4d", p.UploadedFiles, p.TotalFiles)
	}
	res, err := runner.Run(ctx, req, opts)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr())
	printUploadSummary(cmd, res)
	return nil
}

// runUploadInteractive runs the pipeline in a goroutine and renders progress
// with bubbletea. All pipeline events go through tea.Program.Send so they're
// safely serialized with the model's Update loop.
func runUploadInteractive(ctx context.Context, runner *pipeline.Runner, req pipeline.Request, opts pipeline.Options, cmd *cobra.Command) error {
	model := tui.NewProgressModel(req.Title)
	prog := tea.NewProgram(model, tea.WithContext(ctx))

	opts.OnPhase = func(p pipeline.Phase) { prog.Send(tui.PhaseMsg{Phase: p}) }
	opts.OnTileProgress = func(p tile.Progress) { prog.Send(tui.TileProgressMsg{Progress: p}) }
	opts.OnUploadProgress = func(p pipeline.UploadProgress) { prog.Send(tui.UploadProgressMsg{Progress: p}) }

	go func() {
		res, err := runner.Run(ctx, req, opts)
		if err != nil {
			prog.Send(tui.ErrMsg{Err: err})
			return
		}
		prog.Send(tui.DoneMsg{Result: res})
	}()

	final, err := prog.Run()
	if err != nil {
		return err
	}
	m, ok := final.(tui.ProgressModel)
	if !ok {
		return errors.New("unexpected model type")
	}
	if e := m.Err(); e != nil {
		return e
	}
	printUploadSummary(cmd, m.Result())
	return nil
}

func printUploadSummary(cmd *cobra.Command, res *pipeline.Result) {
	if res == nil {
		return
	}
	fmt.Fprintf(
		cmd.OutOrStdout(),
		"ok\n  id:    %s\n  slug:  %s\n  size:  %dx%d (%d tiles)\n  info:  %s\n",
		res.ID, res.Slug, res.Width, res.Height, res.TileCount, res.InfoJSONURL,
	)
}

// isInteractive reports whether stdout AND stderr are both TTYs. Either piped
// stream means the user is consuming the output as text — drop into the plain
// rendering path so we don't dump ANSI into a file.
func isInteractive() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stderr.Fd())
}

func buildRequest(src string, f uploadFlags) (*pipeline.Request, error) {
	if f.title == "" {
		return nil, errors.New("title is required")
	}
	s := strings.TrimSpace(f.slug)
	if s == "" {
		s = slug.FromTitle(f.title)
	}
	if err := slug.Validate(s); err != nil {
		return nil, err
	}
	req := &pipeline.Request{
		Source:      src,
		Slug:        s,
		Title:       f.title,
		Description: f.description,
		Tags:        f.tags,
	}
	if f.captured != "" {
		t, err := time.Parse("2006-01-02", f.captured)
		if err != nil {
			return nil, fmt.Errorf("invalid --captured %q: %w", f.captured, err)
		}
		req.CapturedAt = &t
	}
	if f.location != "" {
		pt, err := parseLatLon(f.location)
		if err != nil {
			return nil, err
		}
		req.Location = pt
	}
	return req, nil
}

func parseLatLon(s string) (*orb.Point, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid location %q (want \"lat,lon\")", s)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude %q: %w", parts[0], err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude %q: %w", parts[1], err)
	}
	pt := orb.Point{lon, lat}
	return &pt, nil
}

// buildRunner wires the real Tiler/Uploader adapters and a lazy DB pool
// factory. The DB pool is NOT opened here — RepoFactory dials Neon at the
// moment of insert, after tiling and upload have completed. Opening earlier
// leaves the pool idle for many minutes during the upload, and a dial after
// that idle window has wedged in production. Lazy open eliminates that case
// entirely.
//
// Returns a no-op closer for symmetry with the previous signature; the
// RepoFactory's per-call closer handles pool teardown.
func buildRunner(ctx context.Context, cfg *config.Config) (*pipeline.Runner, func(), error) {
	r2, err := upload.NewClient(ctx, upload.Credentials{
		AccountID:       cfg.R2AccountID,
		AccessKeyID:     cfg.R2AccessKeyID,
		SecretAccessKey: cfg.R2SecretAccessKey,
		Bucket:          cfg.R2Bucket,
	})
	if err != nil {
		return nil, nil, err
	}
	runner := &pipeline.Runner{
		Tiler:    pipeline.TileAdapter{},
		Uploader: pipeline.UploadAdapter{Client: r2},
		RepoFactory: func(ctx context.Context) (pipeline.Repo, func(), error) {
			pool, err := db.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				return nil, nil, err
			}
			return pool, pool.Close, nil
		},
	}
	return runner, func() {}, nil
}
