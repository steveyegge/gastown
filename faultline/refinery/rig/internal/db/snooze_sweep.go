package db

import (
	"context"
	"log/slog"
	"time"
)

// SnoozeSweep runs periodic checks for expired snoozed issues.
type SnoozeSweep struct {
	db       *DB
	log      *slog.Logger
	interval time.Duration
	// QuietThreshold is how long since last_seen before an expired snooze
	// is considered "quiet" and auto-resolved (default 24 hours).
	QuietThreshold time.Duration
}

// NewSnoozeSweep creates a snooze sweep worker.
func NewSnoozeSweep(d *DB, log *slog.Logger, interval time.Duration) *SnoozeSweep {
	return &SnoozeSweep{
		db:             d,
		log:            log,
		interval:       interval,
		QuietThreshold: 24 * time.Hour,
	}
}

// Run starts the sweep loop and blocks until ctx is cancelled.
func (s *SnoozeSweep) Run(ctx context.Context) {
	s.sweep(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

func (s *SnoozeSweep) sweep(ctx context.Context) {
	expired, err := s.db.GetExpiredSnoozedIssues(ctx)
	if err != nil {
		s.log.Error("snooze sweep: get expired issues", "err", err)
		return
	}
	if len(expired) == 0 {
		return
	}

	var autoResolved, unsnoozed int
	quietCutoff := time.Now().UTC().Add(-s.QuietThreshold)

	for _, issue := range expired {
		if issue.LastSeen.Before(quietCutoff) {
			// Issue is quiet (no recent events) — auto-resolve.
			if err := s.db.ResolveIssueGroup(ctx, issue.ProjectID, issue.ID); err != nil {
				s.log.Error("snooze sweep: auto-resolve", "issue", issue.ID, "err", err)
				continue
			}
			// Clear snooze fields on the now-resolved issue.
			_, _ = s.db.ExecContext(ctx,
				"UPDATE issue_groups SET snoozed_until = NULL, snooze_reason = NULL, snoozed_by = NULL WHERE project_id = ? AND id = ?",
				issue.ProjectID, issue.ID,
			)
			_ = s.db.InsertLifecycleEvent(ctx, issue.ProjectID, issue.ID, LifecycleResolved, nil, nil, map[string]interface{}{
				"trigger": "snooze_expired_quiet",
				"reason":  issue.SnoozeReason,
			})
			autoResolved++
		} else {
			// Issue is still active — unsnooze it.
			if err := s.db.UnsnoozeIssueGroup(ctx, issue.ProjectID, issue.ID); err != nil {
				s.log.Error("snooze sweep: unsnooze", "issue", issue.ID, "err", err)
				continue
			}
			_ = s.db.InsertLifecycleEvent(ctx, issue.ProjectID, issue.ID, LifecycleUnsnoozed, nil, nil, map[string]interface{}{
				"trigger": "snooze_expired_active",
				"reason":  issue.SnoozeReason,
			})
			unsnoozed++
		}
	}

	if autoResolved > 0 || unsnoozed > 0 {
		s.log.Info("snooze sweep", "auto_resolved", autoResolved, "unsnoozed", unsnoozed)
	}
}
