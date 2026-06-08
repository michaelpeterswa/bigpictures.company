package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// HeartbeatInterval is the default poll cadence for StartHeartbeat. It must
// be comfortably below Neon's default 5-minute scale-to-zero window so the
// compute never crosses the suspend threshold mid-job.
const HeartbeatInterval = 60 * time.Second

// StartHeartbeat opens a Postgres connection in the background and pings it
// every `interval` to keep Neon's compute from suspending due to inactivity.
// Returns a stop function the caller MUST invoke (typically via defer) to
// close the connection and release the goroutine.
//
// Rationale: Neon's compute scales to zero after a window of no DB activity
// (default 5 minutes — counted from the last query, NOT from the last open
// connection). A `pano upload` run holds an idle pool for 20-30 minutes
// during tiling + R2 upload, which trips suspend; the end-of-job INSERT
// then has to wake the compute, and the first dial after resume is the
// historically wedged operation. A periodic SELECT 1 keeps the compute
// warm so the INSERT dial lands fast.
//
// Errors are deliberately swallowed: heartbeat is best-effort. If the
// initial dial fails or a ping fails mid-flight, the loop closes the dead
// conn and re-dials on the next tick. The pipeline's ConnectAndInsert does
// its own retry+backoff on the actual INSERT, so a missing heartbeat
// degrades performance but not correctness.
func StartHeartbeat(ctx context.Context, url string, interval time.Duration) func() {
	if interval <= 0 {
		interval = HeartbeatInterval
	}
	hbCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		heartbeatLoop(hbCtx, url, interval)
	}()
	return func() {
		cancel()
		<-done
	}
}

// heartbeatLoop is the goroutine body. Reconnects on any ping failure so a
// transient blip doesn't disable the heartbeat for the rest of the run.
func heartbeatLoop(ctx context.Context, url string, interval time.Duration) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		// Bad URL — nothing useful we can do. Surface via the typed insert
		// failure later; heartbeat absence isn't a fatal condition.
		return
	}
	cfg.ConnectTimeout = 10 * time.Second

	var conn *pgx.Conn
	closeConn := func() {
		if conn != nil {
			_ = conn.Close(context.Background())
			conn = nil
		}
	}
	defer closeConn()

	// First tick immediately so the heartbeat starts working even if the
	// upload phase itself takes well under the interval.
	tick := func() {
		if conn == nil {
			dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
			c, dialErr := pgx.ConnectConfig(dialCtx, cfg)
			dialCancel()
			if dialErr == nil {
				conn = c
			}
		}
		if conn != nil {
			pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
			if err := conn.Ping(pingCtx); err != nil {
				closeConn() // reconnect on next tick
			}
			pingCancel()
		}
	}

	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}
