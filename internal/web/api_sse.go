package web

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// handleSSE streams Server-Sent Events to the dashboard client.
// It polls key dashboard state every 2 seconds and sends an event when
// changes are detected, allowing the client to trigger a re-render.
// Falls through gracefully if the client disconnects.
func (h *APIHandler) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: ok\n\n")
	flusher.Flush()

	var lastHash string
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Send keepalive comment every 15 seconds to prevent connection timeouts
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-ticker.C:
			hash := h.computeDashboardHash(ctx)
			if hash != "" && hash != lastHash {
				lastHash = hash
				fmt.Fprintf(w, "event: dashboard-update\ndata: %s\n\n", hash)
				flusher.Flush()
			}
		}
	}
}

// computeDashboardHash generates a lightweight hash of key dashboard state.
// It runs quick commands in parallel and hashes their output to detect changes.
func (h *APIHandler) computeDashboardHash(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var parts []string

	var wg sync.WaitGroup
	wg.Add(3)

	// Check worker/polecat state
	go func() {
		defer wg.Done()
		if out, err := h.runGtCommand(ctx, 3*time.Second, []string{"status", "--json"}); err == nil {
			mu.Lock()
			parts = append(parts, "status:"+out)
			mu.Unlock()
		}
	}()

	// Check hooks state
	go func() {
		defer wg.Done()
		if out, err := h.runGtCommand(ctx, 3*time.Second, []string{"hooks", "list"}); err == nil {
			mu.Lock()
			parts = append(parts, "hooks:"+out)
			mu.Unlock()
		}
	}()

	// Check mail count
	go func() {
		defer wg.Done()
		if out, err := h.runGtCommand(ctx, 3*time.Second, []string{"mail", "inbox"}); err == nil {
			mu.Lock()
			parts = append(parts, "mail:"+out)
			mu.Unlock()
		}
	}()

	wg.Wait()

	if len(parts) == 0 {
		return ""
	}

	h256 := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", h256[:8])
}
