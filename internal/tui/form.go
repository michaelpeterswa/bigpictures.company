package tui

import (
	"strings"

	"github.com/charmbracelet/huh"
)

// UploadFormResult holds the values a user enters in the interactive upload
// form. Used to populate flags when `pano upload <file>` is invoked without
// any flags on a TTY.
type UploadFormResult struct {
	Title       string
	Description string
	Slug        string // optional override
	Captured    string // YYYY-MM-DD; empty = unset
	Location    string // "lat,lon"; empty = unset
	Tags        []string
}

// RunUploadForm walks the user through the upload form. The form blocks until
// submitted or cancelled. Returns (nil, nil) on cancel.
func RunUploadForm(defaults UploadFormResult) (*UploadFormResult, error) {
	r := defaults
	var tagsStr string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Description("Short, human-readable title — shown in the gallery").
				Value(&r.Title).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errEmpty
					}
					return nil
				}),
			huh.NewInput().
				Title("Slug").
				Description("URL slug. Leave blank to derive from the title").
				Value(&r.Slug),
		),
		huh.NewGroup(
			huh.NewText().
				Title("Description").
				Description("Optional. Markdown is fine").
				Value(&r.Description),
			huh.NewInput().
				Title("Capture date").
				Description("YYYY-MM-DD; leave blank to use EXIF or none").
				Value(&r.Captured),
			huh.NewInput().
				Title("Location").
				Description("\"lat,lon\" (e.g. 46.926,-121.557); leave blank for none").
				Value(&r.Location),
			huh.NewInput().
				Title("Tags").
				Description("Comma-separated").
				Value(&tagsStr),
		),
	)
	if err := form.Run(); err != nil {
		return nil, err
	}

	r.Tags = splitTags(tagsStr)
	return &r, nil
}

func splitTags(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

type sentinelErr string

func (s sentinelErr) Error() string { return string(s) }

const errEmpty sentinelErr = "title is required"
