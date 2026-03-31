package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Committer runs a background loop that periodically commits staged Dolt
// changes. It checks an atomic dirty counter on the parent DB; if no writes
// occurred since the last commit the tick is a no-op.
type Committer struct {
	db       *DB
	log      *slog.Logger
	interval time.Duration
}

// NewCommitter creates a committer that batches Dolt commits at the given
// interval (typically 60s).
func NewCommitter(d *DB, log *slog.Logger, interval time.Duration) *Committer {
	return &Committer{db: d, log: log, interval: interval}
}

// Run starts the commit loop and blocks until ctx is cancelled. On shutdown it
// performs one final flush so that any trailing writes are committed.
func (c *Committer) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush with a fresh context so we don't fail on the
			// cancelled parent.
			flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			c.flush(flushCtx)
			cancel()
			return
		case <-ticker.C:
			c.flush(ctx)
		}
	}
}

// flush atomically swaps the dirty counter to zero. If there were writes it
// stages all changes and commits them. On error the dirty count is restored
// so the next tick retries.
func (c *Committer) flush(ctx context.Context) {
	n := c.db.dirty.Swap(0)
	if n == 0 {
		return
	}

	if _, err := c.db.ExecContext(ctx, "CALL dolt_add('-A')"); err != nil {
		c.log.Error("dolt_add failed", "err", err)
		c.db.dirty.Add(n) // restore so we retry
		return
	}

	msg := fmt.Sprintf("batch: %d writes", n)
	if _, err := c.db.ExecContext(ctx, "CALL dolt_commit('-m', ?, '--allow-empty')", msg); err != nil {
		c.log.Error("dolt_commit failed", "err", err, "writes", n)
		c.db.dirty.Add(n) // restore so we retry
		return
	}

	c.log.Info("dolt commit", "writes", n)
}
