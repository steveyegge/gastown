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
