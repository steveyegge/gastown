package telegram

import (
	"testing"
	"time"
)

// --- InboundMessage tests ---

func TestInboundMessage_Fields(t *testing.T) {
	replyTo := 42
	msg := InboundMessage{
		ChatID:       -100123456789,
		MessageID:    int64(7),
		Text:         "hello world",
		Username:     "alice",
		UserID:       99991,
		ReplyToMsgID: &replyTo,
	}

	if msg.ChatID != -100123456789 {
		t.Errorf("ChatID: got %d, want -100123456789", msg.ChatID)
	}
	if msg.MessageID != 7 {
		t.Errorf("MessageID: got %d, want 7", msg.MessageID)
	}
	if msg.Text != "hello world" {
		t.Errorf("Text: got %q, want %q", msg.Text, "hello world")
	}
	if msg.Username != "alice" {
		t.Errorf("Username: got %q, want %q", msg.Username, "alice")
	}
	if msg.UserID != 99991 {
		t.Errorf("UserID: got %d, want 99991", msg.UserID)
	}
	if msg.ReplyToMsgID == nil || *msg.ReplyToMsgID != 42 {
		t.Errorf("ReplyToMsgID: got %v, want ptr to 42", msg.ReplyToMsgID)
	}
}

func TestInboundMessage_NilReply(t *testing.T) {
	msg := InboundMessage{
		ChatID:    123,
		MessageID: int64(1),
		Text:      "no reply",
		Username:  "bob",
		UserID:    12345,
	}
	if msg.ReplyToMsgID != nil {
		t.Errorf("ReplyToMsgID should be nil for non-reply messages")
	}
}

// --- AccessGate tests ---

func TestAccessGate_AllowedUser(t *testing.T) {
	cfg := Config{
		Token:     "123456:ABCdef-ghijklmnop",
		ChatID:    -100111,
		AllowFrom: []int64{111, 222, 333},
	}
	gate := NewAccessGate(cfg)

	if !gate.Check(111, false) {
		t.Error("user 111 should be allowed")
	}
	if !gate.Check(222, false) {
		t.Error("user 222 should be allowed")
	}
	if !gate.Check(333, false) {
		t.Error("user 333 should be allowed")
	}
}

func TestAccessGate_BlockedUser(t *testing.T) {
	cfg := Config{
		Token:     "123456:ABCdef-ghijklmnop",
		ChatID:    -100111,
		AllowFrom: []int64{111},
	}
	gate := NewAccessGate(cfg)

	if gate.Check(999, false) {
		t.Error("user 999 is not in AllowFrom, should be blocked")
	}
}

func TestAccessGate_EmptyAllowFromBlocksAll(t *testing.T) {
	cfg := Config{
		Token:     "123456:ABCdef-ghijklmnop",
		ChatID:    -100111,
		AllowFrom: nil,
	}
	gate := NewAccessGate(cfg)

	if gate.Check(111, false) {
		t.Error("empty AllowFrom should block everyone (fail-closed)")
	}
}

func TestAccessGate_BotsAlwaysBlocked(t *testing.T) {
	cfg := Config{
		Token:     "123456:ABCdef-ghijklmnop",
		ChatID:    -100111,
		AllowFrom: []int64{111, 222},
	}
	gate := NewAccessGate(cfg)

	// Even if userID is in allow list, bots are rejected first
	if gate.Check(111, true) {
		t.Error("bots should always be blocked, even if userID is in AllowFrom")
	}
	if gate.Check(999, true) {
		t.Error("bots should always be blocked")
	}
}

// --- RateLimiter tests ---

func TestRateLimiter_AllowsUpToLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		if !rl.Allow(42) {
			t.Errorf("call %d should be allowed (limit=3)", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)

	for i := 0; i < 3; i++ {
		rl.Allow(42)
	}

	if rl.Allow(42) {
		t.Error("4th call within window should be blocked")
	}
}

func TestRateLimiter_SeparateLimitsPerUser(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// Exhaust user 1's limit
	rl.Allow(1)
	rl.Allow(1)
	if rl.Allow(1) {
		t.Error("user 1: 3rd call should be blocked")
	}

	// User 2 should still have their own fresh limit
	if !rl.Allow(2) {
		t.Error("user 2: 1st call should be allowed (separate limit)")
	}
	if !rl.Allow(2) {
		t.Error("user 2: 2nd call should be allowed (separate limit)")
	}
	if rl.Allow(2) {
		t.Error("user 2: 3rd call should be blocked")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	window := 50 * time.Millisecond
	rl := NewRateLimiter(2, window)

	rl.Allow(7)
	rl.Allow(7)
	if rl.Allow(7) {
		t.Error("3rd call should be blocked immediately after exhausting limit")
	}

	// Wait for window to expire
	time.Sleep(window + 10*time.Millisecond)

	if !rl.Allow(7) {
		t.Error("after window expiry, calls should be allowed again")
	}
}

func TestRateLimiter_ZeroLimitBlocksAll(t *testing.T) {
	rl := NewRateLimiter(0, time.Minute)
	if rl.Allow(1) {
		t.Error("limit=0 should block all calls")
	}
}
