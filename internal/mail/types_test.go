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
