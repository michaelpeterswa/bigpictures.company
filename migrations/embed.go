// Package migrations embeds the SQL migration files into the pano binary so
// `pano migrate up` works without any files on disk. The files are exposed via
// [FS] for consumption by an iofs source driver.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
