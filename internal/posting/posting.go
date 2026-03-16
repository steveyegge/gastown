// Package posting provides helpers for reading and writing agent posting files.
//
// A posting is a temporary role assignment (e.g., a crew member posted as "scout").
// When active, the posting name appears in bracket notation in identity strings:
//
//	gastown/crew/diesel[scout]
//
// Postings are stored as plain text files at <workerDir>/.runtime/posting.
package posting

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// RuntimeDir is the directory within a worker's root that holds runtime state.
	runtimeDir = ".runtime"
	// FileName is the name of the posting file within the runtime directory.
	fileName = "posting"
)

// Read returns the active posting for the given worker directory, or "" if none.
// The posting file is at <workerDir>/.runtime/posting and contains just the
// posting name (e.g., "scout").
func Read(workerDir string) string {
	if workerDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(workerDir, runtimeDir, fileName))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Write sets the active posting for the given worker directory.
// Creates the .runtime directory if needed.
func Write(workerDir, posting string) error {
	dir := filepath.Join(workerDir, runtimeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, fileName), []byte(posting+"\n"), 0644)
}

// Clear removes the posting file for the given worker directory.
func Clear(workerDir string) error {
	return os.Remove(filepath.Join(workerDir, runtimeDir, fileName))
}

// AppendBracket appends bracket notation to a base identity string if posting is non-empty.
// Example: AppendBracket("gastown/crew/diesel", "scout") => "gastown/crew/diesel[scout]"
func AppendBracket(base, posting string) string {
	if posting == "" {
		return base
	}
	return base + "[" + posting + "]"
}
