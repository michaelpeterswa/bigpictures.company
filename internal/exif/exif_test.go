package exif

import (
	"testing"
	"time"

	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

func TestBuildCapturedAt_PrefersOriginal(t *testing.T) {
	tags := map[string]any{
		"DateTime":         "2025:08:14 12:00:00",
		"DateTimeOriginal": "2025:08:13 09:30:00",
	}
	got := buildCapturedAt(tags)
	if got == nil {
		t.Fatal("got nil")
	}
	if got.Year() != 2025 || got.Month() != 8 || got.Day() != 13 || got.Hour() != 9 {
		t.Errorf("got %v, want DateTimeOriginal", got)
	}
	if got.Location() != time.UTC {
		t.Errorf("location = %v, want UTC", got.Location())
	}
}

func TestBuildCapturedAt_FallbackToDateTime(t *testing.T) {
	tags := map[string]any{
		"DateTime": "2025:08:14 12:00:00",
	}
	got := buildCapturedAt(tags)
	if got == nil {
		t.Fatal("got nil")
	}
	if got.Year() != 2025 || got.Day() != 14 {
		t.Errorf("got %v", got)
	}
}

func TestBuildCapturedAt_None(t *testing.T) {
	if got := buildCapturedAt(map[string]any{}); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestBuildCapturedAt_Malformed(t *testing.T) {
	tags := map[string]any{"DateTime": "not a date"}
	if got := buildCapturedAt(tags); got != nil {
		t.Errorf("got %v, want nil for malformed date", got)
	}
}

func rational(num, den int64) exifcommon.Rational {
	return exifcommon.Rational{Numerator: uint32(num), Denominator: uint32(den)}
}

func TestBuildGPS_Quadrants(t *testing.T) {
	// 46° 55' 33" lat × 121° 33' 25" lon, varying hemispheres.
	lat := []exifcommon.Rational{rational(46, 1), rational(55, 1), rational(33, 1)}
	lon := []exifcommon.Rational{rational(121, 1), rational(33, 1), rational(25, 1)}
	cases := []struct {
		latRef, lonRef    string
		wantSignLat, wantSignLon float64
	}{
		{"N", "E", 1, 1},
		{"N", "W", 1, -1},
		{"S", "E", -1, 1},
		{"S", "W", -1, -1},
	}
	for _, c := range cases {
		t.Run(c.latRef+c.lonRef, func(t *testing.T) {
			tags := map[string]any{
				"GPSLatitude":     lat,
				"GPSLongitude":    lon,
				"GPSLatitudeRef":  c.latRef,
				"GPSLongitudeRef": c.lonRef,
			}
			g := buildGPS(tags)
			if g == nil {
				t.Fatal("nil")
			}
			if (g.Lat > 0) != (c.wantSignLat > 0) {
				t.Errorf("lat sign wrong: got %f", g.Lat)
			}
			if (g.Lon > 0) != (c.wantSignLon > 0) {
				t.Errorf("lon sign wrong: got %f", g.Lon)
			}
			absLat := 46 + 55.0/60.0 + 33.0/3600.0
			absLon := 121 + 33.0/60.0 + 25.0/3600.0
			if d := abs(absLat - abs(g.Lat)); d > 1e-6 {
				t.Errorf("lat magnitude off by %f (got %f)", d, g.Lat)
			}
			if d := abs(absLon - abs(g.Lon)); d > 1e-6 {
				t.Errorf("lon magnitude off by %f (got %f)", d, g.Lon)
			}
		})
	}
}

func TestBuildGPS_MissingFields(t *testing.T) {
	tags := map[string]any{
		"GPSLatitudeRef": "N",
	}
	if g := buildGPS(tags); g != nil {
		t.Errorf("got %v, want nil for missing latitude", g)
	}
}

func TestBuildCamera_Skip(t *testing.T) {
	if c := buildCamera(map[string]any{}); c != nil {
		t.Errorf("got %v, want nil", c)
	}
}

func TestBuildExposure_PartialFields(t *testing.T) {
	tags := map[string]any{
		"ISOSpeedRatings": []uint16{400},
		"FNumber":         []exifcommon.Rational{rational(28, 10)}, // f/2.8
	}
	e := buildExposure(tags)
	if e == nil {
		t.Fatal("nil")
	}
	if e.ISO != 400 {
		t.Errorf("ISO = %d, want 400", e.ISO)
	}
	if d := abs(e.FNumber - 2.8); d > 1e-6 {
		t.Errorf("FNumber = %f, want 2.8", e.FNumber)
	}
}

func TestBuildCamera_StripsSonyBrackets(t *testing.T) {
	// Sony stores the model literally as "[ILCE-7M4]" in EXIF; other tools
	// strip the wrapping. We should too.
	tags := map[string]any{
		"Make":  "SONY",
		"Model": "[ILCE-7M4]",
	}
	c := buildCamera(tags)
	if c == nil {
		t.Fatal("nil camera")
	}
	if c.Model != "ILCE-7M4" {
		t.Errorf("Model = %q, want %q", c.Model, "ILCE-7M4")
	}
}

func TestAsString_RationalSlice(t *testing.T) {
	// ExposureTime arrives as []Rational{1/800}. Render as "1/800", not
	// "[{1 800}]" (which is the Go literal form).
	v := []exifcommon.Rational{rational(1, 800)}
	got := asString(v)
	if got != "1/800" {
		t.Errorf("asString(%v) = %q, want %q", v, got, "1/800")
	}
}

func TestStripWrappingBrackets(t *testing.T) {
	cases := []struct{ in, want string }{
		{"[ILCE-7M4]", "ILCE-7M4"},
		{"ILCE-7M4", "ILCE-7M4"},
		{"[]", ""},
		{"[]extra", "[]extra"}, // not a wrapping pair
		{"", ""},
	}
	for _, c := range cases {
		if got := stripWrappingBrackets(c.in); got != c.want {
			t.Errorf("stripWrappingBrackets(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
