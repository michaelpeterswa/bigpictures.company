// Package db wraps Postgres access for the pano CLI: a pgx connection pool,
// migration runner backed by embedded SQL, and typed query helpers for the
// panoramas table. All error paths return RFC 9457 Problems.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// DB is a thin wrapper around *pgxpool.Pool, exposed so callers can compose
// queries without depending on pgx directly when they don't need to.
type DB struct {
	pool *pgxpool.Pool
}

// Open dials the Postgres URL and returns a DB whose pool is ready to use.
// Returns a Problem under "db/connect-failed" on any dial or ping failure.
//
// Pool tuning: the `pano upload` command idles the pool for many minutes
// during the R2 upload phase, after which Neon may have silently closed the
// idle conn server-side. Defaults left a dead conn in the pool for the
// insert to wedge on. We aggressively expire idle conns so the insert
// acquires a fresh one regardless of how long the upload took.
func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, problems.New(
			"db/connect-failed",
			"Could not parse database URL",
			err.Error(),
		)
	}
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 30 * time.Second
	cfg.HealthCheckPeriod = 15 * time.Second
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, problems.New(
			"db/connect-failed",
			"Could not open database pool",
			err.Error(),
		)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, problems.New(
			"db/connect-failed",
			"Database ping failed",
			err.Error(),
		)
	}
	return &DB{pool: pool}, nil
}

// Pool returns the underlying pgx pool for callers that need direct access.
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

// Close releases the pool.
func (d *DB) Close() {
	if d != nil && d.pool != nil {
		d.pool.Close()
	}
}

// Ping returns nil if the database is reachable.
func (d *DB) Ping(ctx context.Context) error {
	if err := d.pool.Ping(ctx); err != nil {
		return problems.New(
			"db/ping-failed",
			"Database ping failed",
			err.Error(),
		)
	}
	return nil
}

// NotFoundType is the canonical Problem type URI for missing-row errors. Use
// [IsNotFound] to test an error for it rather than comparing the string.
const NotFoundType = problemBase + "/db/not-found"

const problemBase = "https://bigpictures.company/problems"

// IsNotFound reports whether err (or anything in its chain) is a not-found
// Problem returned by this package.
func IsNotFound(err error) bool {
	p, ok := problems.As(err)
	if !ok {
		return false
	}
	return p.Type == NotFoundType
}
