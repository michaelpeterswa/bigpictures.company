package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchTileDimensions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"@context":"http://iiif.io/api/image/3/context.json","width":28792,"height":7390,"type":"ImageService3"}`))
	}))
	defer srv.Close()

	w, h, err := fetchTileDimensions(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetchTileDimensions: %v", err)
	}
	if w != 28792 || h != 7390 {
		t.Errorf("got %dx%d, want 28792x7390", w, h)
	}
}

func TestFetchTileDimensions_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, _, err := fetchTileDimensions(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status; got %v", err)
	}
}

func TestFetchTileDimensions_MissingFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"width":0,"height":0}`))
	}))
	defer srv.Close()

	_, _, err := fetchTileDimensions(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error on width=0/height=0")
	}
}

func TestFetchTileDimensions_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, _, err := fetchTileDimensions(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}
