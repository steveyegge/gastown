package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

// PagerOptions configures pager behavior for command output.
type PagerOptions struct {
	// NoPager disables pager for this command (--no-pager flag)
	NoPager bool
}

// shouldUsePager determines if output should be piped to a pager.
// Returns false if explicitly disabled, env var set, or stdout is not a TTY.
func shouldUsePager(opts PagerOptions) bool {
	if opts.NoPager {
		return false
	}
	if os.Getenv("GT_NO_PAGER") != "" {
		return false
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}
	return true
}

// getPagerCommand returns the pager command to use.
// Checks GT_PAGER, then PAGER, defaults to "less".
func getPagerCommand() string {
	if pager := os.Getenv("GT_PAGER"); pager != "" {
		return pager
	}
	if pager := os.Getenv("PAGER"); pager != "" {
		return pager
	}
	return "less"
}

// getTerminalHeight returns the terminal height in lines.
// Returns 0 if unable to determine (not a TTY).
func getTerminalHeight() int {
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		return 0
	}
	_, height, err := term.GetSize(fd)
	if err != nil {
		return 0
	}
	return height
}

// contentHeight counts the number of lines in content.
// Returns 0 if content is empty.
func contentHeight(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

// ToPager pipes content to a pager if appropriate.
// Prints directly if pager is disabled, stdout is not a TTY, or content fits in terminal.
func ToPager(content string, opts PagerOptions) error {
	if !shouldUsePager(opts) {
		fmt.Print(content)
		return nil
	}

	termHeight := getTerminalHeight()
	lines := contentHeight(content)

	// print directly if content fits in terminal (leave room for prompt)
	if termHeight > 0 && lines <= termHeight-1 {
		fmt.Print(content)
		return nil
	}

	pagerCmd := getPagerCommand()
	parts := strings.Fields(pagerCmd)
	if len(parts) == 0 {
		fmt.Print(content)
		return nil
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// set LESS options if not already configured
	// -R: allow ANSI color codes
	// -F: quit if content fits on one screen
	// -X: don't clear screen on exit
	if os.Getenv("LESS") == "" {
		cmd.Env = append(os.Environ(), "LESS=-RFX")
	}

	return cmd.Run()
}
