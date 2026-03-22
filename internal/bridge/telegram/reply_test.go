package telegram

import (
	"context"
	"testing"
)

// --- mock types ---

type mockBotSender struct {
	sent   []sentMessage
	nextID int
}

type sentMessage struct {
	text         string
	replyToMsgID *int
}

func (m *mockBotSender) SendMessage(text string, replyToMsgID *int) (int, error) {
	m.nextID++
	m.sent = append(m.sent, sentMessage{text, replyToMsgID})
	return m.nextID, nil
}

type mockInboxReader struct {
	messages []InboxMessage
	markedRead []string
}

func (m *mockInboxReader) UnreadMessages(_ context.Context) ([]InboxMessage, error) {
	msgs := m.messages
	m.messages = nil
	return msgs, nil
}

func (m *mockInboxReader) MarkRead(_ context.Context, id string) error {
	m.markedRead = append(m.markedRead, id)
	return nil
}

// --- tests ---

func TestReplyForwarder_ForwardsMayorReply(t *testing.T) {
	bot := &mockBotSender{}
	inbox := &mockInboxReader{
		messages: []InboxMessage{
			{
				ID:       "msg-1",
				From:     "mayor",
				Subject:  "Re: your question",
				Body:     "Here is my answer",
				ThreadID: "thread-42",
			},
		},
	}
	msgMap := NewMessageMap(100)
	// Pre-seed the map: thread-42 was started by Telegram message 99 in chat 1001.
	msgMap.Store(1001, 99, "thread-42")

	rf := NewReplyForwarder(bot, inbox, msgMap)
	rf.PollOnce(context.Background())

	if len(bot.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(bot.sent))
	}
	msg := bot.sent[0]

	// Body must appear in the forwarded text.
	if !containsStr(msg.text, "Here is my answer") {
		t.Errorf("expected body in text, got %q", msg.text)
	}
	// From must appear in the forwarded text.
	if !containsStr(msg.text, "mayor") {
		t.Errorf("expected sender in text, got %q", msg.text)
	}

	// Reply threading: should reply to the original Telegram message.
	if msg.replyToMsgID == nil {
		t.Fatal("expected replyToMsgID to be set for threaded reply")
	}
	if *msg.replyToMsgID != 99 {
		t.Errorf("expected replyToMsgID=99, got %d", *msg.replyToMsgID)
	}

	// Message should be marked read.
	if len(inbox.markedRead) != 1 || inbox.markedRead[0] != "msg-1" {
		t.Errorf("expected msg-1 to be marked read, got %v", inbox.markedRead)
	}
}

func TestReplyForwarder_NoUnreadMessages(t *testing.T) {
	bot := &mockBotSender{}
	inbox := &mockInboxReader{} // no messages
	msgMap := NewMessageMap(100)

	rf := NewReplyForwarder(bot, inbox, msgMap)
	rf.PollOnce(context.Background())

	if len(bot.sent) != 0 {
		t.Errorf("expected no messages sent, got %d", len(bot.sent))
	}
	if len(inbox.markedRead) != 0 {
		t.Errorf("expected nothing marked read, got %v", inbox.markedRead)
	}
}

func TestReplyForwarder_NoThreadSendsWithoutReplyTo(t *testing.T) {
	bot := &mockBotSender{}
	inbox := &mockInboxReader{
		messages: []InboxMessage{
			{
				ID:      "msg-2",
				From:    "mayor",
				Subject: "Unsolicited update",
				Body:    "FYI something happened",
				// ThreadID intentionally empty — no thread context
			},
		},
	}
	msgMap := NewMessageMap(100)

	rf := NewReplyForwarder(bot, inbox, msgMap)
	rf.PollOnce(context.Background())

	if len(bot.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(bot.sent))
	}
	if bot.sent[0].replyToMsgID != nil {
		t.Errorf("expected no replyToMsgID, got %v", bot.sent[0].replyToMsgID)
	}
	if !containsStr(bot.sent[0].text, "FYI something happened") {
		t.Errorf("expected body in text, got %q", bot.sent[0].text)
	}

	// Should still be marked read.
	if len(inbox.markedRead) != 1 || inbox.markedRead[0] != "msg-2" {
		t.Errorf("expected msg-2 to be marked read, got %v", inbox.markedRead)
	}
}

// containsStr is a simple substring check helper.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
