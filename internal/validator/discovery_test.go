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

	if len(validators) != 1 {
		t.Errorf("got %d validators, want 1", len(validators))
	}

	if len(validators) > 0 {
		v := validators[0]
		if v.Name != "test" {
			t.Errorf("Name = %q, want %q", v.Name, "test")
		}
		if v.When != "create" {
			t.Errorf("When = %q, want %q", v.When, "create")
		}
		if v.Scope != "decision" {
			t.Errorf("Scope = %q, want %q", v.Scope, "decision")
		}
		if v.Source != "beads" {
			t.Errorf("Source = %q, want %q", v.Source, "beads")
		}
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

	// Should get decision-scoped and any-scoped, but not task-scoped
	if len(validators) != 2 {
		t.Errorf("got %d validators, want 2", len(validators))
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
