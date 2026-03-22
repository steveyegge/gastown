package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBridgeShutdown(t *testing.T) {
	// Do NOT call bridge.Run() — it would make real API calls.
	// Instead test Stop() mechanism directly.
	cfg := Config{
		Token:     "123:AAHtest",
		ChatID:    123,
		AllowFrom: []int64{123},
		Target:    "mayor/",
		Enabled:   true,
		Notify:    []string{"escalations"},
		RateLimit: 30,
	}
	bridge := NewBridge(cfg, nil, "/tmp/test-town")

	ctx, cancel := context.WithCancel(context.Background())
	bridge.mu.Lock()
	bridge.cancel = cancel
	bridge.mu.Unlock()

	bridge.Stop()
	assert.Error(t, ctx.Err(), "context should be cancelled after Stop()")
}

func TestBridgeConfigValidation(t *testing.T) {
	// Invalid config should fail fast, before any API call.
	cfg := Config{} // missing token
	bridge := NewBridge(cfg, nil, "/tmp/test")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := bridge.Run(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}
