package cmd

import (
	"testing"
)

// validatePersonaNameArg is the CLI-level validation gate.
// These tests ensure the cmd layer correctly rejects dangerous inputs
// and accepts valid persona names (regression guard for WU2/WU3).

func TestValidatePersonaNameArg_TraversalRejected(t *testing.T) {
	dangerous := []string{
		"../../etc/passwd",
		"../foo",
		"foo/bar",
		`foo\bar`,
	}
	for _, name := range dangerous {
		t.Run(name, func(t *testing.T) {
			if err := validatePersonaNameArg(name); err == nil {
				t.Errorf("validatePersonaNameArg(%q): expected error, got nil", name)
			}
		})
	}
}

func TestValidatePersonaNameArg_ValidNamesAllowed(t *testing.T) {
	valid := []string{
		"rust-expert",   // hyphenated (regression guard)
		"go-dev",        // hyphenated with short suffix
		"senior_dev",    // underscore
		"v2.0",          // dots and digits
		"alice",         // simple lowercase
		"UPPERCASE",     // uppercase letters
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			if err := validatePersonaNameArg(name); err != nil {
				t.Errorf("validatePersonaNameArg(%q): unexpected error: %v", name, err)
			}
		})
	}
}

func TestValidatePersonaNameArg_EmptyRejected(t *testing.T) {
	if err := validatePersonaNameArg(""); err == nil {
		t.Error("validatePersonaNameArg(\"\"): expected error for empty name, got nil")
	}
}
