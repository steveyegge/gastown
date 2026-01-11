// Package mail provides messaging for agent communication via beads.
package mail

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Priority levels for messages.
type Priority string

const (
	// PriorityLow is for non-urgent messages.
	PriorityLow Priority = "low"

	// PriorityNormal is the default priority.
	PriorityNormal Priority = "normal"

	// PriorityHigh indicates an important message.
	PriorityHigh Priority = "high"

	// PriorityUrgent indicates an urgent message requiring immediate attention.
	PriorityUrgent Priority = "urgent"
)

// MessageType indicates the purpose of a message.
type MessageType string


const (
	// TypeTask indicates a message requiring action from the recipient.
	TypeTask MessageType = "task"

	// TypeScavenge indicates optional first-come-first-served work.
	TypeScavenge MessageType = "scavenge"

	// TypeNotification is an informational message (default).
	TypeNotification MessageType = "notification"

	// TypeReply is a response to another message.
	TypeReply MessageType = "reply"
)

// Delivery specifies how a message is delivered to the recipient.
type Delivery string

const (
	// DeliveryQueue creates the message in the mailbox for periodic checking.
	// This is the default delivery mode. Agent checks with `gt mail check`.
	DeliveryQueue Delivery = "queue"

	// DeliveryInterrupt injects a system-reminder directly into the agent's session.
	// Use for lifecycle events, URGENT priority, or stuck detection.
	DeliveryInterrupt Delivery = "interrupt"
)

// Message represents a mail message between agents.
// This is the GGT-side representation; it gets translated to/from beads messages.
type Message struct {
	// ID is a unique message identifier (beads issue ID like "bd-abc123").
	ID string `json:"id"`

	// From is the sender address (e.g., "gastown/Toast" or "mayor/").
	From string `json:"from"`

	// To is the recipient address.
	To string `json:"to"`

	// Subject is a brief summary.
	Subject string `json:"subject"`

	// Body is the full message content.
	Body string `json:"body"`

	// Timestamp is when the message was sent.
	Timestamp time.Time `json:"timestamp"`

	// Read indicates if the message has been read (closed in beads).
	Read bool `json:"read"`

	// Priority is the message priority.
	Priority Priority `json:"priority"`

	// Type indicates the message type (task, scavenge, notification, reply).
	Type MessageType `json:"type"`

	// Delivery specifies how the message is delivered (queue or interrupt).
	// Queue: agent checks periodically. Interrupt: inject into session.
	Delivery Delivery `json:"delivery,omitempty"`

	// ThreadID groups related messages into a conversation thread.
	ThreadID string `json:"thread_id,omitempty"`

	// ReplyTo is the ID of the message this is replying to.
	ReplyTo string `json:"reply_to,omitempty"`

	// Pinned marks the message as pinned (won't be auto-archived).
	Pinned bool `json:"pinned,omitempty"`

	// Wisp marks this as a transient message (stored in same DB but filtered from JSONL export).
	// Wisp messages auto-cleanup on patrol squash.
	Wisp bool `json:"wisp,omitempty"`

	// CC contains addresses that should receive a copy of this message.
	// CC'd recipients see the message in their inbox but are not the primary recipient.
	CC []string `json:"cc,omitempty"`
}

// NewMessage creates a new message with a generated ID and thread ID.
func NewMessage(from, to, subject, body string) *Message {
	return &Message{
		ID:        generateID(),
		From:      from,
		To:        to,
		Subject:   subject,
		Body:      body,
		Timestamp: time.Now(),
		Read:      false,
		Priority:  PriorityNormal,
		Type:      TypeNotification,
		ThreadID:  generateThreadID(),
	}
}

// NewReplyMessage creates a reply message that inherits the thread from the original.
func NewReplyMessage(from, to, subject, body string, original *Message) *Message {
	return &Message{
		ID:        generateID(),
		From:      from,
		To:        to,
		Subject:   subject,
		Body:      body,
		Timestamp: time.Now(),
		Read:      false,
		Priority:  PriorityNormal,
		Type:      TypeReply,
		ThreadID:  original.ThreadID,
		ReplyTo:   original.ID,
	}
}

// generateID creates a random message ID.
// Falls back to time-based ID if crypto/rand fails (extremely rare).
func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based ID instead of panicking
		return fmt.Sprintf("msg-%x", time.Now().UnixNano())
	}
	return "msg-" + hex.EncodeToString(b)
}

// generateThreadID creates a random thread ID.
// Falls back to time-based ID if crypto/rand fails (extremely rare).
func generateThreadID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based ID instead of panicking
		return fmt.Sprintf("thread-%x", time.Now().UnixNano())
	}
	return "thread-" + hex.EncodeToString(b)
}

// BeadsMessage represents a message as returned by bd list/show commands.
// Messages are beads issues with type=message and metadata stored in labels.
type BeadsMessage struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`       // Subject
	Description string    `json:"description"` // Body
	Assignee    string    `json:"assignee"`    // To identity
	Priority    int       `json:"priority"`    // 0=urgent, 1=high, 2=normal, 3=low
	Status      string    `json:"status"`      // open=unread, closed=read
	CreatedAt   time.Time `json:"created_at"`
	Labels      []string  `json:"labels"` // Metadata labels (from:X, thread:X, reply-to:X, msg-type:X, cc:X)
	Pinned      bool      `json:"pinned,omitempty"`
	Wisp        bool      `json:"wisp,omitempty"` // Ephemeral message (filtered from JSONL export)

	// Cached parsed values (populated by ParseLabels)
	sender   string
	threadID string
	replyTo  string
	msgType  string
	cc       []string // CC recipients
}

// ParseLabels extracts metadata from the labels array.
func (bm *BeadsMessage) ParseLabels() {
	for _, label := range bm.Labels {
		if strings.HasPrefix(label, "from:") {
			bm.sender = strings.TrimPrefix(label, "from:")
		} else if strings.HasPrefix(label, "thread:") {
			bm.threadID = strings.TrimPrefix(label, "thread:")
		} else if strings.HasPrefix(label, "reply-to:") {
			bm.replyTo = strings.TrimPrefix(label, "reply-to:")
		} else if strings.HasPrefix(label, "msg-type:") {
			bm.msgType = strings.TrimPrefix(label, "msg-type:")
		} else if strings.HasPrefix(label, "cc:") {
			bm.cc = append(bm.cc, strings.TrimPrefix(label, "cc:"))
		}
	}
}

// GetCC returns the parsed CC recipients.
func (bm *BeadsMessage) GetCC() []string {
	return bm.cc
}

// IsCCRecipient checks if the given identity is in the CC list.
func (bm *BeadsMessage) IsCCRecipient(identity string) bool {
	for _, cc := range bm.cc {
		if cc == identity {
			return true
		}
	}
	return false
}

// ToMessage converts a BeadsMessage to a GGT Message.
func (bm *BeadsMessage) ToMessage() *Message {
	// Parse labels to extract metadata
	bm.ParseLabels()

	// Convert beads priority (0=urgent, 1=high, 2=normal, 3=low) to GGT Priority
	var priority Priority
	switch bm.Priority {
	case 0:
		priority = PriorityUrgent
	case 1:
		priority = PriorityHigh
	case 3:
		priority = PriorityLow
	default:
		priority = PriorityNormal
	}

	// Convert message type, default to notification
	msgType := TypeNotification
	switch MessageType(bm.msgType) {
	case TypeTask, TypeScavenge, TypeReply:
		msgType = MessageType(bm.msgType)
	}

	// Convert CC identities to addresses
	var ccAddrs []string
	for _, cc := range bm.cc {
		ccAddrs = append(ccAddrs, identityToAddress(cc))
	}

	return &Message{
		ID:        bm.ID,
		From:      identityToAddress(bm.sender),
		To:        identityToAddress(bm.Assignee),
		Subject:   bm.Title,
		Body:      bm.Description,
		Timestamp: bm.CreatedAt,
		Read:      bm.Status == "closed" || bm.HasLabel("read"),
		Priority:  priority,
		Type:      msgType,
		ThreadID:  bm.threadID,
		ReplyTo:   bm.replyTo,
		Wisp:      bm.Wisp,
		CC:        ccAddrs,
	}
}

// HasLabel checks if the message has a specific label.
func (bm *BeadsMessage) HasLabel(label string) bool {
	for _, l := range bm.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// PriorityToBeads converts a GGT Priority to beads priority integer.
// Returns: 0=urgent, 1=high, 2=normal, 3=low
func PriorityToBeads(p Priority) int {
	switch p {
	case PriorityUrgent:
		return 0
	case PriorityHigh:
		return 1
	case PriorityLow:
		return 3
	default:
		return 2 // normal
	}
}

// ParsePriority parses a priority string, returning PriorityNormal for invalid values.
func ParsePriority(s string) Priority {
	switch Priority(s) {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return Priority(s)
	default:
		return PriorityNormal
	}
}

// PriorityFromInt converts a beads-style integer priority to a Priority.
// Accepts: 0=urgent, 1=high, 2=normal, 3=low, 4=backlog (treated as low).
// Invalid values default to PriorityNormal.
func PriorityFromInt(p int) Priority {
	switch p {
	case 0:
		return PriorityUrgent
	case 1:
		return PriorityHigh
	case 2:
		return PriorityNormal
	case 3, 4:
		return PriorityLow
	default:
		return PriorityNormal
	}
}

// ParseMessageType parses a message type string, returning TypeNotification for invalid values.
func ParseMessageType(s string) MessageType {
	switch MessageType(s) {
	case TypeTask, TypeScavenge, TypeNotification, TypeReply:
		return MessageType(s)
	default:
		return TypeNotification
	}
}

// addressToIdentity converts a GGT address to a beads identity.
//
// Liberal normalization: accepts multiple address formats and normalizes
// to canonical form (Postel's Law - be liberal in what you accept).
//
// Addresses use slash format:
//   - "overseer" → "overseer" (human operator, no trailing slash)
//   - "mayor/" → "mayor/"
//   - "mayor" → "mayor/"
//   - "deacon/" → "deacon/"
//   - "deacon" → "deacon/"
//   - "gastown/polecats/Toast" → "gastown/Toast" (normalized)
//   - "gastown/crew/max" → "gastown/max" (normalized)
//   - "gastown/Toast" → "gastown/Toast" (already canonical)
//   - "gastown/refinery" → "gastown/refinery"
//   - "gastown/" → "gastown" (rig broadcast)
func addressToIdentity(address string) string {
	// Overseer (human operator) - no trailing slash, distinct from agents
	if address == "overseer" {
		return "overseer"
	}

	// Town-level agents: mayor and deacon keep trailing slash
	if address == "mayor" || address == "mayor/" {
		return "mayor/"
	}
	if address == "deacon" || address == "deacon/" {
		return "deacon/"
	}

	// Trim trailing slash for rig-level addresses
	if len(address) > 0 && address[len(address)-1] == '/' {
		address = address[:len(address)-1]
	}

	// Normalize crew/ and polecats/ to canonical form:
	// "rig/crew/name" → "rig/name"
	// "rig/polecats/name" → "rig/name"
	parts := strings.Split(address, "/")
	if len(parts) == 3 && (parts[1] == "crew" || parts[1] == "polecats") {
		return parts[0] + "/" + parts[2]
	}

	return address
}

// identityToAddress converts a beads identity back to a GGT address.
//
// Liberal normalization (Postel's Law):
//   - "overseer" → "overseer" (human operator)
//   - "mayor/" → "mayor/"
//   - "deacon/" → "deacon/"
//   - "gastown/polecats/Toast" → "gastown/Toast" (normalized)
//   - "gastown/crew/max" → "gastown/max" (normalized)
//   - "gastown/Toast" → "gastown/Toast" (already canonical)
//   - "gastown/refinery" → "gastown/refinery"
func identityToAddress(identity string) string {
	// Overseer (human operator) - no trailing slash
	if identity == "overseer" {
		return "overseer"
	}

	// Town-level agents ensure trailing slash
	if identity == "mayor" || identity == "mayor/" {
		return "mayor/"
	}
	if identity == "deacon" || identity == "deacon/" {
		return "deacon/"
	}

	// Normalize crew/ and polecats/ to canonical form
	parts := strings.Split(identity, "/")
	if len(parts) == 3 && (parts[1] == "crew" || parts[1] == "polecats") {
		return parts[0] + "/" + parts[2]
	}

	return identity
}
