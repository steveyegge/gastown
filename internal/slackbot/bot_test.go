package slackbot

import (
	"testing"
)

func TestNewBot_MissingBotToken(t *testing.T) {
	cfg := Config{
		AppToken: "xapp-test",
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for missing bot token")
	}
}

func TestNewBot_MissingAppToken(t *testing.T) {
	cfg := Config{
		BotToken: "xoxb-test",
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for missing app token")
	}
}

func TestNewBot_InvalidAppToken(t *testing.T) {
	cfg := Config{
		BotToken: "xoxb-test",
		AppToken: "invalid-token",
	}
	_, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid app token format")
	}
}

func TestNewBot_ValidConfig(t *testing.T) {
	cfg := Config{
		BotToken:    "xoxb-test-token",
		AppToken:    "xapp-test-token",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, err := New(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bot == nil {
		t.Error("expected bot to be created")
	}
}
