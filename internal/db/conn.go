package db

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Conn is a single, short-lived pgx connection. It's the recommended path
// for one-shot writes in this codebase (the end-of-job INSERT in `pano
// upload`), where pgxpool offers no value — there's no concurrency, no
// per-request churn — and its detached-context dial path makes a wedged
// dial impossible to bound at the caller level.
//
// See internal/db/conn.go's package doc and the design notes in
// docs/connection-strategy.md (forthcoming).
type Conn struct {
	c *pgx.Conn
}

// connConnectTimeout is the bound applied to a fresh dial. pgxpool detaches
// the caller's context during dial; only ConnConfig.ConnectTimeout applies
// at the pgconn layer. Neon's documented cold-start is "few hundred ms
// typical, p95 ~2.6s, worst ~3.1s" — 15s leaves comfortable headroom for
// transient TLS-handshake slowness during compute resume.
const connConnectTimeout = 15 * time.Second

// OpenConn dials Postgres for a one-shot connection with a bounded dial.
func OpenConn(ctx context.Context, url string) (*Conn, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, problems.New(
			"db/connect-failed",
			"Could not parse database URL",
			err.Error(),
		)
	}
	cfg.ConnectTimeout = connConnectTimeout
	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, problems.New(
			"db/connect-failed",
			"Could not open database connection",
			err.Error(),
		)
	}
	return &Conn{c: c}, nil
}

// Close releases the connection. Safe to call on a nil *Conn.
func (c *Conn) Close(ctx context.Context) error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.Close(ctx)
}

// InsertPanorama writes a row through this single connection.
func (c *Conn) InsertPanorama(ctx context.Context, p *Panorama) (string, error) {
	return insertPanoramaWith(ctx, c.c, p)
}

// RetryOptions configure ConnectAndInsert's exponential-backoff retry loop.
type RetryOptions struct {
	// MaxAttempts is the total number of attempts including the first
	// (default 4 — i.e. one initial try plus three retries).
	MaxAttempts int
	// InitialDelay is the wait before the second attempt (default 1s).
	InitialDelay time.Duration
	// MaxDelay caps the exponential growth (default 8s).
	MaxDelay time.Duration
	// JitterRatio is the ±fraction applied to each delay (default 0.2).
	JitterRatio float64
}

func (o *RetryOptions) setDefaults() {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 4
	}
	if o.InitialDelay <= 0 {
		o.InitialDelay = 1 * time.Second
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = 8 * time.Second
	}
	if o.JitterRatio <= 0 {
		o.JitterRatio = 0.2
	}
}

// ConnectAndInsert is the end-of-job INSERT path for the upload pipeline.
// Each attempt opens a fresh single-conn (no pool), runs InsertPanorama,
// and closes. On transient errors it retries with exponential backoff +
// jitter, matching Neon's documented mitigation for cold-start latency
// (https://neon.com/docs/connect/connection-latency).
//
// Errors that are NOT retried:
//   - db/slug-conflict (UNIQUE violation will keep failing identically)
//   - Caller context already cancelled (no point in another attempt)
//
// Everything else — dial errors, network errors, transient server errors —
// is retried up to MaxAttempts. The final error returned is the LAST
// attempt's failure; intermediate errors are dropped.
func ConnectAndInsert(ctx context.Context, url string, p *Panorama, opts RetryOptions) (string, error) {
	opts.setDefaults()
	delay := opts.InitialDelay
	var lastErr error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		if attempt > 1 {
			wait := withJitter(delay, opts.JitterRatio)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			delay = nextDelay(delay, opts.MaxDelay)
		}
		id, err := tryConnectAndInsert(ctx, url, p)
		if err == nil {
			return id, nil
		}
		lastErr = err
		if isSlugConflict(err) {
			return "", err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
	}
	return "", lastErr
}

// tryConnectAndInsert is a single attempt: open, insert, close. Separated
// so ConnectAndInsert's retry loop stays a flat read.
func tryConnectAndInsert(ctx context.Context, url string, p *Panorama) (string, error) {
	conn, err := OpenConn(ctx, url)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	return conn.InsertPanorama(ctx, p)
}

// withJitter returns d adjusted by ±ratio. A ratio of 0.2 produces a value
// uniformly distributed in [0.8d, 1.2d].
func withJitter(d time.Duration, ratio float64) time.Duration {
	if ratio <= 0 {
		return d
	}
	delta := float64(d) * ratio * (2*rand.Float64() - 1) //nolint:gosec // jitter, not crypto
	out := d + time.Duration(delta)
	if out < 0 {
		return 0
	}
	return out
}

// nextDelay doubles cur, capped at max.
func nextDelay(cur, maxDelay time.Duration) time.Duration {
	n := cur * 2
	if n > maxDelay {
		return maxDelay
	}
	return n
}

// isSlugConflict reports whether err is the typed slug-conflict Problem
// returned by wrapInsertErr on a UNIQUE violation.
func isSlugConflict(err error) bool {
	if err == nil {
		return false
	}
	p, ok := problems.As(err)
	if !ok {
		return false
	}
	return p.Type == problemBase+"/db/slug-conflict"
}
