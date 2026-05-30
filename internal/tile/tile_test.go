package tile

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireVips skips the test if libvips isn't available; the unit-level work
// (progress parsing, version detection) is covered by progress_test.go without
// needing vips on PATH.
func requireVips(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vips"); err != nil {
		t.Skip("vips not on PATH")
	}
}

// makeFixture generates a small synthetic TIFF via `vips black`. Returns its
// path. The image is 1024x1024 so dzsave produces a multi-level pyramid.
func makeFixture(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "fixture.tiff")
	if out, err := exec.Command("vips", "black", path, "1024", "1024", "--bands", "3").CombinedOutput(); err != nil {
		t.Fatalf("vips black: %v: %s", err, out)
	}
	return path
}

func TestTile_ProducesValidIIIF3(t *testing.T) {
	requireVips(t)
	dir := t.TempDir()
	src := makeFixture(t, dir)
	out := filepath.Join(dir, "out")

	var seen []int
	// dzsave appends the output basename onto --id, so we pass the parent
	// URL and assert the resulting id ends with /out.
	res, err := Tile(context.Background(), src, out, Options{
		ID:       "https://example.test/iiif",
		Progress: func(p Progress) { seen = append(seen, p.Percent) },
	})
	if err != nil {
		t.Fatalf("Tile: %v", err)
	}
	if res.Width != 1024 || res.Height != 1024 {
		t.Errorf("dimensions: got %dx%d, want 1024x1024", res.Width, res.Height)
	}
	// For a 1024x1024 image at tile size 256 with scaleFactors [1,2,4],
	// dzsave should produce 16 + 4 + 1 = 21 tiles. If --skip-blanks ever
	// regresses away from -1, the uniform-black fixture will be entirely
	// skipped and this count drops to ~0. Asserting the exact count
	// catches that regression cheaply.
	if res.TileCount != 21 {
		t.Errorf("TileCount = %d, want 21 (uniform-black image; --skip-blanks regression?)", res.TileCount)
	}

	// info.json must be IIIF Image API 3.0.
	data, err := os.ReadFile(res.InfoJSONPath)
	if err != nil {
		t.Fatalf("read info.json: %v", err)
	}
	var info struct {
		Context  string `json:"@context"`
		ID       string `json:"id"`
		Type     string `json:"type"`
		Profile  string `json:"profile"`
		Protocol string `json:"protocol"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("unmarshal info.json: %v", err)
	}
	const wantCtx = "http://iiif.io/api/image/3/context.json"
	if info.Context != wantCtx {
		t.Errorf("@context = %q, want %q", info.Context, wantCtx)
	}
	if info.Type != "ImageService3" {
		t.Errorf("type = %q, want ImageService3", info.Type)
	}
	if info.Profile != "level0" {
		t.Errorf("profile = %q, want level0", info.Profile)
	}
	if info.ID != "https://example.test/iiif/out" {
		t.Errorf("id = %q, want dzsave to append basename to --id (https://example.test/iiif/out)", info.ID)
	}

	// Re-parse for preferredFormats / extraFormats — without these, OSD
	// defaults to .jpg and 404s on our .webp tiles.
	var formats struct {
		Preferred []string `json:"preferredFormats"`
		Extra     []string `json:"extraFormats"`
	}
	if err := json.Unmarshal(data, &formats); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if len(formats.Preferred) == 0 || formats.Preferred[0] != "webp" {
		t.Errorf("preferredFormats = %v, want [webp]", formats.Preferred)
	}
	if len(formats.Extra) == 0 || formats.Extra[0] != "webp" {
		t.Errorf("extraFormats = %v, want [webp]", formats.Extra)
	}

	if len(seen) == 0 {
		t.Error("no progress events seen")
	}
}

func TestProbe_Dimensions(t *testing.T) {
	requireVips(t)
	dir := t.TempDir()
	src := makeFixture(t, dir)
	dims, err := Probe(context.Background(), src)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if dims.Width != 1024 || dims.Height != 1024 {
		t.Errorf("got %dx%d, want 1024x1024", dims.Width, dims.Height)
	}
}

func TestThumbnails(t *testing.T) {
	requireVips(t)
	dir := t.TempDir()
	src := makeFixture(t, dir)
	thumbs, err := Thumbnails(context.Background(), src, dir, []int{300, 600}, 82)
	if err != nil {
		t.Fatalf("Thumbnails: %v", err)
	}
	if len(thumbs) != 2 {
		t.Fatalf("len(thumbs) = %d, want 2", len(thumbs))
	}
	for _, th := range thumbs {
		if _, err := os.Stat(th.Path); err != nil {
			t.Errorf("thumb at %s missing: %v", th.Path, err)
		}
	}
}

func TestParseVipsVersion(t *testing.T) {
	cases := []struct {
		in               string
		wantMaj, wantMin int
		ok               bool
	}{
		{"vips-8.18.2", 8, 18, true},
		{"libvips 8.11.0", 8, 11, true},
		{"8.10.5", 8, 10, true},
		{"garbage", 0, 0, false},
	}
	for _, c := range cases {
		maj, min, ok := parseVipsVersion(c.in)
		if ok != c.ok || maj != c.wantMaj || min != c.wantMin {
			t.Errorf("parseVipsVersion(%q) = (%d, %d, %t), want (%d, %d, %t)", c.in, maj, min, ok, c.wantMaj, c.wantMin, c.ok)
		}
	}
}
