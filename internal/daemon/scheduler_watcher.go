package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

)

// SchedulerWatcher monitors dispatch health and sends mail alerts on problems.
type SchedulerWatcher struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	logger     func(string, ...interface{})
	gtPath     string
	townRoot   string
	notify     *NotificationManager
	interval   time.Duration
	thresholds WatcherThresholds
}

// WatcherThresholds defines alert thresholds.
type WatcherThresholds struct {
	QueueDepth      int           // Alert if > N items pending
	PendingAge      time.Duration // Alert if oldest > N
	StrandedConvoys int           // Alert if > N stranded
	FailureRate     int           // Alert if > N failures in window
	FailureWindow   time.Duration // Window for failure counting
}

// DefaultThresholds returns sensible default thresholds.
func DefaultThresholds() WatcherThresholds {
	return WatcherThresholds{
		QueueDepth:      10,
		PendingAge:      1 * time.Hour,
		StrandedConvoys: 3,
		FailureRate:     3,
		FailureWindow:   5 * time.Minute,
	}
}

// NewSchedulerWatcher creates a new scheduler health watcher.
func NewSchedulerWatcher(townRoot, gtPath string, logger func(string, ...interface{}), notify *NotificationManager) *SchedulerWatcher {
	return &SchedulerWatcher{
		townRoot:   townRoot,
		gtPath:     gtPath,
		logger:     logger,
		notify:     notify,
		interval:   60 * time.Second,
		thresholds: DefaultThresholds(),
	}
}

// Start begins the scheduler watcher goroutine.
func (w *SchedulerWatcher) Start() error {
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.wg.Add(1)
	go w.run()
	w.logger("Scheduler watcher started (interval=%v)", w.interval)
	return nil
}

// Stop gracefully stops the watcher.
func (w *SchedulerWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.logger("Scheduler watcher stopped")
}

// run is the main watcher loop.
func (w *SchedulerWatcher) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Delay initial check to avoid thundering herd with other daemon tickers.
	// Random jitter of 0-30s prevents all periodic DB queries from firing at once.
	time.Sleep(jitterDuration(30 * time.Second))
	w.checkAll()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.checkAll()
		}
	}
}

// checkAll runs all health checks.
// Fetches the scheduler queue once and reuses it for both depth and age checks.
// Stranded convoy check is removed — ConvoyManager already scans every 30s.
func (w *SchedulerWatcher) checkAll() {
	items := w.fetchSchedulerQueue()
	if items == nil {
		return
	}
	w.checkQueueDepth(items)
	w.checkOldestPending(items)
}

// fetchSchedulerQueue fetches the scheduler queue once for reuse across checks.
func (w *SchedulerWatcher) fetchSchedulerQueue() []map[string]interface{} {
	cmd := exec.CommandContext(w.ctx, w.gtPath, "scheduler", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		w.logger("Scheduler watcher: failed to list queue: %v", err)
		return nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(out, &items); err != nil {
		w.logger("Scheduler watcher: failed to parse queue: %v", err)
		return nil
	}
	return items
}

// checkQueueDepth alerts if scheduler queue exceeds threshold.
func (w *SchedulerWatcher) checkQueueDepth(items []map[string]interface{}) {
	if len(items) > w.thresholds.QueueDepth {
		w.sendAlert("queue-depth", "SCHEDULER: Queue depth high",
			fmt.Sprintf("Scheduler queue has %d pending items (threshold: %d)\n\nRun 'gt scheduler list' to see pending work.", len(items), w.thresholds.QueueDepth))
	}
}

// checkOldestPending alerts if oldest pending item exceeds age threshold.
func (w *SchedulerWatcher) checkOldestPending(items []map[string]interface{}) {
	if len(items) == 0 {
		return
	}

	// Find oldest item (assume items sorted by created_at, first is oldest)
	if item, ok := items[0]["created_at"].(string); ok {
		created, err := time.Parse(time.RFC3339, item)
		if err != nil {
			return
		}
		age := time.Since(created)
		if age > w.thresholds.PendingAge {
			w.sendAlert("pending-age", "SCHEDULER: Old pending work",
				fmt.Sprintf("Oldest pending item is %v old (threshold: %v)\n\nItem may be stuck or blocked.\nRun 'gt scheduler list' to investigate.", age.Round(time.Minute), w.thresholds.PendingAge))
		}
	}
}

// sendAlert sends a mail alert if not recently sent (deduplication).
func (w *SchedulerWatcher) sendAlert(slot, subject, message string) error {
	// Use notification manager for deduplication
	shouldSend, err := w.notify.SendIfReady("scheduler", slot, message)
	if err != nil {
		w.logger("Scheduler watcher: notification check failed: %v", err)
		return err
	}
	if !shouldSend {
		w.logger("Scheduler watcher: skipping alert (recently sent): %s", slot)
		return nil
	}

	// Send mail to mayor
	cmd := exec.CommandContext(w.ctx, w.gtPath, "mail", "send", "mayor/", "-s", subject, "-m", message)
	cmd.Dir = w.townRoot
	if err := cmd.Run(); err != nil {
		w.logger("Scheduler watcher: failed to send mail: %v", err)
		return err
	}

	w.logger("Scheduler watcher: sent alert: %s", subject)
	return nil
}
