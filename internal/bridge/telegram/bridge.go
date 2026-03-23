package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Bridge is the main lifecycle manager that wires all Telegram bridge
// components together. Use NewBridge to construct one, Run to start it,
// and Stop to shut it down.
type Bridge struct {
	cfg         Config
	sender      Sender
	townRoot    string
	logger      *log.Logger
	inboxReader InboxReader // Optional: injected by daemon, nil = use CLIInboxReader
	msgMap      *MessageMap // persists across reconnects to avoid duplicate sends

	mu     sync.Mutex
	cancel context.CancelFunc
}

// NewBridge creates a Bridge. If sender is nil, a CLISender backed by
// townRoot is used as the default.
func NewBridge(cfg Config, sender Sender, townRoot string) *Bridge {
	if sender == nil {
		sender = NewCLISender(townRoot)
	}
	return &Bridge{
		cfg:      cfg,
		sender:   sender,
		townRoot: townRoot,
		logger:   log.Default(),
		msgMap:   NewMessageMap(10000),
	}
}

// SetLogger sets the logger for the bridge. When running inside the daemon,
// this should be the daemon's file-backed logger so bridge errors appear in
// daemon.log instead of being lost to stderr.
func (b *Bridge) SetLogger(l *log.Logger) {
	b.logger = l
}

// Run validates the config, then enters a retry loop calling runOnce.
// If runOnce returns an error and ctx is not cancelled, it logs the error,
// waits 5 seconds, and retries. Run blocks until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	if err := b.cfg.Validate(); err != nil {
		return fmt.Errorf("bridge: config invalid: %w", err)
	}

	for {
		err := b.runOnce(ctx)
		if ctx.Err() != nil {
			// Context was cancelled — clean shutdown.
			return ctx.Err()
		}
		if err != nil {
			b.logger.Printf("telegram bridge: run error (retrying in 5s): %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// Stop cancels the current run cycle. It is safe to call from any goroutine.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
	}
}

// safeGo runs fn with panic recovery so a panicking goroutine doesn't
// crash the entire bridge. Panics are logged and the goroutine exits cleanly.
func (b *Bridge) safeGo(wg *sync.WaitGroup, name string, fn func()) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			b.logger.Printf("telegram bridge: PANIC in %s (recovered): %v", name, r)
		}
	}()
	fn()
}

// runOnce performs a single connection-to-shutdown cycle:
//  1. Recovers from panics.
//  2. Connects to Telegram.
//  3. Starts the outbound notifier and reply forwarder in background goroutines.
//  4. Reads inbound messages from the bot and relays them until ctx is cancelled.
func (b *Bridge) runOnce(ctx context.Context) (retErr error) {
	// Recover from panics so the outer retry loop can restart cleanly.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("telegram bridge: panic: %v", r)
		}
	}()

	runCtx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	b.cancel = cancel
	b.mu.Unlock()
	defer cancel()

	// Connect to Telegram.
	bot, err := NewBot(b.cfg)
	if err != nil {
		return fmt.Errorf("telegram bridge: connect: %w", err)
	}

	inbound := NewInboundRelay(b.sender, b.msgMap, b.cfg.Target)

	feedPath := filepath.Join(b.townRoot, ".feed.jsonl")
	outbound := NewOutboundNotifier(feedPath, b.cfg.Notify, bot, b.msgMap)

	b.mu.Lock()
	if b.inboxReader == nil {
		cliReader := NewCLIInboxReader(b.townRoot)
		if err := cliReader.SeedForwarded(runCtx); err != nil {
			b.logger.Printf("telegram bridge: seed forwarded (non-fatal): %v", err)
		}
		b.inboxReader = cliReader
	}
	inboxReader := b.inboxReader
	b.mu.Unlock()

	replyFwd := NewReplyForwarder(bot, inboxReader, b.msgMap, b.logger)

	var wg sync.WaitGroup

	wg.Add(1)
	go b.safeGo(&wg, "bot.Poll", func() { bot.Poll(runCtx) })

	wg.Add(1)
	go b.safeGo(&wg, "outbound", func() { outbound.Run(runCtx) })

	wg.Add(1)
	go b.safeGo(&wg, "replyFwd", func() { replyFwd.Run(runCtx) })

	wg.Add(1)
	go b.safeGo(&wg, "inboundCleanup", func() { b.cleanupInboundBeads(runCtx) })

	// Main loop: relay inbound messages until context is cancelled.
	for {
		select {
		case <-runCtx.Done():
			wg.Wait()
			return nil
		case msg, ok := <-bot.Messages():
			if !ok {
				wg.Wait()
				return nil
			}
			if err := inbound.Relay(runCtx, msg); err != nil {
				b.logger.Printf("telegram bridge: relay error: %v", err)
			}
		}
	}
}

// cleanupInboundBeads periodically closes Telegram inbound beads that have
// been delivered. Without this, every inbound Telegram message leaves an open
// wisp bead assigned to the target (mayor/), polluting the issue queue.
// Runs every 60 seconds, closing beads older than 30 seconds to give the
// recipient time to read them via gt mail check --inject.
func (b *Bridge) cleanupInboundBeads(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.closeDeliveredInbound(ctx)
		}
	}
}

// closeDeliveredInbound finds and closes open Telegram inbound beads
// assigned to the configured target that are older than 30 seconds.
func (b *Bridge) closeDeliveredInbound(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "bd", "list",
		"--assignee", b.cfg.Target,
		"--label", "gt:message",
		"--label", "from:overseer",
		"--include-infra",
		"--json",
		"--no-pager",
	)
	cmd.Dir = b.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")

	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return
	}

	var issues []struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return
	}

	cutoff := time.Now().Add(-30 * time.Second)
	closed := 0
	for _, iss := range issues {
		created, err := time.Parse(time.RFC3339, iss.CreatedAt)
		if err != nil || created.After(cutoff) {
			continue // too recent, let the mayor read it first
		}
		closeCmd := exec.CommandContext(ctx, "bd", "close", iss.ID)
		closeCmd.Dir = b.townRoot
		closeCmd.Env = append(os.Environ(), "BD_ACTOR=overseer")
		if _, err := closeCmd.CombinedOutput(); err == nil {
			closed++
		}
	}
	if closed > 0 {
		b.logger.Printf("telegram bridge: closed %d delivered inbound bead(s)", closed)
	}
}
