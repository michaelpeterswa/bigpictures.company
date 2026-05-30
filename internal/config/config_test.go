package config

import (
	"strings"
	"testing"
)

func TestLoad_OK(t *testing.T) {
	t.Setenv("PANO_R2_ACCOUNT_ID", "acct")
	t.Setenv("PANO_R2_ACCESS_KEY_ID", "ak")
	t.Setenv("PANO_R2_SECRET_ACCESS_KEY", "sk")
	t.Setenv("PANO_R2_BUCKET", "bucket")
	t.Setenv("PANO_TILE_BASE_URL", "https://tiles.example")
	t.Setenv("PANO_DATABASE_URL", "postgres://x")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.R2Bucket != "bucket" {
		t.Errorf("R2Bucket = %q, want %q", cfg.R2Bucket, "bucket")
	}
	if cfg.DatabaseURL != "postgres://x" {
		t.Errorf("DatabaseURL = %q, want %q", cfg.DatabaseURL, "postgres://x")
	}
}

func TestRequireR2_MissingFields(t *testing.T) {
	cfg := &Config{R2AccountID: "acct"}
	err := cfg.RequireR2()
	if err == nil {
		t.Fatal("RequireR2 with missing fields: want error, got nil")
	}
	p, ok := AsProblem(err)
	if !ok {
		t.Fatalf("AsProblem: error %v is not a Problem", err)
	}
	wantType := "https://bigpictures.company/problems/config/missing-required"
	if p.Type != wantType {
		t.Errorf("Type = %q, want %q", p.Type, wantType)
	}
	fields, ok := p.Extensions["fields"].([]string)
	if !ok {
		t.Fatalf("Extensions[fields] = %v (type %T), want []string", p.Extensions["fields"], p.Extensions["fields"])
	}
	// All four missing fields except R2AccountID should be present.
	want := map[string]bool{
		"PANO_R2_ACCESS_KEY_ID":     true,
		"PANO_R2_SECRET_ACCESS_KEY": true,
		"PANO_R2_BUCKET":            true,
		"PANO_TILE_BASE_URL":        true,
	}
	if len(fields) != len(want) {
		t.Errorf("fields = %v, want length %d", fields, len(want))
	}
	for _, f := range fields {
		if !want[f] {
			t.Errorf("unexpected missing field %q", f)
		}
	}
}

func TestRequireDatabase_Set(t *testing.T) {
	cfg := &Config{DatabaseURL: "postgres://x"}
	if err := cfg.RequireDatabase(); err != nil {
		t.Errorf("RequireDatabase with value set: want nil, got %v", err)
	}
}

func TestRequireDatabase_Missing(t *testing.T) {
	cfg := &Config{}
	err := cfg.RequireDatabase()
	if err == nil {
		t.Fatal("RequireDatabase with empty value: want error")
	}
	p, ok := AsProblem(err)
	if !ok {
		t.Fatalf("AsProblem: %v not a Problem", err)
	}
	if !strings.Contains(p.Detail, "PANO_DATABASE_URL") {
		t.Errorf("Detail = %q, want it to mention PANO_DATABASE_URL", p.Detail)
	}
}
