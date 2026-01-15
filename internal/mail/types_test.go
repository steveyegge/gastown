package mail

import (
	"testing"
	"time"
)

func TestAddressToIdentity(t *testing.T) {
	tests := []struct {
		address  string
		expected string
	}{
		// Town-level agents keep trailing slash
		{"mayor", "mayor/"},
		{"mayor/", "mayor/"},
		{"deacon", "deacon/"},
		{"deacon/", "deacon/"},

		// Rig-level agents: crew/ and polecats/ normalized to canonical form
		{"gastown/polecats/Toast", "gastown/Toast"},
		{"gastown/crew/max", "gastown/max"},
		{"gastown/Toast", "gastown/Toast"},         // Already canonical
		{"gastown/max", "gastown/max"},             // Already canonical
		{"gastown/refinery", "gastown/refinery"},
		{"gastown/witness", "gastown/witness"},

		// Rig broadcast (trailing slash removed)
		{"gastown/", "gastown"},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := addressToIdentity(tt.address)
			if got != tt.expected {
				t.Errorf("addressToIdentity(%q) = %q, want %q", tt.address, got, tt.expected)
			}
		})
	}
}

func TestIdentityToAddress(t *testing.T) {
	tests := []struct {
		identity string
		expected string
	}{
		// Town-level agents
		{"mayor", "mayor/"},
		{"mayor/", "mayor/"},
		{"deacon", "deacon/"},
		{"deacon/", "deacon/"},

		// Rig-level agents: crew/ and polecats/ normalized
		{"gastown/polecats/Toast", "gastown/Toast"},
		{"gastown/crew/max", "gastown/max"},
		{"gastown/Toast", "gastown/Toast"},  // Already canonical
		{"gastown/refinery", "gastown/refinery"},
		{"gastown/witness", "gastown/witness"},

		// Rig name only (no transformation)
		{"gastown", "gastown"},
	}

	for _, tt := range tests {
		t.Run(tt.identity, func(t *testing.T) {
			got := identityToAddress(tt.identity)
			if got != tt.expected {
				t.Errorf("identityToAddress(%q) = %q, want %q", tt.identity, got, tt.expected)
			}
		})
	}
}

func TestPriorityToBeads(t *testing.T) {
	tests := []struct {
		priority Priority
		expected int
	}{
		{PriorityUrgent, 0},
		{PriorityHigh, 1},
		{PriorityNormal, 2},
		{PriorityLow, 3},
		{Priority("unknown"), 2}, // Default to normal
	}

	for _, tt := range tests {
		t.Run(string(tt.priority), func(t *testing.T) {
			got := PriorityToBeads(tt.priority)
			if got != tt.expected {
				t.Errorf("PriorityToBeads(%q) = %d, want %d", tt.priority, got, tt.expected)
			}
		})
	}
}

func TestPriorityFromInt(t *testing.T) {
	tests := []struct {
		p        int
		expected Priority
	}{
		{0, PriorityUrgent},
		{1, PriorityHigh},
		{2, PriorityNormal},
		{3, PriorityLow},
		{4, PriorityLow},  // Out of range maps to low
		{-1, PriorityNormal}, // Negative maps to normal
	}

	for _, tt := range tests {
		got := PriorityFromInt(tt.p)
		if got != tt.expected {
			t.Errorf("PriorityFromInt(%d) = %q, want %q", tt.p, got, tt.expected)
		}
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		s        string
		expected Priority
	}{
		{"urgent", PriorityUrgent},
		{"high", PriorityHigh},
		{"normal", PriorityNormal},
		{"low", PriorityLow},
		{"unknown", PriorityNormal}, // Default
		{"", PriorityNormal},        // Empty
		{"URGENT", PriorityNormal},  // Case-sensitive, defaults to normal
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := ParsePriority(tt.s)
			if got != tt.expected {
				t.Errorf("ParsePriority(%q) = %q, want %q", tt.s, got, tt.expected)
			}
		})
	}
}

func TestParseMessageType(t *testing.T) {
	tests := []struct {
		s        string
		expected MessageType
	}{
		{"task", TypeTask},
		{"scavenge", TypeScavenge},
		{"notification", TypeNotification},
		{"reply", TypeReply},
		{"unknown", TypeNotification}, // Default
		{"", TypeNotification},        // Empty
		{"TASK", TypeNotification},    // Case-sensitive, defaults to notification
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := ParseMessageType(tt.s)
			if got != tt.expected {
				t.Errorf("ParseMessageType(%q) = %q, want %q", tt.s, got, tt.expected)
			}
		})
	}
}

func TestNewMessage(t *testing.T) {
	msg := NewMessage("mayor/", "gastown/Toast", "Test Subject", "Test Body")

	if msg.From != "mayor/" {
		t.Errorf("From = %q, want 'mayor/'", msg.From)
	}
	if msg.To != "gastown/Toast" {
		t.Errorf("To = %q, want 'gastown/Toast'", msg.To)
	}
	if msg.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want 'Test Subject'", msg.Subject)
	}
	if msg.Body != "Test Body" {
		t.Errorf("Body = %q, want 'Test Body'", msg.Body)
	}
	if msg.ID == "" {
		t.Error("ID should be generated")
	}
	if msg.ThreadID == "" {
		t.Error("ThreadID should be generated")
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
	if msg.Priority != PriorityNormal {
		t.Errorf("Priority = %q, want PriorityNormal", msg.Priority)
	}
	if msg.Type != TypeNotification {
		t.Errorf("Type = %q, want TypeNotification", msg.Type)
	}
}

func TestNewReplyMessage(t *testing.T) {
	original := &Message{
		ID:       "orig-001",
		ThreadID: "thread-001",
		From:     "gastown/Toast",
		To:       "mayor/",
		Subject:  "Original Subject",
	}

	reply := NewReplyMessage("mayor/", "gastown/Toast", "Re: Original Subject", "Reply body", original)

	if reply.ThreadID != "thread-001" {
		t.Errorf("ThreadID = %q, want 'thread-001'", reply.ThreadID)
	}
	if reply.ReplyTo != "orig-001" {
		t.Errorf("ReplyTo = %q, want 'orig-001'", reply.ReplyTo)
	}
	if reply.From != "mayor/" {
		t.Errorf("From = %q, want 'mayor/'", reply.From)
	}
	if reply.To != "gastown/Toast" {
		t.Errorf("To = %q, want 'gastown/Toast'", reply.To)
	}
	if reply.Subject != "Re: Original Subject" {
		t.Errorf("Subject = %q, want 'Re: Original Subject'", reply.Subject)
	}
}

func TestBeadsMessageToMessage(t *testing.T) {
	now := time.Now()
	bm := BeadsMessage{
		ID:          "hq-test",
		Title:       "Test Subject",
		Description: "Test Body",
		Status:      "open",
		Assignee:    "gastown/Toast",
		Labels:      []string{"from:mayor/", "thread:t-001"},
		CreatedAt:   now,
		Priority:    1,
	}

	msg := bm.ToMessage()

	if msg.ID != "hq-test" {
		t.Errorf("ID = %q, want 'hq-test'", msg.ID)
	}
	if msg.Subject != "Test Subject" {
		t.Errorf("Subject = %q, want 'Test Subject'", msg.Subject)
	}
	if msg.Body != "Test Body" {
		t.Errorf("Body = %q, want 'Test Body'", msg.Body)
	}
	if msg.From != "mayor/" {
		t.Errorf("From = %q, want 'mayor/'", msg.From)
	}
	if msg.ThreadID != "t-001" {
		t.Errorf("ThreadID = %q, want 't-001'", msg.ThreadID)
	}
	if msg.To != "gastown/Toast" {
		t.Errorf("To = %q, want 'gastown/Toast'", msg.To)
	}
	if msg.Priority != PriorityHigh {
		t.Errorf("Priority = %q, want PriorityHigh", msg.Priority)
	}
}

func TestBeadsMessageToMessageWithReplyTo(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-reply",
		Title:       "Reply Subject",
		Description: "Reply Body",
		Status:      "open",
		Assignee:    "gastown/Toast",
		Labels:      []string{"from:mayor/", "thread:t-002", "reply-to:orig-001", "msg-type:reply"},
		CreatedAt:   time.Now(),
		Priority:    2,
	}

	msg := bm.ToMessage()

	if msg.ReplyTo != "orig-001" {
		t.Errorf("ReplyTo = %q, want 'orig-001'", msg.ReplyTo)
	}
	if msg.Type != TypeReply {
		t.Errorf("Type = %q, want TypeReply", msg.Type)
	}
}

func TestBeadsMessageToMessagePriorities(t *testing.T) {
	tests := []struct {
		priority int
		expected Priority
	}{
		{0, PriorityUrgent},
		{1, PriorityHigh},
		{2, PriorityNormal},
		{3, PriorityLow},
		{4, PriorityNormal},  // Out of range defaults to normal
		{99, PriorityNormal}, // Out of range defaults to normal
	}

	for _, tt := range tests {
		bm := BeadsMessage{
			ID:       "hq-test",
			Priority: tt.priority,
		}
		msg := bm.ToMessage()
		if msg.Priority != tt.expected {
			t.Errorf("Priority %d -> %q, want %q", tt.priority, msg.Priority, tt.expected)
		}
	}
}

func TestBeadsMessageToMessageTypes(t *testing.T) {
	tests := []struct {
		msgType  string
		expected MessageType
	}{
		{"task", TypeTask},
		{"scavenge", TypeScavenge},
		{"reply", TypeReply},
		{"notification", TypeNotification},
		{"", TypeNotification}, // Default
	}

	for _, tt := range tests {
		bm := BeadsMessage{
			ID:     "hq-test",
			Labels: []string{"msg-type:" + tt.msgType},
		}
		msg := bm.ToMessage()
		if msg.Type != tt.expected {
			t.Errorf("msg-type:%s -> %q, want %q", tt.msgType, msg.Type, tt.expected)
		}
	}
}

func TestBeadsMessageToMessageEmptyLabels(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-empty",
		Title:       "Empty Labels",
		Description: "Test with empty labels",
		Assignee:    "gastown/Toast",
		Labels:      []string{}, // No labels
		Priority:    2,
	}

	msg := bm.ToMessage()

	if msg.From != "" {
		t.Errorf("From should be empty, got %q", msg.From)
	}
	if msg.ThreadID != "" {
		t.Errorf("ThreadID should be empty, got %q", msg.ThreadID)
	}
}

func TestNewQueueMessage(t *testing.T) {
	msg := NewQueueMessage("mayor/", "work-requests", "New Task", "Please process this")

	if msg.From != "mayor/" {
		t.Errorf("From = %q, want 'mayor/'", msg.From)
	}
	if msg.Queue != "work-requests" {
		t.Errorf("Queue = %q, want 'work-requests'", msg.Queue)
	}
	if msg.To != "" {
		t.Errorf("To should be empty for queue messages, got %q", msg.To)
	}
	if msg.Channel != "" {
		t.Errorf("Channel should be empty for queue messages, got %q", msg.Channel)
	}
	if msg.Type != TypeTask {
		t.Errorf("Type = %q, want TypeTask", msg.Type)
	}
	if msg.ID == "" {
		t.Error("ID should be generated")
	}
	if msg.ThreadID == "" {
		t.Error("ThreadID should be generated")
	}
}

func TestNewChannelMessage(t *testing.T) {
	msg := NewChannelMessage("deacon/", "alerts", "System Alert", "System is healthy")

	if msg.From != "deacon/" {
		t.Errorf("From = %q, want 'deacon/'", msg.From)
	}
	if msg.Channel != "alerts" {
		t.Errorf("Channel = %q, want 'alerts'", msg.Channel)
	}
	if msg.To != "" {
		t.Errorf("To should be empty for channel messages, got %q", msg.To)
	}
	if msg.Queue != "" {
		t.Errorf("Queue should be empty for channel messages, got %q", msg.Queue)
	}
	if msg.Type != TypeNotification {
		t.Errorf("Type = %q, want TypeNotification", msg.Type)
	}
}

func TestMessageIsQueueMessage(t *testing.T) {
	directMsg := NewMessage("mayor/", "gastown/Toast", "Test", "Body")
	queueMsg := NewQueueMessage("mayor/", "work-requests", "Task", "Body")
	channelMsg := NewChannelMessage("deacon/", "alerts", "Alert", "Body")

	if directMsg.IsQueueMessage() {
		t.Error("Direct message should not be a queue message")
	}
	if !queueMsg.IsQueueMessage() {
		t.Error("Queue message should be a queue message")
	}
	if channelMsg.IsQueueMessage() {
		t.Error("Channel message should not be a queue message")
	}
}

func TestMessageIsChannelMessage(t *testing.T) {
	directMsg := NewMessage("mayor/", "gastown/Toast", "Test", "Body")
	queueMsg := NewQueueMessage("mayor/", "work-requests", "Task", "Body")
	channelMsg := NewChannelMessage("deacon/", "alerts", "Alert", "Body")

	if directMsg.IsChannelMessage() {
		t.Error("Direct message should not be a channel message")
	}
	if queueMsg.IsChannelMessage() {
		t.Error("Queue message should not be a channel message")
	}
	if !channelMsg.IsChannelMessage() {
		t.Error("Channel message should be a channel message")
	}
}

func TestMessageIsDirectMessage(t *testing.T) {
	directMsg := NewMessage("mayor/", "gastown/Toast", "Test", "Body")
	queueMsg := NewQueueMessage("mayor/", "work-requests", "Task", "Body")
	channelMsg := NewChannelMessage("deacon/", "alerts", "Alert", "Body")

	if !directMsg.IsDirectMessage() {
		t.Error("Direct message should be a direct message")
	}
	if queueMsg.IsDirectMessage() {
		t.Error("Queue message should not be a direct message")
	}
	if channelMsg.IsDirectMessage() {
		t.Error("Channel message should not be a direct message")
	}
}

func TestMessageValidate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid direct message",
			msg:     NewMessage("mayor/", "gastown/Toast", "Test", "Body"),
			wantErr: false,
		},
		{
			name:    "valid queue message",
			msg:     NewQueueMessage("mayor/", "work-requests", "Task", "Body"),
			wantErr: false,
		},
		{
			name:    "valid channel message",
			msg:     NewChannelMessage("deacon/", "alerts", "Alert", "Body"),
			wantErr: false,
		},
		{
			name: "no routing target",
			msg: &Message{
				ID:      "msg-001",
				From:    "mayor/",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "must have exactly one of",
		},
		{
			name: "both to and queue",
			msg: &Message{
				ID:      "msg-001",
				From:    "mayor/",
				To:      "gastown/Toast",
				Queue:   "work-requests",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "both to and channel",
			msg: &Message{
				ID:      "msg-001",
				From:    "mayor/",
				To:      "gastown/Toast",
				Channel: "alerts",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "both queue and channel",
			msg: &Message{
				ID:      "msg-001",
				From:    "mayor/",
				Queue:   "work-requests",
				Channel: "alerts",
				Subject: "Test",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "claimed_by on non-queue message",
			msg: &Message{
				ID:        "msg-001",
				From:      "mayor/",
				To:        "gastown/Toast",
				Subject:   "Test",
				ClaimedBy: "gastown/nux",
			},
			wantErr: true,
			errMsg:  "claimed_by is only valid for queue messages",
		},
		{
			name: "claimed_by on queue message is valid",
			msg: &Message{
				ID:        "msg-001",
				From:      "mayor/",
				Queue:     "work-requests",
				Subject:   "Test",
				ClaimedBy: "gastown/nux",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBeadsMessageParseQueueChannelLabels(t *testing.T) {
	claimedTime := time.Date(2026, 1, 14, 12, 0, 0, 0, time.UTC)
	claimedAtStr := claimedTime.Format(time.RFC3339)

	bm := BeadsMessage{
		ID:          "hq-queue",
		Title:       "Queue Message",
		Description: "Test queue message",
		Status:      "open",
		Labels: []string{
			"from:mayor/",
			"queue:work-requests",
			"claimed-by:gastown/nux",
			"claimed-at:" + claimedAtStr,
		},
		Priority: 2,
	}

	msg := bm.ToMessage()

	if msg.Queue != "work-requests" {
		t.Errorf("Queue = %q, want 'work-requests'", msg.Queue)
	}
	if msg.ClaimedBy != "gastown/nux" {
		t.Errorf("ClaimedBy = %q, want 'gastown/nux'", msg.ClaimedBy)
	}
	if msg.ClaimedAt == nil {
		t.Error("ClaimedAt should not be nil")
	} else if !msg.ClaimedAt.Equal(claimedTime) {
		t.Errorf("ClaimedAt = %v, want %v", msg.ClaimedAt, claimedTime)
	}
}

func TestBeadsMessageParseChannelLabel(t *testing.T) {
	bm := BeadsMessage{
		ID:          "hq-channel",
		Title:       "Channel Message",
		Description: "Test channel message",
		Status:      "open",
		Labels:      []string{"from:deacon/", "channel:alerts"},
		Priority:    2,
	}

	msg := bm.ToMessage()

	if msg.Channel != "alerts" {
		t.Errorf("Channel = %q, want 'alerts'", msg.Channel)
	}
	if msg.Queue != "" {
		t.Errorf("Queue should be empty, got %q", msg.Queue)
	}
}

func TestBeadsMessageIsQueueMessage(t *testing.T) {
	queueMsg := BeadsMessage{
		ID:     "hq-queue",
		Labels: []string{"queue:work-requests"},
	}
	directMsg := BeadsMessage{
		ID:       "hq-direct",
		Assignee: "gastown/Toast",
	}
	channelMsg := BeadsMessage{
		ID:     "hq-channel",
		Labels: []string{"channel:alerts"},
	}

	if !queueMsg.IsQueueMessage() {
		t.Error("Queue message should be identified as queue message")
	}
	if directMsg.IsQueueMessage() {
		t.Error("Direct message should not be identified as queue message")
	}
	if channelMsg.IsQueueMessage() {
		t.Error("Channel message should not be identified as queue message")
	}
}

func TestBeadsMessageIsChannelMessage(t *testing.T) {
	queueMsg := BeadsMessage{
		ID:     "hq-queue",
		Labels: []string{"queue:work-requests"},
	}
	directMsg := BeadsMessage{
		ID:       "hq-direct",
		Assignee: "gastown/Toast",
	}
	channelMsg := BeadsMessage{
		ID:     "hq-channel",
		Labels: []string{"channel:alerts"},
	}

	if queueMsg.IsChannelMessage() {
		t.Error("Queue message should not be identified as channel message")
	}
	if directMsg.IsChannelMessage() {
		t.Error("Direct message should not be identified as channel message")
	}
	if !channelMsg.IsChannelMessage() {
		t.Error("Channel message should be identified as channel message")
	}
}

func TestBeadsMessageIsDirectMessage(t *testing.T) {
	queueMsg := BeadsMessage{
		ID:     "hq-queue",
		Labels: []string{"queue:work-requests"},
	}
	directMsg := BeadsMessage{
		ID:       "hq-direct",
		Assignee: "gastown/Toast",
	}
	channelMsg := BeadsMessage{
		ID:     "hq-channel",
		Labels: []string{"channel:alerts"},
	}

	if queueMsg.IsDirectMessage() {
		t.Error("Queue message should not be identified as direct message")
	}
	if !directMsg.IsDirectMessage() {
		t.Error("Direct message should be identified as direct message")
	}
	if channelMsg.IsDirectMessage() {
		t.Error("Channel message should not be identified as direct message")
	}
}

func TestMessageIsClaimed(t *testing.T) {
	unclaimed := NewQueueMessage("mayor/", "work-requests", "Task", "Body")
	if unclaimed.IsClaimed() {
		t.Error("Unclaimed message should not be claimed")
	}

	claimed := NewQueueMessage("mayor/", "work-requests", "Task", "Body")
	claimed.ClaimedBy = "gastown/nux"
	now := time.Now()
	claimed.ClaimedAt = &now

	if !claimed.IsClaimed() {
		t.Error("Claimed message should be claimed")
	}
}
