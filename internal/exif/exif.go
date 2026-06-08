// Package exif extracts EXIF metadata from TIFF source files into a flat,
// JSON-ready map suitable for storage in the panoramas.exif JSONB column.
//
// Stitched panoramas often have sparse or missing EXIF — Hugin/PTGui/Lightroom
// strip or rewrite different fields. We never fail an upload over missing
// EXIF; callers get an empty Metadata and a nil CapturedAt and decide what to
// do with that.
package exif

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	goexif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	tiffstructure "github.com/dsoprea/go-tiff-image-structure/v2"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Metadata is the normalized JSON-ready EXIF blob.
//
// The structure is intentionally shallow so it can be marshaled straight into
// the JSONB column without further transformation, and so the Next.js side
// can read it without knowing about EXIF tag IDs.
type Metadata struct {
	CapturedAt *time.Time `json:"captured_at,omitempty"`
	GPS        *GPS       `json:"gps,omitempty"`
	Camera     *Camera    `json:"camera,omitempty"`
	Lens       *Lens      `json:"lens,omitempty"`
	Exposure   *Exposure  `json:"exposure,omitempty"`
}

// GPS is decimal lat/lon — easier to consume than rationals with hemisphere
// reference tags.
type GPS struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Camera holds the EXIF Make + Model tags after vendor-specific cleanup.
type Camera struct {
	Make  string `json:"make,omitempty"`
	Model string `json:"model,omitempty"`
}

// Lens holds the lens model string pulled from whichever EXIF lens tag
// the camera populated (LensModel / LensSpecification / LensInfo).
type Lens struct {
	Model string `json:"model,omitempty"`
}

// Exposure aggregates the standard exposure-related EXIF fields. Fields are
// omitted (zero-value) when the source EXIF didn't populate them.
type Exposure struct {
	ISO         int     `json:"iso,omitempty"`
	FNumber     float64 `json:"f_number,omitempty"`
	ShutterText string  `json:"shutter,omitempty"`
	FocalLength float64 `json:"focal_length,omitempty"`
}

// Extract reads the TIFF at path and returns a normalized Metadata. Missing
// EXIF yields a zero-value Metadata; only catastrophic failures (unreadable
// file) return an error.
func Extract(path string) (*Metadata, error) {
	tags, err := flatTags(path)
	if err != nil {
		return nil, err
	}
	return buildMetadata(tags), nil
}

// flatTags returns a name-keyed map of EXIF tag values. Stitched panos often
// have no EXIF at all — that's not an error condition; we return an empty map.
func flatTags(path string) (map[string]any, error) {
	parser := tiffstructure.NewTiffMediaParser()
	mc, err := parser.ParseFile(path)
	if err != nil {
		// dsoprea's tiff parser surfaces a missing-EXIF block as goexif's
		// ErrNoExif sentinel during ParseFile (not just on tmc.Exif()).
		// Hugin/PTGui frequently strip EXIF entirely from stitched output —
		// treat that as "empty metadata", not as an upload failure.
		if errors.Is(err, goexif.ErrNoExif) {
			return map[string]any{}, nil
		}
		return nil, problems.New(
			"exif/parse-failed",
			"Could not parse TIFF",
			err.Error(),
			problems.WithExt(problems.Ext("path", path)),
		)
	}
	tmc, ok := mc.(*tiffstructure.TiffMediaContext)
	if !ok {
		return map[string]any{}, nil
	}
	_, data, err := tmc.Exif()
	if err != nil {
		return map[string]any{}, nil
	}
	flat, _, err := goexif.GetFlatExifData(data, nil)
	if err != nil {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(flat))
	for _, t := range flat {
		if t.Value == nil {
			continue
		}
		// First occurrence wins for tag-name collisions across IFDs; this
		// keeps root-IFD fields (Make/Model) from being overwritten by sub-IFD
		// metadata using the same name.
		if _, dup := out[t.TagName]; !dup {
			out[t.TagName] = t.Value
		}
	}
	return out, nil
}

func buildMetadata(t map[string]any) *Metadata {
	md := &Metadata{}
	if cam := buildCamera(t); cam != nil {
		md.Camera = cam
	}
	if l := buildLens(t); l != nil {
		md.Lens = l
	}
	if ts := buildCapturedAt(t); ts != nil {
		md.CapturedAt = ts
	}
	if g := buildGPS(t); g != nil {
		md.GPS = g
	}
	if e := buildExposure(t); e != nil {
		md.Exposure = e
	}
	return md
}

func buildCamera(t map[string]any) *Camera {
	mk := stripWrappingBrackets(strings.TrimSpace(asString(t["Make"])))
	mo := stripWrappingBrackets(strings.TrimSpace(asString(t["Model"])))
	if mk == "" && mo == "" {
		return nil
	}
	return &Camera{Make: mk, Model: mo}
}

// stripWrappingBrackets removes a single pair of "[...]" around a string,
// which is how some camera vendors (notably Sony) store the model name in
// EXIF. Other extractors (exiftool) hide them; we do the same so the value
// is clean for display and JSON storage.
func stripWrappingBrackets(s string) string {
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func buildLens(t map[string]any) *Lens {
	for _, key := range []string{"LensModel", "LensSpecification", "LensInfo"} {
		if v := strings.TrimSpace(asString(t[key])); v != "" {
			return &Lens{Model: v}
		}
	}
	return nil
}

// buildCapturedAt prefers DateTimeOriginal over DateTime, normalizes to UTC.
// EXIF dates use "YYYY:MM:DD HH:MM:SS" without a timezone; if no TZ tag is
// present we treat the value as already-UTC (documented).
func buildCapturedAt(t map[string]any) *time.Time {
	for _, key := range []string{"DateTimeOriginal", "DateTimeDigitized", "DateTime"} {
		raw := strings.TrimSpace(asString(t[key]))
		if raw == "" {
			continue
		}
		if parsed, err := time.Parse("2006:01:02 15:04:05", raw); err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

// buildGPS combines GPSLatitude + GPSLatitudeRef (and longitude pair) into
// signed decimal degrees. Returns nil if any of the four fields are missing.
func buildGPS(t map[string]any) *GPS {
	lat, latOK := decodeGPSDegrees(t["GPSLatitude"])
	lon, lonOK := decodeGPSDegrees(t["GPSLongitude"])
	if !latOK || !lonOK {
		return nil
	}
	latRef := strings.ToUpper(strings.TrimSpace(asString(t["GPSLatitudeRef"])))
	lonRef := strings.ToUpper(strings.TrimSpace(asString(t["GPSLongitudeRef"])))
	if latRef == "S" {
		lat = -lat
	}
	if lonRef == "W" {
		lon = -lon
	}
	return &GPS{Lat: lat, Lon: lon}
}

// decodeGPSDegrees expects a 3-element rational slice (degrees, minutes,
// seconds). Returns 0, false if the value isn't shaped like that.
func decodeGPSDegrees(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case []exifcommon.Rational:
		if len(x) < 3 {
			return 0, false
		}
		return ratio(x[0]) + ratio(x[1])/60.0 + ratio(x[2])/3600.0, true
	}
	return 0, false
}

func buildExposure(t map[string]any) *Exposure {
	out := &Exposure{}
	hit := false
	if iso, ok := asInt(t["ISOSpeedRatings"]); ok {
		out.ISO = iso
		hit = true
	}
	if f, ok := asFloat(t["FNumber"]); ok {
		out.FNumber = f
		hit = true
	}
	if s := asString(t["ExposureTime"]); s != "" {
		out.ShutterText = s
		hit = true
	}
	if fl, ok := asFloat(t["FocalLength"]); ok {
		out.FocalLength = fl
		hit = true
	}
	if !hit {
		return nil
	}
	return out
}

// --- value coercion helpers ---

func asString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []string:
		// EXIF STRING tags often arrive as single-element slices; strip
		// the wrapping rather than render as a Go literal.
		return strings.Join(x, " ")
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, asString(e))
		}
		return strings.Join(parts, " ")
	case exifcommon.Rational:
		return formatRational(x)
	case []exifcommon.Rational:
		// ExposureTime, FNumber, etc. land here as a single-element slice.
		// Render as N/D so JSON consumers get "1/800", not "[{1 800}]".
		parts := make([]string, 0, len(x))
		for _, r := range x {
			parts = append(parts, formatRational(r))
		}
		return strings.Join(parts, " ")
	case fmt.Stringer:
		return x.String()
	}
	// Catch-all for any slice type the switch didn't enumerate (go-exif
	// uses some non-public concrete types). Walking via reflect keeps
	// model strings from rendering as `[ILCE-7M4]`.
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		parts := make([]string, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			parts = append(parts, asString(rv.Index(i).Interface()))
		}
		return strings.Join(parts, " ")
	}
	return fmt.Sprint(v)
}

func formatRational(r exifcommon.Rational) string {
	return fmt.Sprintf("%d/%d", r.Numerator, r.Denominator)
}

func asInt(v any) (int, bool) {
	if v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case uint16:
		return int(x), true
	case uint32:
		return int(x), true
	case []uint16:
		if len(x) > 0 {
			return int(x[0]), true
		}
	case []uint32:
		if len(x) > 0 {
			return int(x[0]), true
		}
	}
	if n, err := strconv.Atoi(asString(v)); err == nil {
		return n, true
	}
	return 0, false
}

func asFloat(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case []exifcommon.Rational:
		if len(x) > 0 {
			return ratio(x[0]), true
		}
	case exifcommon.Rational:
		return ratio(x), true
	}
	if f, err := strconv.ParseFloat(asString(v), 64); err == nil {
		return f, true
	}
	return 0, false
}

func ratio(r exifcommon.Rational) float64 {
	if r.Denominator == 0 {
		return 0
	}
	return float64(r.Numerator) / float64(r.Denominator)
}
