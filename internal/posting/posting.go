// Package posting manages session-level posting state for Gas Town agents.
//
// A "posting" is a transient role assumption stored in .runtime/posting.
// It takes priority over persistent config (RigSettings.WorkerPostings)
// and is cleared on crew handoff, polecat completion, or explicit drop.
// Polecat handoffs preserve the posting (it's tied to the bead assignment).
//
// The file contains a single line: the posting name.
// When active, the posting name appears in bracket notation in identity strings:
//
//	gastown/crew/diesel[scout]
package posting

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/util"
)

// FilePosting is the filename within .runtime/ that stores the assumed posting.
const FilePosting = "posting"

// postingPath returns the full path to the posting state file.
func postingPath(workDir string) string {
	return filepath.Join(workDir, constants.DirRuntime, FilePosting)
}

// Read returns the current posting name for the given work directory.
// Returns empty string if no posting is assumed or the file doesn't exist.
func Read(workDir string) string {
	if workDir == "" {
		return ""
	}
	data, err := os.ReadFile(postingPath(workDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Write sets the posting name for the given work directory.
// Creates .runtime/ if it doesn't exist.
func Write(workDir, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return Clear(workDir)
	}

	runtimeDir := filepath.Join(workDir, constants.DirRuntime)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return err
	}

	return util.AtomicWriteFile(postingPath(workDir), []byte(name+"\n"), 0644)
}

// Clear removes the posting state file, dropping the assumed posting.
func Clear(workDir string) error {
	err := os.Remove(postingPath(workDir))
	if os.IsNotExist(err) {
		return nil // already clear
	}
	return err
}

// AppendBracket appends bracket notation to a base identity string if posting is non-empty.
// Example: AppendBracket("gastown/crew/diesel", "scout") => "gastown/crew/diesel[scout]"
func AppendBracket(base, posting string) string {
	if posting == "" {
		return base
	}
	return base + "[" + posting + "]"
}

// WindowNameBudget is the maximum character count for tmux window names.
const WindowNameBudget = 24

// windowPrefix is prepended to every tmux window name.
// Empty: session name already carries the "gt-" prefix, so window names
// like "ace[inspector]" avoid redundant "gt-ace[inspector]". (gt-nj0)
const windowPrefix = ""

// maxPostingChars is the maximum characters a posting gets before truncation.
const maxPostingChars = 10

// ellipsis is the Unicode ellipsis character used for truncation.
const ellipsis = "…"

// truncate shortens s to maxLen characters using a trailing ellipsis if needed.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen < 2 {
		return s
	}
	return string(runes[:maxLen-1]) + ellipsis
}

// FormatWindowName builds a tmux window name with truncation to fit
// within WindowNameBudget (24 chars).
//
// Format: {name}[{posting}]
//
// Algorithm:
//  1. Fixed overhead: "[" (1) + "]" (1) = 2 chars
//  2. Available = 24 - 2 = 22 chars for name + posting
//  3. Posting gets priority: up to 10 chars; if over, truncate to 9 + "…"
//  4. Name gets remainder; if over, truncate to (remainder - 1) + "…"
//  5. No posting: return "{name}" with no brackets or truncation
func FormatWindowName(name, postingName string) string {
	if postingName == "" {
		return windowPrefix + name
	}

	const overhead = 1 + 1 // "[" + "]"
	available := WindowNameBudget - overhead

	p := truncate(postingName, maxPostingChars)
	pLen := len([]rune(p))

	nameMax := available - pLen
	n := truncate(name, nameMax)

	return windowPrefix + n + "[" + p + "]"
}
