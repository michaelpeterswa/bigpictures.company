// Package version exposes build metadata injected via -ldflags at link time.
package version

var (
	// Version is the semver tag (or "dev" for untagged builds).
	Version = "dev"

	// Commit is the short git SHA at build time.
	Commit = "unknown"

	// Date is the UTC build timestamp in RFC 3339 form.
	Date = "unknown"
)
