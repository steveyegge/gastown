package tmux

import (
	"bytes"
	"regexp"
	"time"
)

// Timing constants for the Clear/Inject/Verify/Restore protocol.
const (
	// nudgeClearDelayMs is the time to wait after Ctrl-C for input to clear.
	nudgeClearDelayMs = 50

	// nudgeInjectDelayMs is the time to wait after injecting text before verification.
	nudgeInjectDelayMs = 50

	// nudgeLullDetectMs is the time to detect a typing pause before retry.
	nudgeLullDetectMs = 300

	// nudgeMaxRetries is the maximum retries before giving up.
	nudgeMaxRetries = 2

	// nudgeContextLines is how many lines of context to use for matching.
	nudgeContextLines = 5

	// nudgeTailCaptureLines is how many lines to capture for verification.
	nudgeTailCaptureLines = 30

	// nudgePastePlaceholderLines is how many lines to scan for paste placeholder.
	nudgePastePlaceholderLines = 50
)

// pastedTextPlaceholderRe matches Claude Code's large paste placeholder pattern.
// Example: "[Pasted text #3 +47 lines]"
var pastedTextPlaceholderRe = regexp.MustCompile(`\[Pasted text #\d+ \+\d+ lines\]`)

// preservedInput accumulates user input that needs to be restored.
type preservedInput struct {
	original    []byte // From BEFORE capture (what was there initially)
	extraBefore []byte // Text before nudge in AFTER (typed after Ctrl-C)
	extraAfter  []byte // Text after nudge in AFTER (typed during inject)
}

// combined returns all preserved input concatenated.
func (p *preservedInput) combined() []byte {
	total := len(p.original) + len(p.extraBefore) + len(p.extraAfter)
	if total == 0 {
		return nil
	}
	result := make([]byte, 0, total)
	result = append(result, p.original...)
	result = append(result, p.extraBefore...)
	result = append(result, p.extraAfter...)
	return result
}

// nudgeSessionReliable implements the Clear/Inject/Verify/Restore protocol.
// This preserves any user input that was in the field when the nudge arrives.
//
// Protocol:
// 1. Check if pane is in blocking mode (copy mode, etc.)
// 2. Capture full scrollback BEFORE
// 3. Clear input with Ctrl-C
// 4. Inject nudge text (NO Enter yet)
// 5. Capture tail AFTER
// 6. Verify nudge integrity (detect user typing during injection)
// 7. If clean, send Enter
// 8. Restore any preserved user input
//
// Returns nil on success, error on failure.
func (t *Tmux) nudgeSessionReliable(session, message string) error {
	preserved := &preservedInput{}

	// Pre-check: is pane in blocking mode?
	if t.IsPaneInMode(session) {
		// Can't deliver while in copy mode - but this is transient
		// Return error and let caller retry if needed
		return ErrPaneInMode
	}

	for retry := 0; retry <= nudgeMaxRetries; retry++ {
		// Step 1: Capture BEFORE (full scrollback)
		beforeFull, err := t.capturePaneFull(session)
		if err != nil {
			return err
		}

		// Check for large paste placeholder
		if hasPastedTextPlaceholder(beforeFull, nudgePastePlaceholderLines) {
			// User is in the middle of a large paste operation
			return ErrPastePlaceholder
		}

		// Step 2: Clear input with Ctrl-C
		if err := t.SendKeysRaw(session, "C-c"); err != nil {
			return err
		}
		time.Sleep(nudgeClearDelayMs * time.Millisecond)

		// Step 3: Inject nudge text (NO Enter yet)
		if err := t.SendKeysLiteral(session, message); err != nil {
			t.restoreInput(session, preserved)
			return err
		}
		time.Sleep(nudgeInjectDelayMs * time.Millisecond)

		// Step 4: Capture AFTER (tail only)
		afterTail, err := t.capturePaneTail(session, nudgeTailCaptureLines)
		if err != nil {
			t.restoreInput(session, preserved)
			return err
		}

		// Step 5: Verify nudge integrity
		found, textBefore, textAfter, corrupted := verifyNudgeIntegrity(afterTail, message)

		if !found {
			// Nudge didn't appear - something went wrong
			t.restoreInput(session, preserved)
			return ErrNudgeNotFound
		}

		if corrupted {
			// Nudge was corrupted - clear and retry
			_ = t.SendKeysRaw(session, "C-c")
			time.Sleep(nudgeClearDelayMs * time.Millisecond)
			continue
		}

		// Accumulate any extra text (user was typing)
		preserved.extraBefore = append(preserved.extraBefore, textBefore...)
		preserved.extraAfter = append(preserved.extraAfter, textAfter...)

		// Check if input is clean (only nudge)
		if len(textBefore) == 0 && len(textAfter) == 0 {
			// Clean delivery - send Enter
			// Send Escape first for vim mode compatibility
			_, _ = t.run("send-keys", "-t", session, "Escape")
			time.Sleep(100 * time.Millisecond)

			if err := t.SendKeysRaw(session, "Enter"); err != nil {
				t.restoreInput(session, preserved)
				return err
			}

			// Find and store original input for restoration
			preserved.original = findOriginalInput(beforeFull, afterTail, message)

			// Restore any accumulated input
			t.restoreInput(session, preserved)

			// Wake the pane for detached sessions
			t.WakePaneIfDetached(session)
			return nil
		}

		// Extra text detected - user was typing
		// Wait for typing lull before retry
		if retry < nudgeMaxRetries {
			if !t.waitForTypingLull(session, nudgeLullDetectMs*time.Millisecond) {
				// Continuous typing - clear current attempt and retry
				_ = t.SendKeysRaw(session, "C-c")
				time.Sleep(nudgeClearDelayMs * time.Millisecond)
				continue
			}
			// Clear for retry
			_ = t.SendKeysRaw(session, "C-c")
			time.Sleep(nudgeClearDelayMs * time.Millisecond)
		}
	}

	// Max retries exhausted - restore what we have and return error
	t.restoreInput(session, preserved)
	return ErrMaxRetries
}

// capturePaneFull captures the full scrollback of a pane.
func (t *Tmux) capturePaneFull(session string) ([]byte, error) {
	content, err := t.CapturePaneAll(session)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// capturePaneTail captures the last N lines of a pane.
func (t *Tmux) capturePaneTail(session string, lines int) ([]byte, error) {
	content, err := t.CapturePane(session, lines)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// restoreInput sends preserved input back to the session.
func (t *Tmux) restoreInput(session string, preserved *preservedInput) {
	combined := preserved.combined()
	if len(combined) == 0 {
		return
	}
	// Send the preserved text back (without Enter)
	_ = t.SendKeysLiteral(session, string(combined))
}

// waitForTypingLull waits for a pause in user typing.
// Returns true if a lull was detected, false if typing continues.
func (t *Tmux) waitForTypingLull(session string, duration time.Duration) bool {
	start := time.Now()
	lastCapture, _ := t.capturePaneTail(session, 5)

	for time.Since(start) < duration {
		time.Sleep(50 * time.Millisecond)
		current, err := t.capturePaneTail(session, 5)
		if err != nil {
			continue
		}
		if !bytes.Equal(current, lastCapture) {
			// Content changed - typing continues, reset timer
			lastCapture = current
			start = time.Now()
		}
	}

	// Lull detected - no change for duration
	return true
}

// verifyNudgeIntegrity checks if the nudge arrived intact and detects extra text.
// Returns: found, textBefore, textAfter, corrupted
func verifyNudgeIntegrity(afterTail []byte, nudgeMessage string) (bool, []byte, []byte, bool) {
	lines := tailLines(afterTail, nudgeTailCaptureLines)
	if len(lines) == 0 {
		return false, nil, nil, false
	}

	// Find the line containing our nudge
	nudgeBytes := []byte(nudgeMessage)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		idx := bytes.Index(line, nudgeBytes)
		if idx == -1 {
			continue
		}

		// Found nudge - check for extra text
		textBefore := bytes.TrimSpace(line[:idx])
		textAfter := bytes.TrimSpace(line[idx+len(nudgeBytes):])

		// Check if nudge is intact (not corrupted)
		// A corrupted nudge would have characters inserted into the middle
		// For now, if we found the exact message, it's not corrupted
		corrupted := false

		return true, textBefore, textAfter, corrupted
	}

	return false, nil, nil, false
}

// findOriginalInput locates the original input in the BEFORE capture using context matching.
func findOriginalInput(beforeFull, afterTail []byte, nudgeMessage string) []byte {
	// Get context lines before the nudge in AFTER
	afterLines := splitLines(afterTail)
	nudgeBytes := []byte(nudgeMessage)

	// Find nudge position in AFTER
	nudgeLineIdx := -1
	for i := len(afterLines) - 1; i >= 0; i-- {
		if bytes.Contains(afterLines[i], nudgeBytes) {
			nudgeLineIdx = i
			break
		}
	}

	if nudgeLineIdx < 0 {
		return nil
	}

	// Get context lines before the nudge (up to nudgeContextLines)
	contextStart := nudgeLineIdx - nudgeContextLines
	if contextStart < 0 {
		contextStart = 0
	}
	context := afterLines[contextStart:nudgeLineIdx]
	if len(context) == 0 {
		return nil
	}

	// Search for this context in BEFORE
	beforeLines := splitLines(beforeFull)

	// Search from the end (most likely location)
	for i := len(beforeLines) - 1; i >= len(context); i-- {
		// Check if context matches
		match := true
		for j := 0; j < len(context); j++ {
			if !linesMatch(beforeLines[i-len(context)+j], context[j]) {
				match = false
				break
			}
		}
		if match {
			// The line after context in BEFORE is the original input
			if i < len(beforeLines) {
				return bytes.TrimSpace(beforeLines[i])
			}
			break
		}
	}

	return nil
}

// tailLines extracts the last n lines from data efficiently.
// Works backwards from the end of the buffer.
func tailLines(data []byte, n int) [][]byte {
	if len(data) == 0 || n <= 0 {
		return nil
	}

	// Count lines from the end
	var lines [][]byte
	end := len(data)

	// Skip trailing newline if present
	if end > 0 && data[end-1] == '\n' {
		end--
	}

	for i := end - 1; i >= 0 && len(lines) < n; i-- {
		if data[i] == '\n' {
			lines = append(lines, data[i+1:end])
			end = i
		}
	}

	// Don't forget the first line (no leading newline)
	if end > 0 && len(lines) < n {
		lines = append(lines, data[0:end])
	}

	// Reverse to get correct order
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	return lines
}

// splitLines splits data into lines.
func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	return bytes.Split(data, []byte{'\n'})
}

// linesMatch compares two lines ignoring trailing whitespace.
func linesMatch(a, b []byte) bool {
	return bytes.Equal(bytes.TrimRight(a, " \t\r"), bytes.TrimRight(b, " \t\r"))
}

// hasPastedTextPlaceholder checks if the capture contains a large paste placeholder.
// Scans the last maxLines lines for the placeholder pattern.
func hasPastedTextPlaceholder(data []byte, maxLines int) bool {
	lines := tailLines(data, maxLines)
	for _, line := range lines {
		if pastedTextPlaceholderRe.Match(line) {
			return true
		}
	}
	return false
}
