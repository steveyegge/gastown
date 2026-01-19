package mail

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/runtime"
)

// timeNow is a function that returns the current time. It can be overridden in tests.
var timeNow = time.Now

// Common errors
var (
	ErrMessageNotFound = errors.New("message not found")
	ErrEmptyInbox      = errors.New("inbox is empty")
)

// Mailbox manages messages for an identity via beads.
type Mailbox struct {
	identity string // beads identity (e.g., "gastown/polecats/Toast")
	workDir  string // directory to run bd commands in
	beadsDir string // explicit .beads directory path (set via BEADS_DIR)
	path     string // for legacy JSONL mode (crew workers)
	legacy   bool   // true = use JSONL files, false = use beads
}

// NewMailbox creates a mailbox for the given JSONL path (legacy mode).
// Used by crew workers that have local JSONL inboxes.
func NewMailbox(path string) *Mailbox {
	return &Mailbox{
		path:   filepath.Join(path, "inbox.jsonl"),
		legacy: true,
	}
}

// NewMailboxBeads creates a mailbox backed by beads.
func NewMailboxBeads(identity, workDir string) *Mailbox {
	return &Mailbox{
		identity: identity,
		workDir:  workDir,
		legacy:   false,
	}
}

// NewMailboxFromAddress creates a beads-backed mailbox from a GGT address.
// Follows .beads/redirect for crew workers and polecats using shared beads.
func NewMailboxFromAddress(address, workDir string) *Mailbox {
	beadsDir := beads.ResolveBeadsDir(workDir)
	return &Mailbox{
		identity: AddressToIdentity(address),
		workDir:  workDir,
		beadsDir: beadsDir,
		legacy:   false,
	}
}

// NewMailboxWithBeadsDir creates a mailbox with an explicit beads directory.
func NewMailboxWithBeadsDir(address, workDir, beadsDir string) *Mailbox {
	return &Mailbox{
		identity: AddressToIdentity(address),
		workDir:  workDir,
		beadsDir: beadsDir,
		legacy:   false,
	}
}

// Identity returns the beads identity for this mailbox.
func (m *Mailbox) Identity() string {
	return m.identity
}

// Path returns the JSONL path for legacy mailboxes.
func (m *Mailbox) Path() string {
	return m.path
}

// List returns all open messages in the mailbox.
func (m *Mailbox) List() ([]*Message, error) {
	if m.legacy {
		return m.listLegacy()
	}
	return m.listBeads()
}

func (m *Mailbox) listBeads() ([]*Message, error) {
	// Single query to beads - returns both persistent and wisp messages
	// Wisps are stored in same DB with wisp=true flag, filtered from JSONL export
	messages, err := m.listFromDir(m.beadsDir)
	if err != nil {
		return nil, err
	}

	// Sort by timestamp (newest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.After(messages[j].Timestamp)
	})

	return messages, nil
}

// listFromDir queries messages from a beads directory.
// Returns messages where identity is the assignee OR a CC recipient.
// Includes both open and hooked messages (hooked = auto-assigned handoff mail).
// If all queries fail, returns the last error encountered.
func (m *Mailbox) listFromDir(beadsDir string) ([]*Message, error) {
	seen := make(map[string]bool)
	var messages []*Message
	var lastErr error
	anySucceeded := false

	// Get all identity variants to query (handles legacy vs normalized formats)
	identities := m.identityVariants()

	// Query for each identity variant in both open and hooked statuses
	for _, identity := range identities {
		for _, status := range []string{"open", "hooked"} {
			msgs, err := m.queryMessages(beadsDir, "--assignee", identity, status)
			if err != nil {
				lastErr = err
			} else {
				anySucceeded = true
				for _, msg := range msgs {
					if !seen[msg.ID] {
						seen[msg.ID] = true
						messages = append(messages, msg)
					}
				}
			}
		}
	}

	// Query for CC'd messages (open only)
	for _, identity := range identities {
		ccMsgs, err := m.queryMessages(beadsDir, "--label", "cc:"+identity, "open")
		if err != nil {
			lastErr = err
		} else {
			anySucceeded = true
			for _, msg := range ccMsgs {
				if !seen[msg.ID] {
					seen[msg.ID] = true
					messages = append(messages, msg)
				}
			}
		}
	}

	// If ALL queries failed, return the last error
	if !anySucceeded && lastErr != nil {
		return nil, fmt.Errorf("all mailbox queries failed: %w", lastErr)
	}

	return messages, nil
}

// identityVariants returns all identity formats to query.
// For town-level agents (mayor/, deacon/), also includes the variant without
// trailing slash for backwards compatibility with legacy messages.
func (m *Mailbox) identityVariants() []string {
	variants := []string{m.identity}

	// Town-level agents may have legacy messages without trailing slash
	if m.identity == "mayor/" {
		variants = append(variants, "mayor")
	} else if m.identity == "deacon/" {
		variants = append(variants, "deacon")
	}

	return variants
}

// queryMessages runs a bd list query with the given filter flag and value.
func (m *Mailbox) queryMessages(beadsDir, filterFlag, filterValue, status string) ([]*Message, error) {
	args := []string{"list",
		"--type", "message",
		filterFlag, filterValue,
		"--status", status,
		"--json",
	}

	stdout, err := runBdCommand(args, m.workDir, beadsDir)
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var beadsMsgs []BeadsMessage
	if err := json.Unmarshal(stdout, &beadsMsgs); err != nil {
		// Empty inbox returns empty array or nothing
		if len(stdout) == 0 || string(stdout) == "null" {
			return nil, nil
		}
		return nil, err
	}

	// Convert to GGT messages - wisp status comes from beads issue.wisp field
	var messages []*Message
	for _, bm := range beadsMsgs {
		messages = append(messages, bm.ToMessage())
	}

	return messages, nil
}

func (m *Mailbox) listLegacy() ([]*Message, error) {
	file, err := os.Open(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }() // non-fatal: OS will close on exit

	var messages []*Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}
		messages = append(messages, &msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Sort by timestamp (newest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.After(messages[j].Timestamp)
	})

	return messages, nil
}

// ListUnread returns unread (open) messages.
func (m *Mailbox) ListUnread() ([]*Message, error) {
	all, err := m.List()
	if err != nil {
		return nil, err
	}
	var unread []*Message
	for _, msg := range all {
		if !msg.Read {
			unread = append(unread, msg)
		}
	}
	return unread, nil
}

// Get returns a message by ID.
func (m *Mailbox) Get(id string) (*Message, error) {
	if m.legacy {
		return m.getLegacy(id)
	}
	return m.getBeads(id)
}

func (m *Mailbox) getBeads(id string) (*Message, error) {
	// Single DB query - wisps and persistent messages in same store
	return m.getFromDir(id, m.beadsDir)
}

// getFromDir retrieves a message from a beads directory.
func (m *Mailbox) getFromDir(id, beadsDir string) (*Message, error) {
	args := []string{"show", id, "--json"}

	stdout, err := runBdCommand(args, m.workDir, beadsDir)
	if err != nil {
		if bdErr, ok := err.(*bdError); ok && bdErr.ContainsError("not found") {
			return nil, ErrMessageNotFound
		}
		return nil, err
	}

	// bd show --json returns an array
	var bms []BeadsMessage
	if err := json.Unmarshal(stdout, &bms); err != nil {
		return nil, err
	}
	if len(bms) == 0 {
		return nil, ErrMessageNotFound
	}

	// Wisp status comes from beads issue.wisp field via ToMessage()
	return bms[0].ToMessage(), nil
}

func (m *Mailbox) getLegacy(id string) (*Message, error) {
	messages, err := m.List()
	if err != nil {
		return nil, err
	}
	for _, msg := range messages {
		if msg.ID == id {
			return msg, nil
		}
	}
	return nil, ErrMessageNotFound
}

// MarkRead marks a message as read.
func (m *Mailbox) MarkRead(id string) error {
	if m.legacy {
		return m.markReadLegacy(id)
	}
	return m.markReadBeads(id)
}

func (m *Mailbox) markReadBeads(id string) error {
	// Single DB - wisps and persistent messages in same store
	return m.closeInDir(id, m.beadsDir)
}

// closeInDir closes a message in a specific beads directory.
func (m *Mailbox) closeInDir(id, beadsDir string) error {
	args := []string{"close", id}
	// Pass session ID for work attribution if available
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
		args = append(args, "--session="+sessionID)
	}

	_, err := runBdCommand(args, m.workDir, beadsDir)
	if err != nil {
		if bdErr, ok := err.(*bdError); ok && bdErr.ContainsError("not found") {
			return ErrMessageNotFound
		}
		return err
	}

	return nil
}

func (m *Mailbox) markReadLegacy(id string) error {
	messages, err := m.List()
	if err != nil {
		return err
	}

	found := false
	for _, msg := range messages {
		if msg.ID == id {
			msg.Read = true
			found = true
		}
	}

	if !found {
		return ErrMessageNotFound
	}

	return m.rewriteLegacy(messages)
}

// MarkReadOnly marks a message as read WITHOUT archiving/closing it.
// For beads mode, this adds a "read" label to the message.
// For legacy mode, this sets the Read field to true.
// The message remains in the inbox but is displayed as read.
func (m *Mailbox) MarkReadOnly(id string) error {
	if m.legacy {
		return m.markReadLegacy(id)
	}
	return m.markReadOnlyBeads(id)
}

func (m *Mailbox) markReadOnlyBeads(id string) error {
	// Add "read" label to mark as read without closing
	args := []string{"label", "add", id, "read"}

	_, err := runBdCommand(args, m.workDir, m.beadsDir)
	if err != nil {
		if bdErr, ok := err.(*bdError); ok && bdErr.ContainsError("not found") {
			return ErrMessageNotFound
		}
		return err
	}

	return nil
}

// MarkUnreadOnly marks a message as unread (removes "read" label).
// For beads mode, this removes the "read" label from the message.
// For legacy mode, this sets the Read field to false.
func (m *Mailbox) MarkUnreadOnly(id string) error {
	if m.legacy {
		return m.markUnreadLegacy(id)
	}
	return m.markUnreadOnlyBeads(id)
}

func (m *Mailbox) markUnreadOnlyBeads(id string) error {
	// Remove "read" label to mark as unread
	args := []string{"label", "remove", id, "read"}

	_, err := runBdCommand(args, m.workDir, m.beadsDir)
	if err != nil {
		if bdErr, ok := err.(*bdError); ok && bdErr.ContainsError("not found") {
			return ErrMessageNotFound
		}
		// Ignore error if label doesn't exist
		if bdErr, ok := err.(*bdError); ok && bdErr.ContainsError("does not have label") {
			return nil
		}
		return err
	}

	return nil
}

// MarkUnread marks a message as unread (reopens in beads).
func (m *Mailbox) MarkUnread(id string) error {
	if m.legacy {
		return m.markUnreadLegacy(id)
	}
	return m.markUnreadBeads(id)
}

func (m *Mailbox) markUnreadBeads(id string) error {
	args := []string{"reopen", id}

	_, err := runBdCommand(args, m.workDir, m.beadsDir)
	if err != nil {
		if bdErr, ok := err.(*bdError); ok && bdErr.ContainsError("not found") {
			return ErrMessageNotFound
		}
		return err
	}

	return nil
}

func (m *Mailbox) markUnreadLegacy(id string) error {
	messages, err := m.List()
	if err != nil {
		return err
	}

	found := false
	for _, msg := range messages {
		if msg.ID == id {
			msg.Read = false
			found = true
		}
	}

	if !found {
		return ErrMessageNotFound
	}

	return m.rewriteLegacy(messages)
}

// Delete removes a message.
func (m *Mailbox) Delete(id string) error {
	if m.legacy {
		return m.deleteLegacy(id)
	}
	return m.MarkRead(id) // beads: just acknowledge/close
}

func (m *Mailbox) deleteLegacy(id string) error {
	messages, err := m.List()
	if err != nil {
		return err
	}

	var filtered []*Message
	found := false
	for _, msg := range messages {
		if msg.ID == id {
			found = true
		} else {
			filtered = append(filtered, msg)
		}
	}

	if !found {
		return ErrMessageNotFound
	}

	return m.rewriteLegacy(filtered)
}

// Archive moves a message to the archive file and removes it from inbox.
func (m *Mailbox) Archive(id string) error {
	// Get the message first
	msg, err := m.Get(id)
	if err != nil {
		return err
	}

	// Append to archive file
	if err := m.appendToArchive(msg); err != nil {
		return err
	}

	// Delete from inbox
	return m.Delete(id)
}

// ArchivePath returns the path to the archive file.
func (m *Mailbox) ArchivePath() string {
	if m.legacy {
		return m.path + ".archive"
	}
	// For beads, use archive.jsonl in the same directory as beads
	return filepath.Join(m.beadsDir, "archive.jsonl")
}

func (m *Mailbox) appendToArchive(msg *Message) error {
	archivePath := m.ArchivePath()

	// Ensure directory exists
	dir := filepath.Dir(archivePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Open for append
	file, err := os.OpenFile(archivePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G302: archive is non-sensitive operational data
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = file.WriteString(string(data) + "\n")
	return err
}

// ListArchived returns all messages in the archive file.
func (m *Mailbox) ListArchived() ([]*Message, error) {
	archivePath := m.ArchivePath()

	file, err := os.Open(archivePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var messages []*Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines
		}
		messages = append(messages, &msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

// PurgeArchive removes messages from the archive, optionally filtering by age.
// If olderThanDays is 0, removes all archived messages.
func (m *Mailbox) PurgeArchive(olderThanDays int) (int, error) {
	messages, err := m.ListArchived()
	if err != nil {
		return 0, err
	}

	if len(messages) == 0 {
		return 0, nil
	}

	// If no age filter, remove all
	if olderThanDays <= 0 {
		if err := os.Remove(m.ArchivePath()); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		return len(messages), nil
	}

	// Filter by age
	cutoff := timeNow().AddDate(0, 0, -olderThanDays)
	var keep []*Message
	purged := 0

	for _, msg := range messages {
		if msg.Timestamp.Before(cutoff) {
			purged++
		} else {
			keep = append(keep, msg)
		}
	}

	// Rewrite archive with remaining messages
	if len(keep) == 0 {
		if err := os.Remove(m.ArchivePath()); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
	} else {
		if err := m.rewriteArchive(keep); err != nil {
			return 0, err
		}
	}

	return purged, nil
}

func (m *Mailbox) rewriteArchive(messages []*Message) error {
	archivePath := m.ArchivePath()
	tmpPath := archivePath + ".tmp"

	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			_ = file.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		_, _ = file.WriteString(string(data) + "\n")
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, archivePath)
}

// SearchOptions specifies search parameters.
type SearchOptions struct {
	Query       string // Regex pattern to search for
	FromFilter  string // Optional: only match messages from this sender
	SubjectOnly bool   // Only search subject
	BodyOnly    bool   // Only search body
}

// Search finds messages matching the given criteria.
// Returns messages from both inbox and archive.
// Query and FromFilter are treated as literal strings (not regex) to prevent ReDoS.
func (m *Mailbox) Search(opts SearchOptions) ([]*Message, error) {
	// Use QuoteMeta to escape special regex chars - prevents ReDoS attacks
	// and provides intuitive literal string matching for users
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(opts.Query))
	if err != nil {
		return nil, fmt.Errorf("invalid search pattern: %w", err)
	}

	var fromRe *regexp.Regexp
	if opts.FromFilter != "" {
		fromRe, err = regexp.Compile("(?i)" + regexp.QuoteMeta(opts.FromFilter))
		if err != nil {
			return nil, fmt.Errorf("invalid from pattern: %w", err)
		}
	}

	// Get inbox messages
	inbox, err := m.List()
	if err != nil {
		return nil, err
	}

	// Get archived messages
	archived, err := m.ListArchived()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Combine and search
	all := append(inbox, archived...)
	var matches []*Message

	for _, msg := range all {
		// Apply from filter
		if fromRe != nil && !fromRe.MatchString(msg.From) {
			continue
		}

		// Search in specified fields
		matched := false
		if opts.SubjectOnly {
			matched = re.MatchString(msg.Subject)
		} else if opts.BodyOnly {
			matched = re.MatchString(msg.Body)
		} else {
			// Search in both subject and body
			matched = re.MatchString(msg.Subject) || re.MatchString(msg.Body)
		}

		if matched {
			matches = append(matches, msg)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Timestamp.After(matches[j].Timestamp)
	})

	return matches, nil
}

// Count returns the total and unread message counts.
func (m *Mailbox) Count() (total, unread int, err error) {
	messages, err := m.List()
	if err != nil {
		return 0, 0, err
	}

	total = len(messages)
	// Count messages that are NOT marked as read (including via "read" label)
	for _, msg := range messages {
		if !msg.Read {
			unread++
		}
	}

	return total, unread, nil
}

// Append adds a message to the mailbox (legacy mode only).
// For beads mode, use Router.Send() instead.
func (m *Mailbox) Append(msg *Message) error {
	if !m.legacy {
		return errors.New("use Router.Send() to send messages via beads")
	}
	return m.appendLegacy(msg)
}

func (m *Mailbox) appendLegacy(msg *Message) error {
	// Ensure directory exists
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Open for append
	file, err := os.OpenFile(m.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }() // non-fatal: OS will close on exit

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = file.WriteString(string(data) + "\n")
	return err
}

// rewriteLegacy rewrites the mailbox with the given messages.
func (m *Mailbox) rewriteLegacy(messages []*Message) error {
	// Sort by timestamp (oldest first for JSONL)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	// Write to temp file
	tmpPath := m.path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			_ = file.Close()         // best-effort cleanup
			_ = os.Remove(tmpPath)   // best-effort cleanup
			return err
		}
		_, _ = file.WriteString(string(data) + "\n") // non-fatal: partial write is acceptable
	}

	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup
		return err
	}

	// Atomic rename
	return os.Rename(tmpPath, m.path)
}

// ListByThread returns all messages in a given thread.
func (m *Mailbox) ListByThread(threadID string) ([]*Message, error) {
	if m.legacy {
		return m.listByThreadLegacy(threadID)
	}
	return m.listByThreadBeads(threadID)
}

func (m *Mailbox) listByThreadBeads(threadID string) ([]*Message, error) {
	args := []string{"message", "thread", threadID, "--json"}

	stdout, err := runBdCommand(args, m.workDir, m.beadsDir, "BD_IDENTITY="+m.identity)
	if err != nil {
		return nil, err
	}

	var beadsMsgs []BeadsMessage
	if err := json.Unmarshal(stdout, &beadsMsgs); err != nil {
		if len(stdout) == 0 || string(stdout) == "null" {
			return nil, nil
		}
		return nil, err
	}

	var messages []*Message
	for _, bm := range beadsMsgs {
		messages = append(messages, bm.ToMessage())
	}

	// Sort by timestamp (oldest first for thread view)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	return messages, nil
}

func (m *Mailbox) listByThreadLegacy(threadID string) ([]*Message, error) {
	messages, err := m.List()
	if err != nil {
		return nil, err
	}

	var thread []*Message
	for _, msg := range messages {
		if msg.ThreadID == threadID {
			thread = append(thread, msg)
		}
	}

	// Sort by timestamp (oldest first for thread view)
	sort.Slice(thread, func(i, j int) bool {
		return thread[i].Timestamp.Before(thread[j].Timestamp)
	})

	return thread, nil
}
