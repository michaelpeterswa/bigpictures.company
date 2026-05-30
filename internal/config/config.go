// Package config loads CLI configuration from PANO_* environment variables via
// caarlos/env. The struct is intentionally one big bag — different subcommands
// require different subsets, so per-command Require* methods check only the
// fields that command actually needs. This avoids forcing every invocation
// (e.g. `pano --version`) to set every variable.
package config

import (
	"fmt"

	"alpineworks.io/rfc9457"
	"github.com/caarlos0/env/v11"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Config holds every PANO_* variable. Fields are optional at load time;
// individual subcommands call Require* methods to enforce what they need.
type Config struct {
	R2AccountID       string `env:"PANO_R2_ACCOUNT_ID"`
	R2AccessKeyID     string `env:"PANO_R2_ACCESS_KEY_ID"`
	R2SecretAccessKey string `env:"PANO_R2_SECRET_ACCESS_KEY"`
	R2Bucket          string `env:"PANO_R2_BUCKET"`
	TileBaseURL       string `env:"PANO_TILE_BASE_URL"`
	DatabaseURL       string `env:"PANO_DATABASE_URL"`
	TmpDir            string `env:"PANO_TMP_DIR"`
}

// Load reads PANO_* variables from the environment. Parser-level errors are
// wrapped as RFC 9457 Problems under "config/parse-failed".
func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, problems.New(
			"config/parse-failed",
			"Configuration parse failed",
			err.Error(),
		)
	}
	return &cfg, nil
}

// RequireR2 returns a Problem listing any unset R2 fields.
func (c *Config) RequireR2() error {
	return c.requireFields(map[string]string{
		"PANO_R2_ACCOUNT_ID":        c.R2AccountID,
		"PANO_R2_ACCESS_KEY_ID":     c.R2AccessKeyID,
		"PANO_R2_SECRET_ACCESS_KEY": c.R2SecretAccessKey,
		"PANO_R2_BUCKET":            c.R2Bucket,
		"PANO_TILE_BASE_URL":        c.TileBaseURL,
	})
}

// RequireDatabase returns a Problem if PANO_DATABASE_URL is empty.
func (c *Config) RequireDatabase() error {
	return c.requireFields(map[string]string{
		"PANO_DATABASE_URL": c.DatabaseURL,
	})
}

func (c *Config) requireFields(fields map[string]string) error {
	var missing []string
	for name, value := range fields {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return MissingRequired(missing)
}

// MissingRequired builds the canonical Problem for one or more unset env vars.
// The unset variable names land in the "fields" extension so machine consumers
// can surface them without parsing the detail string.
func MissingRequired(fields []string) *rfc9457.RFC9457 {
	detail := fmt.Sprintf("missing required environment variable(s): %v", fields)
	return problems.New(
		"config/missing-required",
		"Required configuration is missing",
		detail,
		problems.WithExt(problems.Ext("fields", fields)),
	)
}

// AsProblem unwraps any error in the chain to a *rfc9457.RFC9457, returning
// (nil, false) if the chain does not contain one. Delegates to problems.As.
func AsProblem(err error) (*rfc9457.RFC9457, bool) {
	return problems.As(err)
}
