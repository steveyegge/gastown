package cmd

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/channelevents"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

const (
	// sessionTranscriptMaxBytes is the maximum size of the session_transcript
	// field in the emitted wide event. Truncation preserves the tail (most
	// recent output) rather than the head, so Grafana queries see the last
	// activity rather than the beginning-of-session noise.
	sessionTranscriptMaxBytes = 8 * 1024 // 8 KB

	// sessionTranscriptDefaultLines is the default number of pane lines
	// captured at session end. Configurable via --lines flag.
	sessionTranscriptDefaultLines = 500
)

var sessionRecordingLines int

var tapSessionRecordingCmd = &cobra.Command{
	Use:   "session-recording",
	Short: "Capture session transcript at Stop hook and emit as wide event",
	Long: `Called by the Claude Code Stop hook. Captures the last N lines of the
current tmux pane, compresses the result, and emits it as a wide event on
the "session" channel with the following fields:

  event_type         "session_end"
  session            tmux session name (from GT_SESSION or derived)
  session_transcript last N lines of pane output, truncated at 8 KB
  transcript_lines   number of lines in the transcript

Grafana / VictoriaLogs can query the "session" channel to retrieve
transcripts for failed or interesting sessions.

The command is idempotent and best-effort: if tmux is not available or
the pane cannot be captured, it exits silently.`,
	RunE:         runTapSessionRecording,
	SilenceUsage: true,
}

func init() {
	tapSessionRecordingCmd.Flags().IntVarP(
		&sessionRecordingLines, "lines", "n", sessionTranscriptDefaultLines,
		"Number of pane lines to capture",
	)
	tapCmd.AddCommand(tapSessionRecordingCmd)
}

// runTapSessionRecording is the entrypoint for `gt tap session-recording`.
func runTapSessionRecording(cmd *cobra.Command, args []string) error {
	// Detect the tmux session name (same logic as costs record).
	sessionName := os.Getenv("GT_SESSION")
	if sessionName == "" {
		sessionName = deriveSessionName()
	}
	if sessionName == "" {
		sessionName = detectCurrentTmuxSession()
	}
	if sessionName == "" {
		// Not a Gas Town session — exit silently.
		return nil
	}

	// Capture the pane output.
	t := tmux.NewTmux()
	runner := func(paneID string, lines int) (string, error) {
		return t.CapturePane(paneID, lines)
	}
	transcript := captureSessionTranscriptWith(sessionName, sessionRecordingLines, runner)

	// Find town root for event emission.
	townRoot, _, _ := workspace.FindFromCwdWithFallback()
	if townRoot == "" {
		townRoot = os.Getenv("GT_TOWN_ROOT")
	}
	if townRoot == "" {
		// No workspace — emit via cwd-relative helper (best-effort).
		_, _ = channelevents.Emit("session", "session_end", buildSessionEndPairs(sessionName, transcript))
		return nil
	}

	_, _ = channelevents.EmitToTown(townRoot, "session", "session_end", buildSessionEndPairs(sessionName, transcript))
	return nil
}

// captureSessionTranscriptRunner is a function that runs tmux capture-pane
// for a given pane ID and line count. Using a function type allows tests to
// inject a fake runner without needing a live tmux server.
type captureSessionTranscriptRunner func(paneID string, maxLines int) (string, error)

// captureSessionTranscript returns the last maxLines of pane output for the
// given paneID, truncated to sessionTranscriptMaxBytes (tail-truncated so the
// most recent output is preserved). It uses the real tmux binary.
func captureSessionTranscript(paneID string, maxLines int) string {
	t := tmux.NewTmux()
	runner := func(id string, lines int) (string, error) {
		return t.CapturePane(id, lines)
	}
	return captureSessionTranscriptWith(paneID, maxLines, runner)
}

// captureSessionTranscriptWith is the testable core: it accepts an injected
// runner so unit tests never need a running tmux server.
func captureSessionTranscriptWith(paneID string, maxLines int, runner captureSessionTranscriptRunner) string {
	out, err := runner(paneID, maxLines)
	if err != nil {
		return ""
	}

	// Truncate to sessionTranscriptMaxBytes, preserving the tail.
	if utf8.RuneCountInString(out) > 0 && len(out) > sessionTranscriptMaxBytes {
		// Keep the last 8KB of bytes. Because we are splitting on raw bytes we
		// may cut in the middle of a multi-byte UTF-8 sequence. Trim back to the
		// nearest valid rune boundary to avoid emitting invalid UTF-8.
		raw := []byte(out)
		start := len(raw) - sessionTranscriptMaxBytes
		if start < 0 {
			start = 0
		}
		// Advance start until we are at a valid UTF-8 rune boundary.
		for start < len(raw) && raw[start]&0xC0 == 0x80 {
			start++
		}
		out = string(raw[start:])
	}

	return out
}

// buildSessionEndPayload creates the map[string]interface{} payload used by
// events.Log (and readable in unit tests without needing channelevents).
func buildSessionEndPayload(sessionName, transcript string) map[string]interface{} {
	lineCount := 0
	if transcript != "" {
		lineCount = strings.Count(transcript, "\n") + 1
	}
	return map[string]interface{}{
		"event_type":         "session_end",
		"session":            sessionName,
		"session_transcript": transcript,
		"transcript_lines":   lineCount,
	}
}

// buildSessionEndPairs converts the payload to the key=value slice that
// channelevents.Emit / EmitToTown expect.
func buildSessionEndPairs(sessionName, transcript string) []string {
	lineCount := 0
	if transcript != "" {
		lineCount = strings.Count(transcript, "\n") + 1
	}
	return []string{
		"event_type=session_end",
		fmt.Sprintf("session=%s", sessionName),
		fmt.Sprintf("session_transcript=%s", transcript),
		fmt.Sprintf("transcript_lines=%d", lineCount),
	}
}
