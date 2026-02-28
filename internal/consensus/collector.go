package consensus

import (
	"strings"
	"time"
)

// collectOne waits for a session to become idle after prompt dispatch,
// captures the pane, and diffs against the pre-send snapshot to extract
// the new response content. Uses provider-aware idle detection.
func collectOne(tmux TmuxClient, session, preSnapshot string, provider ProviderInfo, timeout time.Duration) SessionResult {
	start := time.Now()

	// Wake the pane to ensure the event loop processes our input.
	tmux.WakePane(session)

	// Wait for the session to finish processing using provider-aware detection.
	if err := waitForIdle(tmux, session, provider, timeout); err != nil {
		status := StatusTimeout
		if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "idle") {
			status = StatusError
		}
		return SessionResult{
			Session:  session,
			Status:   status,
			Duration: time.Since(start),
			Error:    err.Error(),
			Provider: provider.Name,
		}
	}

	// Capture the pane after completion.
	post, err := tmux.CapturePaneAll(session)
	if err != nil {
		return SessionResult{
			Session:  session,
			Status:   StatusError,
			Duration: time.Since(start),
			Error:    err.Error(),
			Provider: provider.Name,
		}
	}

	// Diff pre/post to extract new content.
	response := extractNewContent(preSnapshot, post)
	response = stripPrompt(response, provider)

	return SessionResult{
		Session:  session,
		Status:   StatusOK,
		Response: response,
		Duration: time.Since(start),
		Provider: provider.Name,
	}
}

// extractNewContent finds new output by diffing pre/post pane snapshots.
// The post snapshot is the pre snapshot with new content appended (the pane
// scrollback grows). We match pre lines as a prefix of post and return
// everything after.
func extractNewContent(pre, post string) string {
	if pre == "" {
		return strings.TrimSpace(post)
	}

	preLines := strings.Split(pre, "\n")
	postLines := strings.Split(post, "\n")

	// Match pre lines from the start of post. The pane scrollback means
	// post should begin with the same content as pre.
	matchLen := 0
	for i := 0; i < len(preLines) && i < len(postLines); i++ {
		if preLines[i] == postLines[i] {
			matchLen = i + 1
		} else {
			break
		}
	}

	if matchLen == 0 {
		return strings.TrimSpace(post)
	}

	// Everything after the matched prefix is new content.
	newLines := postLines[matchLen:]
	return strings.TrimSpace(strings.Join(newLines, "\n"))
}
