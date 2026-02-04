package mail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewMailbox(t *testing.T) {
	m := NewMailbox("/tmp/test")
	if filepath.ToSlash(m.path) != "/tmp/test/inbox.jsonl" {
		t.Errorf("NewMailbox path = %q, want %q", m.path, "/tmp/test/inbox.jsonl")
	}
	if !m.legacy {
		t.Error("NewMailbox should create legacy mailbox")
	}
}

func TestNewMailboxBeads(t *testing.T) {
	m := NewMailboxBeads("gastown/Toast", "/work/dir")
	if m.identity != "gastown/Toast" {
		t.Errorf("identity = %q, want %q", m.identity, "gastown/Toast")
	}
	if m.legacy {
		t.Error("NewMailboxBeads should not create legacy mailbox")
	}
}

func TestMailboxLegacyAppend(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msg := &Message{
		ID:        "msg-001",
		From:      "mayor/",
		To:        "gastown/Toast",
		Subject:   "Test message",
		Body:      "Hello world",
		Timestamp: time.Now(),
	}

	if err := m.Append(msg); err != nil {
		t.Fatalf("Append error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		t.Fatal("inbox.jsonl was not created")
	}

	// Verify content
	content, err := os.ReadFile(m.path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var readMsg Message
	if err := json.Unmarshal(content[:len(content)-1], &readMsg); err != nil { // -1 for newline
		t.Fatalf("Unmarshal error: %v", err)
	}

	if readMsg.ID != msg.ID {
		t.Errorf("ID = %q, want %q", readMsg.ID, msg.ID)
	}
}

func TestMailboxLegacyList(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	// Append multiple messages
	msgs := []*Message{
		{ID: "msg-001", Subject: "First", Timestamp: time.Now().Add(-2 * time.Hour)},
		{ID: "msg-002", Subject: "Second", Timestamp: time.Now().Add(-1 * time.Hour)},
		{ID: "msg-003", Subject: "Third", Timestamp: time.Now()},
	}

	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	// List should return newest first
	listed, err := m.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if len(listed) != 3 {
		t.Fatalf("List returned %d messages, want 3", len(listed))
	}

	// Verify order (newest first)
	if listed[0].ID != "msg-003" {
		t.Errorf("First message ID = %q, want msg-003 (newest)", listed[0].ID)
	}
	if listed[2].ID != "msg-001" {
		t.Errorf("Last message ID = %q, want msg-001 (oldest)", listed[2].ID)
	}
}

func TestMailboxLegacyGet(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msg := &Message{
		ID:      "msg-001",
		Subject: "Test",
		Body:    "Content",
	}
	if err := m.Append(msg); err != nil {
		t.Fatalf("Append error: %v", err)
	}

	// Get existing message
	got, err := m.Get("msg-001")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Subject != "Test" {
		t.Errorf("Subject = %q, want %q", got.Subject, "Test")
	}

	// Get non-existent message
	_, err = m.Get("msg-nonexistent")
	if err != ErrMessageNotFound {
		t.Errorf("Get non-existent = %v, want ErrMessageNotFound", err)
	}
}

func TestMailboxLegacyMarkRead(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msg := &Message{
		ID:   "msg-001",
		Read: false,
	}
	if err := m.Append(msg); err != nil {
		t.Fatalf("Append error: %v", err)
	}

	// Mark as read
	if err := m.MarkRead("msg-001"); err != nil {
		t.Fatalf("MarkRead error: %v", err)
	}

	// Verify it's now read
	got, err := m.Get("msg-001")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !got.Read {
		t.Error("Message should be marked as read")
	}

	// Mark non-existent
	err = m.MarkRead("msg-nonexistent")
	if err != ErrMessageNotFound {
		t.Errorf("MarkRead non-existent = %v, want ErrMessageNotFound", err)
	}
}

func TestMailboxLegacyDelete(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msgs := []*Message{
		{ID: "msg-001", Subject: "First"},
		{ID: "msg-002", Subject: "Second"},
	}
	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	// Delete one
	if err := m.Delete("msg-001"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// Verify only one remains
	listed, err := m.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List returned %d messages, want 1", len(listed))
	}
	if listed[0].ID != "msg-002" {
		t.Errorf("Remaining message ID = %q, want msg-002", listed[0].ID)
	}

	// Delete non-existent
	err = m.Delete("msg-nonexistent")
	if err != ErrMessageNotFound {
		t.Errorf("Delete non-existent = %v, want ErrMessageNotFound", err)
	}
}

func TestMailboxLegacyCount(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	// Empty inbox
	total, unread, err := m.Count()
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if total != 0 || unread != 0 {
		t.Errorf("Empty inbox count = (%d, %d), want (0, 0)", total, unread)
	}

	// Add messages
	msgs := []*Message{
		{ID: "msg-001", Read: false},
		{ID: "msg-002", Read: true},
		{ID: "msg-003", Read: false},
	}
	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	total, unread, err = m.Count()
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if unread != 2 {
		t.Errorf("unread = %d, want 2", unread)
	}
}

func TestMailboxLegacyListUnread(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msgs := []*Message{
		{ID: "msg-001", Read: false},
		{ID: "msg-002", Read: true},
		{ID: "msg-003", Read: false},
	}
	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	unread, err := m.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread error: %v", err)
	}
	if len(unread) != 2 {
		t.Errorf("ListUnread returned %d, want 2", len(unread))
	}
}

func TestMailboxMarkReadOnlyExcludesFromUnread(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msgs := []*Message{
		{ID: "msg-001", Read: false, Subject: "First"},
		{ID: "msg-002", Read: false, Subject: "Second"},
	}
	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	// Both should be unread initially
	unread, err := m.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread error: %v", err)
	}
	if len(unread) != 2 {
		t.Errorf("ListUnread returned %d, want 2", len(unread))
	}

	// Mark one as read-only (simulates gt mail read behavior)
	if err := m.MarkReadOnly("msg-001"); err != nil {
		t.Fatalf("MarkReadOnly error: %v", err)
	}

	// Should only have 1 unread now
	unread, err = m.ListUnread()
	if err != nil {
		t.Fatalf("ListUnread error: %v", err)
	}
	if len(unread) != 1 {
		t.Errorf("ListUnread returned %d after MarkReadOnly, want 1", len(unread))
	}
	if len(unread) == 1 && unread[0].ID != "msg-002" {
		t.Errorf("Expected msg-002 to be unread, got %s", unread[0].ID)
	}

	// The marked message should still be in full list
	all, err := m.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List returned %d, want 2 (MarkReadOnly should not remove)", len(all))
	}
}

func TestMailboxLegacyListByThread(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msgs := []*Message{
		{ID: "msg-001", ThreadID: "thread-A", Timestamp: time.Now().Add(-2 * time.Hour)},
		{ID: "msg-002", ThreadID: "thread-B", Timestamp: time.Now().Add(-1 * time.Hour)},
		{ID: "msg-003", ThreadID: "thread-A", Timestamp: time.Now()},
	}
	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	// Get thread A
	thread, err := m.ListByThread("thread-A")
	if err != nil {
		t.Fatalf("ListByThread error: %v", err)
	}
	if len(thread) != 2 {
		t.Fatalf("thread-A has %d messages, want 2", len(thread))
	}

	// Verify oldest first
	if thread[0].ID != "msg-001" {
		t.Errorf("First thread message = %q, want msg-001 (oldest)", thread[0].ID)
	}

	// Non-existent thread
	empty, err := m.ListByThread("thread-nonexistent")
	if err != nil {
		t.Fatalf("ListByThread error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("Non-existent thread has %d messages, want 0", len(empty))
	}
}

func TestMailboxLegacyEmptyInbox(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	// List on non-existent file should return empty, not error
	msgs, err := m.List()
	if err != nil {
		t.Fatalf("List on empty inbox error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Empty inbox returned %d messages, want 0", len(msgs))
	}
}

func TestMailboxBeadsAppendError(t *testing.T) {
	m := NewMailboxBeads("gastown/Toast", "/work/dir")

	err := m.Append(&Message{})
	if err == nil {
		t.Error("Append on beads mailbox should error")
	}
}

func TestMailboxIdentityAndPath(t *testing.T) {
	// Legacy mailbox
	legacy := NewMailbox("/tmp/test")
	if legacy.Identity() != "" {
		t.Errorf("Legacy mailbox identity = %q, want empty", legacy.Identity())
	}
	if filepath.ToSlash(legacy.Path()) != "/tmp/test/inbox.jsonl" {
		t.Errorf("Legacy mailbox path = %q, want /tmp/test/inbox.jsonl", legacy.Path())
	}

	// Beads mailbox
	beads := NewMailboxBeads("gastown/Toast", "/work/dir")
	if beads.Identity() != "gastown/Toast" {
		t.Errorf("Beads mailbox identity = %q, want gastown/Toast", beads.Identity())
	}
	if beads.Path() != "" {
		t.Errorf("Beads mailbox path = %q, want empty", beads.Path())
	}
}

func TestMailboxPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mailbox and add message
	m1 := NewMailbox(tmpDir)
	msg := &Message{
		ID:      "persist-001",
		Subject: "Persistent message",
		Body:    "Should survive reload",
	}
	if err := m1.Append(msg); err != nil {
		t.Fatalf("Append error: %v", err)
	}

	// Create new mailbox pointing to same location
	m2 := NewMailbox(tmpDir)
	msgs, err := m2.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("Reloaded mailbox has %d messages, want 1", len(msgs))
	}
	if msgs[0].Subject != "Persistent message" {
		t.Errorf("Subject = %q, want 'Persistent message'", msgs[0].Subject)
	}
}

func TestNewMailboxWithBeadsDir(t *testing.T) {
	m := NewMailboxWithBeadsDir("gastown/Toast", "/work/dir", "/custom/.beads")
	if m.identity != "gastown/Toast" {
		t.Errorf("identity = %q, want 'gastown/Toast'", m.identity)
	}
	if filepath.ToSlash(m.beadsDir) != "/custom/.beads" {
		t.Errorf("beadsDir = %q, want '/custom/.beads'", m.beadsDir)
	}
}

func TestMailboxLegacyMultipleOperations(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	// Append multiple messages
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:        fmt.Sprintf("msg-%03d", i),
			Subject:   fmt.Sprintf("Subject %d", i),
			Body:      fmt.Sprintf("Body %d", i),
			Read:      i%2 == 0, // Alternate read/unread
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
		}
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	// Delete middle message
	if err := m.Delete("msg-002"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	// Mark one as read
	if err := m.MarkRead("msg-001"); err != nil {
		t.Fatalf("MarkRead error: %v", err)
	}

	// Verify counts
	total, unread, err := m.Count()
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	// After marking msg-001 as read, we have: msg-000 (read), msg-001 (read), msg-003 (unread), msg-004 (read)
	// So unread = 1
	if unread != 1 {
		t.Errorf("unread = %d, want 1", unread)
	}
}

func TestMailboxLegacyAppendWithMissingDir(t *testing.T) {
	tmpDir := t.TempDir()
	deepPath := filepath.Join(tmpDir, "deep", "nested", "inbox")
	m := NewMailbox(deepPath)

	msg := &Message{
		ID:      "msg-001",
		Subject: "Test",
	}

	// Should create directories
	if err := m.Append(msg); err != nil {
		t.Fatalf("Append error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		t.Fatal("inbox.jsonl was not created")
	}
}

func TestMailboxLegacyDeleteAll(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	// Add messages
	msgs := []*Message{
		{ID: "msg-001"},
		{ID: "msg-002"},
	}
	for _, msg := range msgs {
		if err := m.Append(msg); err != nil {
			t.Fatalf("Append error: %v", err)
		}
	}

	// Delete all
	for _, msg := range msgs {
		if err := m.Delete(msg.ID); err != nil {
			t.Fatalf("Delete error: %v", err)
		}
	}

	// Should be empty
	total, _, err := m.Count()
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

func TestMailboxLegacyMarkReadTwice(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewMailbox(tmpDir)

	msg := &Message{ID: "msg-001", Read: false}
	if err := m.Append(msg); err != nil {
		t.Fatalf("Append error: %v", err)
	}

	// Mark as read twice
	if err := m.MarkRead("msg-001"); err != nil {
		t.Fatalf("First MarkRead error: %v", err)
	}
	if err := m.MarkRead("msg-001"); err != nil {
		t.Fatalf("Second MarkRead error: %v", err)
	}

	// Should still be read
	got, err := m.Get("msg-001")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !got.Read {
		t.Error("Message should be marked as read")
	}
}

