package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michaelpeterswa/bigpictures.company/internal/db"
	"github.com/michaelpeterswa/bigpictures.company/internal/exif"
	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
	"github.com/michaelpeterswa/bigpictures.company/internal/tile"
)

// noEXIF is a stub for Options.ExtractEXIF that returns an empty Metadata so
// tests don't need a real TIFF.
func noEXIF(path string) (*exif.Metadata, error) {
	return &exif.Metadata{}, nil
}

// capturedTileID lets a test read back the IIIF id URL the orchestrator
// passed to the Tiler. Set inside fakeTiler.Tile.
var capturedTileID string

// fakeTiler writes a minimal info.json + a sentinel tile so the orchestrator
// has a tree to upload. Does not require libvips on PATH.
type fakeTiler struct{}

func (fakeTiler) Probe(ctx context.Context, src string) (*tile.Dimensions, error) {
	return &tile.Dimensions{Width: 100, Height: 50}, nil
}

func (fakeTiler) Tile(ctx context.Context, src, outDir string, opts tile.Options) (*tile.Result, error) {
	capturedTileID = opts.ID
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	infoPath := filepath.Join(outDir, "info.json")
	if err := os.WriteFile(infoPath, []byte(`{"width":100,"height":50}`), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "tile.webp"), []byte("x"), 0o644); err != nil {
		return nil, err
	}
	return &tile.Result{
		OutDir: outDir, InfoJSONPath: infoPath,
		Width: 100, Height: 50, TileCount: 1,
	}, nil
}

func (fakeTiler) Thumbnails(ctx context.Context, src, outDir string, widths []int, q int) ([]tile.Thumb, error) {
	out := make([]tile.Thumb, 0, len(widths))
	for _, w := range widths {
		p := filepath.Join(outDir, "thumb.webp")
		if err := os.WriteFile(p, []byte("t"), 0o644); err != nil {
			return nil, err
		}
		out = append(out, tile.Thumb{Width: w, Path: p})
	}
	return out, nil
}

// fakeUploader records calls and can be configured to fail at specific steps.
type fakeUploader struct {
	uploads        []string
	originals      []string
	asserted       []string
	deletePrefixes []string
	deleteKeys     []string

	failAtUploadIdx   int // 0=disabled, otherwise fail on the N-th UploadDir call
	failPutOriginal   bool
	failAssertHeaders bool
}

func (u *fakeUploader) UploadDir(ctx context.Context, localDir, keyPrefix string, opts UploadDirOpts) error {
	u.uploads = append(u.uploads, keyPrefix)
	if u.failAtUploadIdx > 0 && len(u.uploads) == u.failAtUploadIdx {
		return errors.New("upload boom")
	}
	return nil
}

func (u *fakeUploader) PutOriginal(ctx context.Context, localPath, key string) error {
	u.originals = append(u.originals, key)
	if u.failPutOriginal {
		return errors.New("original boom")
	}
	return nil
}

func (u *fakeUploader) AssertHeaders(ctx context.Context, key string) error {
	u.asserted = append(u.asserted, key)
	if u.failAssertHeaders {
		return errors.New("header mismatch")
	}
	return nil
}

func (u *fakeUploader) DeletePrefix(ctx context.Context, prefix string) error {
	u.deletePrefixes = append(u.deletePrefixes, prefix)
	return nil
}

func (u *fakeUploader) DeleteKey(ctx context.Context, key string) error {
	u.deleteKeys = append(u.deleteKeys, key)
	return nil
}

// fakeRepo records inserts.
type fakeRepo struct {
	inserts    []db.Panorama
	failInsert error
}

func (r *fakeRepo) InsertPanorama(ctx context.Context, p *db.Panorama) (string, error) {
	if r.failInsert != nil {
		return "", r.failInsert
	}
	r.inserts = append(r.inserts, *p)
	return "row-id", nil
}

func makeSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "in.tiff")
	if err := os.WriteFile(src, []byte("fake tiff bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestRunner_HappyPath(t *testing.T) {
	src := makeSource(t)
	up := &fakeUploader{}
	repo := &fakeRepo{}
	r := &Runner{Tiler: fakeTiler{}, Uploader: up, RepoFactory: StaticRepo(repo)}

	res, err := r.Run(context.Background(), Request{
		Source: src, Slug: "test", Title: "Test",
	}, Options{
		TileBaseURL: "https://tiles.example",
		TmpDir:      t.TempDir(),
		ExtractEXIF: noEXIF,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ID != "row-id" || res.Slug != "test" || res.TileCount != 1 {
		t.Errorf("unexpected result: %+v", res)
	}
	if len(repo.inserts) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(repo.inserts))
	}
	if repo.inserts[0].TilePath != "panos/test" {
		t.Errorf("TilePath = %q", repo.inserts[0].TilePath)
	}
	if !strings.HasSuffix(res.InfoJSONURL, "/panos/test/info.json") {
		t.Errorf("InfoJSONURL = %q", res.InfoJSONURL)
	}
	if len(up.deletePrefixes) != 0 || len(up.deleteKeys) != 0 {
		t.Errorf("no cleanup expected on happy path; got prefixes=%v keys=%v",
			up.deletePrefixes, up.deleteKeys)
	}
}

// TestRunner_TileIDPreservesDoubleSlash guards against a regression where the
// orchestrator constructed the IIIF id with path.Join, which collapses
// "https://" to "https:/" and breaks every viewer.
func TestRunner_TileIDPreservesDoubleSlash(t *testing.T) {
	capturedTileID = ""
	src := makeSource(t)
	r := &Runner{Tiler: fakeTiler{}, Uploader: &fakeUploader{}, RepoFactory: StaticRepo(&fakeRepo{})}
	if _, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{
		TileBaseURL: "https://tiles.example.com",
		TmpDir:      t.TempDir(),
		ExtractEXIF: noEXIF,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedTileID != "https://tiles.example.com/panos" {
		t.Errorf("tile ID = %q, want %q", capturedTileID, "https://tiles.example.com/panos")
	}
}

func TestRunner_InvalidSlug(t *testing.T) {
	r := &Runner{Tiler: fakeTiler{}, Uploader: &fakeUploader{}, RepoFactory: StaticRepo(&fakeRepo{})}
	_, err := r.Run(context.Background(), Request{
		Source: makeSource(t), Slug: "BAD UPPER", Title: "Bad",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if err == nil {
		t.Fatal("want error")
	}
	p, ok := problems.As(err)
	if !ok || p.Type != problems.Base+"/slug/invalid" {
		t.Errorf("got %v, want slug/invalid Problem", err)
	}
}

func TestRunner_SourceMissing(t *testing.T) {
	r := &Runner{Tiler: fakeTiler{}, Uploader: &fakeUploader{}, RepoFactory: StaticRepo(&fakeRepo{})}
	_, err := r.Run(context.Background(), Request{
		Source: "/no/such/file.tiff", Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if !SourceNotFound(err) {
		t.Errorf("got %v, want SourceNotFound", err)
	}
}

func TestRunner_TilesFail_NoCleanupYet(t *testing.T) {
	src := makeSource(t)
	up := &fakeUploader{failAtUploadIdx: 1}
	r := &Runner{Tiler: fakeTiler{}, Uploader: up, RepoFactory: StaticRepo(&fakeRepo{})}
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if err == nil {
		t.Fatal("expected error")
	}
	// First UploadDir failure means R2 has whatever it managed to PUT;
	// we don't sweep mid-call (the failed UploadDir already aborted on first
	// error). No insert should have happened.
	if len(up.deletePrefixes) != 0 {
		t.Errorf("no sweep expected on first-upload failure, got %v", up.deletePrefixes)
	}
}

func TestRunner_ThumbsFail_SweepTiles(t *testing.T) {
	// First UploadDir (tiles) succeeds, second (thumbs) fails — orchestrator
	// must sweep the tile prefix.
	src := makeSource(t)
	up := &fakeUploader{failAtUploadIdx: 2}
	r := &Runner{Tiler: fakeTiler{}, Uploader: up, RepoFactory: StaticRepo(&fakeRepo{})}
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(up.deletePrefixes) == 0 || up.deletePrefixes[0] != "panos/ok-slug" {
		t.Errorf("expected DeletePrefix(panos/ok-slug), got %v", up.deletePrefixes)
	}
}

func TestRunner_OriginalFail_SweepEverything(t *testing.T) {
	src := makeSource(t)
	up := &fakeUploader{failPutOriginal: true}
	r := &Runner{Tiler: fakeTiler{}, Uploader: up, RepoFactory: StaticRepo(&fakeRepo{})}
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(up.deletePrefixes) == 0 {
		t.Errorf("expected tile prefix sweep, got %v", up.deletePrefixes)
	}
}

func TestRunner_InsertFail_SweepAll(t *testing.T) {
	src := makeSource(t)
	up := &fakeUploader{}
	repo := &fakeRepo{failInsert: errors.New("db down")}
	r := &Runner{Tiler: fakeTiler{}, Uploader: up, RepoFactory: StaticRepo(repo)}
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(up.deletePrefixes) == 0 {
		t.Errorf("expected tile prefix sweep")
	}
	if len(up.deleteKeys) == 0 {
		t.Errorf("expected original key sweep")
	}
}

// TestRunner_Reprocess_SkipsUploadOriginal verifies that a reprocess request
// neither calls PutOriginal nor opens the RepoFactory. We pass a factory that
// would panic if called, so any regression is loud.
func TestRunner_Reprocess_SkipsUploadOriginal(t *testing.T) {
	src := makeSource(t)
	up := &fakeUploader{}
	r := &Runner{
		Tiler:    fakeTiler{},
		Uploader: up,
		RepoFactory: func(context.Context) (Repo, func(), error) {
			t.Fatal("RepoFactory must not be called on reprocess")
			return nil, nil, nil
		},
	}
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x", Reprocess: true,
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(up.originals) != 0 {
		t.Errorf("PutOriginal must not run on reprocess; got %v", up.originals)
	}
	// Tiles + thumbs SHOULD still upload (that's the whole point of reprocess).
	if len(up.uploads) == 0 {
		t.Errorf("expected tile UploadDir on reprocess")
	}
}

// TestRunner_RepoFactoryError surfaces a factory error as the run's error.
func TestRunner_RepoFactoryError(t *testing.T) {
	src := makeSource(t)
	want := errors.New("dial neon: i/o timeout")
	r := &Runner{
		Tiler:    fakeTiler{},
		Uploader: &fakeUploader{},
		RepoFactory: func(context.Context) (Repo, func(), error) {
			return nil, nil, want
		},
	}
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	if !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

// blockingRepo blocks InsertPanorama forever (or until ctx cancels). Used to
// drive the hard-insert-timeout path. The atomic counter tells the test the
// goroutine was actually entered.
type blockingRepo struct {
	entered atomic.Int32
}

func (b *blockingRepo) InsertPanorama(ctx context.Context, _ *db.Panorama) (string, error) {
	b.entered.Add(1)
	<-ctx.Done()
	return "", ctx.Err()
}

// TestRunner_InsertHardTimeout verifies that an InsertPanorama call which
// ignores its own context cancellation can't hang the runner forever. We swap
// in tiny budgets via a test hook so the test runs in a couple seconds.
func TestRunner_InsertHardTimeout(t *testing.T) {
	src := makeSource(t)
	up := &fakeUploader{}
	repo := &blockingRepo{}
	r := &Runner{Tiler: fakeTiler{}, Uploader: up, RepoFactory: StaticRepo(repo)}

	// Tighten the budgets via swap-and-restore so the test runs in ~300ms
	// instead of 30s. The package-level consts are the only things that
	// determine the wall clock, so this is the seam.
	origCtx, origHard := insertTimeoutsForTest(200*time.Millisecond, 300*time.Millisecond)
	defer insertTimeoutsForTest(origCtx, origHard)

	start := time.Now()
	_, err := r.Run(context.Background(), Request{
		Source: src, Slug: "ok-slug", Title: "x",
	}, Options{TileBaseURL: "https://x", TmpDir: t.TempDir(), ExtractEXIF: noEXIF})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if repo.entered.Load() != 1 {
		t.Errorf("Insert goroutine was not entered; entered=%d", repo.entered.Load())
	}
	// Generous upper bound to avoid flake; the point is the runner returned
	// in well under the historical many-minute hang.
	if elapsed > 5*time.Second {
		t.Errorf("Run took %s; hard timeout did not bound it", elapsed)
	}
	// On insert failure the orchestrator should still sweep tiles + original.
	if len(up.deletePrefixes) == 0 || len(up.deleteKeys) == 0 {
		t.Errorf("expected sweep on insert timeout; prefixes=%v keys=%v",
			up.deletePrefixes, up.deleteKeys)
	}
}
