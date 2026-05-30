// Package slug derives URL-safe identifiers from human titles and enforces
// the format constraint the rest of the system depends on:
//
//	^[a-z0-9][a-z0-9-]{1,62}$
//
// Slugs become public URL components (`/p/<slug>`) and R2 prefixes
// (`panos/<slug>/`), so they must be small, ASCII, and stable. We deliberately
// don't auto-suffix on collisions; the form/CLI surfaces the conflict so the
// user picks the URL they want.
package slug

import (
	"fmt"
	"regexp"

	gosimple "github.com/gosimple/slug"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

const (
	minLen = 2
	maxLen = 63
)

// validRe requires the slug to begin and end with an alphanumeric so we don't
// produce URLs with dangling dashes. Length range: [2, 63].
var validRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

// FromTitle derives a slug from title using transliteration and lowercasing.
// Empty titles return an empty slug.
func FromTitle(title string) string {
	if title == "" {
		return ""
	}
	return gosimple.Make(title)
}

// Validate checks that s matches the canonical slug format. Returns a typed
// Problem when it doesn't.
func Validate(s string) error {
	if validRe.MatchString(s) {
		return nil
	}
	return problems.New(
		"slug/invalid",
		"Slug is not a valid URL component",
		fmt.Sprintf("got %q, want %d–%d chars matching %s", s, minLen, maxLen, validRe.String()),
		problems.WithExt(problems.Ext("slug", s)),
	)
}
