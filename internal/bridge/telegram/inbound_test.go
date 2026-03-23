package telegram

import (
	"context"
	"testing"
)

// mockSender records all calls made to SendMail and Nudge.
type mockSender struct {
	mailCalls  []mailCall
	nudgeCalls []nudgeCall
	nudgeErr   error // if set, Nudge returns this error
}

type mailCall struct{ to, subject, body string }
type nudgeCall struct{ session, message string }

func (m *mockSender) SendMail(_ context.Context, to, subject, body string) error {
	m.mailCalls = append(m.mailCalls, mailCall{to, subject, body})
	return nil
}

func (m *mockSender) Nudge(_ context.Context, session, message string) error {
	m.nudgeCalls = append(m.nudgeCalls, nudgeCall{session, message})
	return m.nudgeErr
}

// helper to build a simple InboundMessage for tests.
func newMsg(text string) InboundMessage {
	return InboundMessage{
		ChatID:    100,
		MessageID: 1,
		Text:      text,
		Username:  "testuser",
		UserID:    42,
	}
}

func TestInboundRelay_RelaysMessageToMayor(t *testing.T) {
	sender := &mockSender{}
	mm := NewMessageMap(10)
	relay := NewInboundRelay(sender, mm, "mayor/")

	msg := newMsg("Hello from Telegram")
	if err := relay.Relay(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.mailCalls) != 1 {
		t.Fatalf("expected 1 mail call, got %d", len(sender.mailCalls))
	}
	mc := sender.mailCalls[0]
	if mc.to != "mayor/" {
		t.Errorf("mail to: got %q, want %q", mc.to, "mayor/")
	}
	if mc.subject != "Telegram" {
		t.Errorf("mail subject: got %q, want %q", mc.subject, "Telegram")
	}
	if mc.body != "Hello from Telegram" {
		t.Errorf("mail body: got %q, want %q", mc.body, "Hello from Telegram")
	}

	if len(sender.nudgeCalls) != 1 {
		t.Fatalf("expected 1 nudge call, got %d", len(sender.nudgeCalls))
	}
	nc := sender.nudgeCalls[0]
	if nc.session != "hq-mayor" {
		t.Errorf("nudge session: got %q, want %q", nc.session, "hq-mayor")
	}
}

func TestInboundRelay_SkipsEmptyText(t *testing.T) {
	sender := &mockSender{}
	mm := NewMessageMap(10)
	relay := NewInboundRelay(sender, mm, "mayor/")

	if err := relay.Relay(context.Background(), newMsg("")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.mailCalls) != 0 {
		t.Errorf("expected no mail calls for empty message, got %d", len(sender.mailCalls))
	}
	if len(sender.nudgeCalls) != 0 {
		t.Errorf("expected no nudge calls for empty message, got %d", len(sender.nudgeCalls))
	}
}

func TestInboundRelay_NudgeFailureIsNonFatal(t *testing.T) {
	sender := &mockSender{nudgeErr: context.DeadlineExceeded}
	mm := NewMessageMap(10)
	relay := NewInboundRelay(sender, mm, "mayor/")

	if err := relay.Relay(context.Background(), newMsg("Hello")); err != nil {
		t.Fatalf("nudge failure should be non-fatal, got error: %v", err)
	}

	if len(sender.mailCalls) != 1 {
		t.Errorf("mail should have been sent despite nudge failure")
	}
}

func TestInboundRelay_ReplyThreadingViaMsgMap(t *testing.T) {
	sender := &mockSender{}
	mm := NewMessageMap(10)
	// Simulate a prior message stored in the map.
	mm.Store(100, 5, "thread-abc-123")

	relay := NewInboundRelay(sender, mm, "mayor/")

	replyTo := 5
	msg := InboundMessage{
		ChatID:       100,
		MessageID:    10,
		Text:         "This is a reply",
		Username:     "testuser",
		UserID:       42,
		ReplyToMsgID: &replyTo,
	}

	if err := relay.Relay(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Relay should still send mail (thread context is for future use).
	if len(sender.mailCalls) != 1 {
		t.Fatalf("expected 1 mail call, got %d", len(sender.mailCalls))
	}
	if sender.mailCalls[0].body != "This is a reply" {
		t.Errorf("mail body: got %q, want %q", sender.mailCalls[0].body, "This is a reply")
	}
	if len(sender.nudgeCalls) != 1 {
		t.Errorf("expected 1 nudge call, got %d", len(sender.nudgeCalls))
	}
}
