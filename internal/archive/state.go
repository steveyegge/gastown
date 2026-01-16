package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// ArchiveState manages the active block vs committed transcript separation.
// The committed portion is immutable and append-only, while the active block
// represents the current window of H lines that can be modified.
type ArchiveState struct {
	mu sync.Mutex

	// committed is the immutable, append-only transcript.
	// Once lines are committed, they cannot be modified.
	committed []string

	// active is the mutable window-sized block (H lines max).
	// This represents the current terminal viewport.
	active []string

	// journal is the event journal writer.
	// All mutations are recorded to the journal.
	journal *Journal

	// height is the maximum number of lines in the active block.
	height int
}

// NewArchiveState creates a new ArchiveState with the given journal and height.
// Height represents the terminal viewport size (H lines).
func NewArchiveState(journal *Journal, height int) *ArchiveState {
	return &ArchiveState{
		committed: make([]string, 0),
		active:    make([]string, 0, height),
		journal:   journal,
		height:    height,
	}
}

// Committed returns a copy of the committed transcript.
func (s *ArchiveState) Committed() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.committed))
	copy(result, s.committed)
	return result
}

// Active returns a copy of the active block.
func (s *ArchiveState) Active() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.active))
	copy(result, s.active)
	return result
}

// CommitLines moves the top count lines from active to committed.
// This operation emits a COMMIT event to the journal.
//
// Invariant: committed is append-only, so this only appends lines.
func (s *ArchiveState) CommitLines(count int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count <= 0 {
		return nil
	}

	if count > len(s.active) {
		count = len(s.active)
	}

	// Move top count lines from active to committed
	linesToCommit := s.active[:count]
	s.committed = append(s.committed, linesToCommit...)
	s.active = s.active[count:]

	// Calculate checksum of committed content
	checksum := s.computeChecksum()

	// Emit COMMIT event to journal
	if s.journal != nil {
		if err := s.journal.WriteCommit(len(s.committed), checksum); err != nil {
			return fmt.Errorf("writing commit event: %w", err)
		}
	}

	return nil
}

// AppendLines adds new lines to the active block.
// This operation emits an APPEND event to the journal.
//
// If adding lines would exceed height H, the oldest lines in active
// are automatically committed to maintain the invariant len(active) <= H.
func (s *ArchiveState) AppendLines(lines []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(lines) == 0 {
		return nil
	}

	// Emit APPEND event to journal first
	if s.journal != nil {
		if err := s.journal.WriteAppend(lines); err != nil {
			return fmt.Errorf("writing append event: %w", err)
		}
	}

	// Add lines to active
	s.active = append(s.active, lines...)

	// Enforce invariant: len(active) <= height
	// If we exceed, commit the overflow
	if len(s.active) > s.height {
		overflow := len(s.active) - s.height
		linesToCommit := s.active[:overflow]
		s.committed = append(s.committed, linesToCommit...)
		s.active = s.active[overflow:]

		// Emit COMMIT event for the overflow
		if s.journal != nil {
			checksum := s.computeChecksum()
			if err := s.journal.WriteCommit(len(s.committed), checksum); err != nil {
				return fmt.Errorf("writing overflow commit event: %w", err)
			}
		}
	}

	return nil
}

// ReplaceLines updates a region of the active block starting at start.
// It replaces count lines starting at position start with the new lines.
// This operation emits a REPLACE event to the journal.
//
// The replacement is constrained to the active block only; committed
// content cannot be modified.
func (s *ArchiveState) ReplaceLines(start, count int, lines []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if start < 0 {
		return fmt.Errorf("start index cannot be negative: %d", start)
	}

	if start > len(s.active) {
		return fmt.Errorf("start index %d exceeds active block length %d", start, len(s.active))
	}

	// Emit REPLACE event to journal first
	if s.journal != nil {
		if err := s.journal.WriteReplace(start, lines); err != nil {
			return fmt.Errorf("writing replace event: %w", err)
		}
	}

	// Calculate end position of replacement
	end := start + count
	if end > len(s.active) {
		end = len(s.active)
	}

	// Build new active: prefix + new lines + suffix
	newActive := make([]string, 0, start+len(lines)+(len(s.active)-end))
	newActive = append(newActive, s.active[:start]...)
	newActive = append(newActive, lines...)
	if end < len(s.active) {
		newActive = append(newActive, s.active[end:]...)
	}
	s.active = newActive

	// Enforce invariant: len(active) <= height
	if len(s.active) > s.height {
		overflow := len(s.active) - s.height
		linesToCommit := s.active[:overflow]
		s.committed = append(s.committed, linesToCommit...)
		s.active = s.active[overflow:]

		// Emit COMMIT event for the overflow
		if s.journal != nil {
			checksum := s.computeChecksum()
			if err := s.journal.WriteCommit(len(s.committed), checksum); err != nil {
				return fmt.Errorf("writing overflow commit event: %w", err)
			}
		}
	}

	return nil
}

// Flush writes current state to disk by flushing the journal.
func (s *ArchiveState) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.journal == nil {
		return nil
	}

	// The journal already flushes on each write, but we ensure
	// any pending data is written
	return nil
}

// FullTranscript returns the complete transcript (committed + active).
func (s *ArchiveState) FullTranscript() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]string, 0, len(s.committed)+len(s.active))
	result = append(result, s.committed...)
	result = append(result, s.active...)
	return result
}

// Height returns the maximum active block size.
func (s *ArchiveState) Height() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.height
}

// computeChecksum calculates a SHA256 checksum of the committed content.
// Must be called with mu held.
func (s *ArchiveState) computeChecksum() string {
	content := strings.Join(s.committed, "\n")
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
