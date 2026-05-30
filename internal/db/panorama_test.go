// These tests run against a real Postgres+PostGIS database. Set
// PANO_TEST_DATABASE_URL to enable; they are skipped otherwise. They are
// destructive: every test truncates the panoramas table before running.
package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/paulmach/orb"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("PANO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PANO_TEST_DATABASE_URL not set")
	}
	m, err := NewMigrator(url)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("Migrator.Up: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Migrator.Close: %v", err)
	}

	conn, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(conn.Close)

	if _, err := conn.pool.Exec(context.Background(), "truncate panoramas"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return conn
}

func TestPanorama_InsertAndGet(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()

	pt := orb.Point{-121.557, 46.926}
	in := &Panorama{
		Slug:         "test-pano",
		Title:        "Test Pano",
		Description:  "Hi",
		CapturedAt:   pgtype.Timestamptz{Time: time.Date(2025, 8, 14, 12, 0, 0, 0, time.UTC), Valid: true},
		Location:     &pt,
		Width:        20000,
		Height:       8000,
		TilePath:     "panos/test-pano",
		ThumbPrefix:  "panos/test-pano",
		OriginalPath: "originals/test-pano.tiff",
		EXIF:         []byte(`{"camera":"X"}`),
		Tags:         []string{"wa", "test"},
	}
	id, err := conn.InsertPanorama(ctx, in)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if id == "" {
		t.Fatal("Insert returned empty id")
	}

	got, err := conn.GetPanoramaBySlug(ctx, "test-pano")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got.Title != "Test Pano" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Location == nil {
		t.Fatal("Location is nil")
	}
	if got.Location.Lon() != pt.Lon() || got.Location.Lat() != pt.Lat() {
		t.Errorf("Location = %v, want %v", *got.Location, pt)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags = %v", got.Tags)
	}
}

func TestPanorama_GetBySlug_NotFound(t *testing.T) {
	conn := testDB(t)
	_, err := conn.GetPanoramaBySlug(context.Background(), "nope")
	if !IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestPanorama_SlugConflict(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	in := &Panorama{
		Slug:         "dup",
		Title:        "Dup",
		Width:        1,
		Height:       1,
		TilePath:     "p/dup",
		ThumbPrefix:  "p/dup",
		OriginalPath: "o/dup.tiff",
	}
	if _, err := conn.InsertPanorama(ctx, in); err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	_, err := conn.InsertPanorama(ctx, in)
	if err == nil {
		t.Fatal("expected slug-conflict error")
	}
}

func TestPanorama_List(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	for i, slug := range []string{"a", "b", "c"} {
		captured := pgtype.Timestamptz{Time: time.Now().Add(time.Duration(i) * time.Hour), Valid: true}
		_, err := conn.InsertPanorama(ctx, &Panorama{
			Slug:         slug,
			Title:        slug,
			CapturedAt:   captured,
			Width:        1,
			Height:       1,
			TilePath:     "p/" + slug,
			ThumbPrefix:  "p/" + slug,
			OriginalPath: "o/" + slug + ".tiff",
		})
		if err != nil {
			t.Fatalf("Insert %s: %v", slug, err)
		}
	}
	list, err := conn.ListPanoramas(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	// Latest captured first.
	if list[0].Slug != "c" {
		t.Errorf("list[0].Slug = %q, want c", list[0].Slug)
	}
}

func TestPanorama_Delete(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	if _, err := conn.InsertPanorama(ctx, &Panorama{
		Slug:         "rm",
		Title:        "rm",
		Width:        1,
		Height:       1,
		TilePath:     "p/rm",
		ThumbPrefix:  "p/rm",
		OriginalPath: "o/rm.tiff",
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if err := conn.DeletePanoramaBySlug(ctx, "rm"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := conn.DeletePanoramaBySlug(ctx, "rm"); !IsNotFound(err) {
		t.Errorf("expected IsNotFound on second delete, got %v", err)
	}
}
