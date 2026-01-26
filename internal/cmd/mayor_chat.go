package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	mayorChatTimeout time.Duration
	mayorChatQuiet   bool
)

var mayorChatCmd = &cobra.Command{
	Use:   "chat [message]",
	Short: "Send a message to Mayor and return response synchronously",
	Long: `Send a message to the Mayor session and return the response synchronously.

This command enables direct conversational interaction with the Mayor agent.
The message can be provided as an argument or read from stdin.

The command will:
1. Check if Mayor is running (error if not)
2. Send the message to Mayor's session
3. Wait for the response (with timeout)
4. Return the response to stdout

Examples:
  gt mayor chat "What's the status of the playground rig?"
  echo "List all active polecats" | gt mayor chat
  gt mayor chat --timeout=60s "Analyze the current workload"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMayorChat,
}

func init() {
	mayorCmd.AddCommand(mayorChatCmd)
	mayorChatCmd.Flags().DurationVar(&mayorChatTimeout, "timeout", 30*time.Second, "Timeout for waiting for response")
	mayorChatCmd.Flags().BoolVarP(&mayorChatQuiet, "quiet", "q", false, "Suppress status messages (only output response)")
}

func runMayorChat(cmd *cobra.Command, args []string) error {
	// Get message from args or stdin
	var message string
	if len(args) > 0 {
		message = args[0]
	} else {
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		message = strings.TrimSpace(string(data))
	}

	if message == "" {
		return fmt.Errorf("message required: provide as argument or stdin")
	}

	// Check workspace
	_, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Check if Mayor is running
	mgr, err := getMayorManager()
	if err != nil {
		return err
	}

	running, err := mgr.IsRunning()
	if err != nil {
		return fmt.Errorf("checking Mayor status: %w", err)
	}
	if !running {
		return fmt.Errorf("Mayor session is not running. Start with: gt mayor start")
	}

	sessionName := mayor.SessionName()

	if !mayorChatQuiet {
		fmt.Fprintf(os.Stderr, "%s Sending message to Mayor...\n", style.Dim.Render("→"))
	}

	// Send message and wait for response
	response, err := sendAndCaptureResponse(sessionName, message, mayorChatTimeout)
	if err != nil {
		return fmt.Errorf("communicating with Mayor: %w", err)
	}

	// Output response to stdout (for programmatic consumption)
	fmt.Println(response)

	return nil
}

// sendAndCaptureResponse sends a message to a tmux session and captures the response.
// This is a simplified implementation that:
// 1. Captures current pane state
// 2. Sends the message
// 3. Waits for output to stabilize
// 4. Captures new output and returns it
func sendAndCaptureResponse(sessionName, message string, timeout time.Duration) (string, error) {
	t := tmux.NewTmux()

	// Capture initial state to know where we started
	beforeLines, err := t.CapturePaneLines(sessionName, 10)
	if err != nil {
		return "", fmt.Errorf("capturing initial state: %w", err)
	}
	beforeLen := len(beforeLines)

	// Send the message using the nudge pattern
	if err := t.NudgeSession(sessionName, message); err != nil {
		return "", fmt.Errorf("sending message: %w", err)
	}

	// Wait for response with polling
	// We'll check for output stabilization by looking for when the output stops changing
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond
	stabilityRequired := 2 * time.Second

	var lastContent string
	var lastChangeTime time.Time
	firstCheck := true

	for time.Now().Before(deadline) {
		// Capture current output (get more lines than before to catch the response)
		currentLines, err := t.CapturePaneLines(sessionName, 100)
		if err != nil {
			return "", fmt.Errorf("capturing output: %w", err)
		}

		currentContent := strings.Join(currentLines, "\n")

		// Check if output has changed
		if currentContent != lastContent {
			lastContent = currentContent
			lastChangeTime = time.Now()
			firstCheck = false
		} else if !firstCheck && time.Since(lastChangeTime) >= stabilityRequired {
			// Output has been stable for required duration - extract response
			response := extractResponse(currentLines, beforeLen, message)
			return response, nil
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for response after %v", timeout)
}

// extractResponse attempts to extract the Mayor's response from captured output.
// It looks for content after our message and before the next prompt.
func extractResponse(lines []string, beforeLen int, sentMessage string) string {
	// Find the line where our message appears
	messageStart := -1
	for i, line := range lines {
		if strings.Contains(line, sentMessage) {
			messageStart = i
			break
		}
	}

	if messageStart == -1 {
		// Couldn't find our message, try to return new content after beforeLen
		if len(lines) > beforeLen {
			return cleanResponseLines(lines[beforeLen:])
		}
		return cleanResponseLines(lines)
	}

	// Get everything after the message line
	responseLines := lines[messageStart+1:]
	return cleanResponseLines(responseLines)
}

// cleanResponseLines filters out tmux UI artifacts and returns clean response text.
func cleanResponseLines(lines []string) string {
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines at start
		if len(cleaned) == 0 && trimmed == "" {
			continue
		}

		// Skip UI artifacts
		if isUIArtifact(trimmed) {
			continue
		}

		cleaned = append(cleaned, line)
	}

	// Remove trailing empty lines
	for len(cleaned) > 0 && strings.TrimSpace(cleaned[len(cleaned)-1]) == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}

	return strings.Join(cleaned, "\n")
}

// isUIArtifact checks if a line is a tmux/Claude UI artifact that should be filtered.
func isUIArtifact(line string) bool {
	// Separator lines (horizontal rules)
	if strings.HasPrefix(line, "─") && len(strings.Trim(line, "─ ")) == 0 {
		return true
	}

	// Prompt indicators
	if line == "❯" || strings.HasPrefix(line, "❯ ") {
		return true
	}

	// Claude Code UI indicators
	if strings.Contains(line, "bypass permissions") {
		return true
	}
	if strings.HasPrefix(line, "⏵⏵") {
		return true
	}

	return false
}
