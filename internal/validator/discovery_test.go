package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseValidatorName(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantOK    bool
		wantWhen  string
		wantScope string
		wantName  string
	}{
		{
			name:      "valid create-decision validator",
			filename:  "create-decision-require-schema.sh",
			wantOK:    true,
			wantWhen:  "create",
			wantScope: "decision",
			wantName:  "require-schema",
		},
		{
			name:      "valid stop-decision validator",
			filename:  "stop-decision-check-artifacts.py",
			wantOK:    true,
			wantWhen:  "stop",
			wantScope: "decision",
			wantName:  "check-artifacts",
		},
		{
			name:      "valid any-any validator",
			filename:  "any-any-log-everything",
			wantOK:    true,
			wantWhen:  "any",
			wantScope: "any",
			wantName:  "log-everything",
		},
		{
			name:      "valid resolve validator",
			filename:  "resolve-decision-notify.sh",
			wantOK:    true,
			wantWhen:  "resolve",
			wantScope: "decision",
			wantName:  "notify",
		},
		{
			name:     "invalid - missing parts",
			filename: "validator.sh",
			wantOK:   false,
		},
		{
			name:     "invalid - only two parts",
			filename: "create-decision.sh",
			wantOK:   false,
		},
		{
			name:     "invalid - wrong when keyword",
			filename: "build-decision-test.sh",
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := parseValidatorName(tt.filename)
			if ok != tt.wantOK {
				t.Errorf("parseValidatorName(%q) ok = %v, want %v", tt.filename, ok, tt.wantOK)
				return
			}
			if !ok {
				return
			}
			if v.When != tt.wantWhen {
				t.Errorf("When = %q, want %q", v.When, tt.wantWhen)
			}
			if v.Scope != tt.wantScope {
				t.Errorf("Scope = %q, want %q", v.Scope, tt.wantScope)
			}
			if v.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", v.Name, tt.wantName)
			}
		})
	}
}

func TestDiscoverValidators(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create .beads/validators with a test validator
	beadsDir := filepath.Join(tmpDir, ".beads", "validators")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create executable validator
	validatorPath := filepath.Join(beadsDir, "create-decision-test.sh")
	if err := os.WriteFile(validatorPath, []byte("#!/bin/sh\necho test"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create non-executable file (should be skipped)
	nonExecPath := filepath.Join(beadsDir, "create-decision-noexec.sh")
	if err := os.WriteFile(nonExecPath, []byte("#!/bin/sh\necho test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Discover validators
	validators, err := DiscoverValidators(tmpDir, "create")
	if err != nil {
		t.Fatalf("DiscoverValidators failed: %v", err)
	}

	// Find our test validator (may also find user-config validators)
	var foundTest bool
	var foundNoExec bool
	for _, v := range validators {
		if v.Name == "test" && v.Source == "beads" {
			foundTest = true
			if v.When != "create" {
				t.Errorf("When = %q, want %q", v.When, "create")
			}
			if v.Scope != "decision" {
				t.Errorf("Scope = %q, want %q", v.Scope, "decision")
			}
		}
		if v.Name == "noexec" {
			foundNoExec = true
		}
	}

	if !foundTest {
		t.Error("expected to find test validator from beads dir")
	}
	if foundNoExec {
		t.Error("non-executable validator should not be discovered")
	}
}

func TestDiscoverForScope(t *testing.T) {
	tmpDir := t.TempDir()

	beadsDir := filepath.Join(tmpDir, ".beads", "validators")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create decision-scoped validator
	decisionValidator := filepath.Join(beadsDir, "create-decision-check.sh")
	if err := os.WriteFile(decisionValidator, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create any-scoped validator
	anyValidator := filepath.Join(beadsDir, "create-any-log.sh")
	if err := os.WriteFile(anyValidator, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create task-scoped validator (should not match decision scope)
	taskValidator := filepath.Join(beadsDir, "create-task-verify.sh")
	if err := os.WriteFile(taskValidator, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatal(err)
	}

	validators, err := DiscoverForScope(tmpDir, "create", "decision")
	if err != nil {
		t.Fatalf("DiscoverForScope failed: %v", err)
	}

	// Should get decision-scoped and any-scoped from beads dir, but not task-scoped
	// Note: may also find validators from ~/.config/gt/validators if they match
	var foundCheck, foundLog, foundTask bool
	for _, v := range validators {
		if v.Name == "check" && v.Source == "beads" {
			foundCheck = true
		}
		if v.Name == "log" && v.Source == "beads" {
			foundLog = true
		}
		if v.Name == "verify" && v.Scope == "task" {
			foundTask = true
		}
	}

	if !foundCheck {
		t.Error("expected to find decision-scoped check validator from beads dir")
	}
	if !foundLog {
		t.Error("expected to find any-scoped log validator from beads dir")
	}
	if foundTask {
		t.Error("task-scoped validator should not be included for decision scope")
	}
}

func TestSourcePriority(t *testing.T) {
	if sourcePriority("beads") <= sourcePriority("project") {
		t.Error("beads should have higher priority than project")
	}
	if sourcePriority("project") <= sourcePriority("user") {
		t.Error("project should have higher priority than user")
	}
}
