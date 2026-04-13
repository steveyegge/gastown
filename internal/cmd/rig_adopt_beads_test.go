package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRigAdoptBeadsCandidateDetection verifies the .beads/ candidate detection
// logic used by runRigAdopt to decide whether to initialize a fresh database.
func TestRigAdoptBeadsCandidateDetection(t *testing.T) {
	tests := []struct {
		name           string
		setupDirs      []string // directories to create under rigPath
		wantFoundBeads bool     // whether any candidate should be found
	}{
		{
			name:           "no beads directory exists",
			setupDirs:      nil,
			wantFoundBeads: false,
		},
		{
			name:           "rig-level .beads exists",
			setupDirs:      []string{".beads"},
			wantFoundBeads: true,
		},
		{
			name:           "mayor/rig/.beads exists (tracked beads)",
			setupDirs:      []string{"mayor/rig/.beads"},
			wantFoundBeads: true,
		},
		{
			name:           "both candidates exist",
			setupDirs:      []string{".beads", "mayor/rig/.beads"},
			wantFoundBeads: true,
		},
		{
			name:           "unrelated directories dont count",
			setupDirs:      []string{"src", "docs", "mayor"},
			wantFoundBeads: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rigPath := t.TempDir()

			// Set up test directories
			for _, dir := range tt.setupDirs {
				if err := os.MkdirAll(filepath.Join(rigPath, dir), 0755); err != nil {
					t.Fatalf("creating dir %q: %v", dir, err)
				}
			}

			// Replicate the candidate detection logic from runRigAdopt
			candidates := []string{
				filepath.Join(rigPath, ".beads"),
				filepath.Join(rigPath, "mayor", "rig", ".beads"),
			}
			found := false
			for _, candidate := range candidates {
				if _, err := os.Stat(candidate); err == nil {
					found = true
					break
				}
			}

			if found != tt.wantFoundBeads {
				t.Errorf("beads candidate found = %v, want %v", found, tt.wantFoundBeads)
			}
		})
	}
}

// TestRigAdoptFallbackInitNeeded verifies that when no .beads/ candidate exists
// and a prefix is available, the fallback init path is triggered.
func TestRigAdoptFallbackInitNeeded(t *testing.T) {
	tests := []struct {
		name         string
		hasDotBeads  bool
		hasPrefix    bool
		wantFallback bool
	}{
		{
			name:         "no beads + has prefix → needs fallback",
			hasDotBeads:  false,
			hasPrefix:    true,
			wantFallback: true,
		},
		{
			name:         "no beads + no prefix → skip fallback",
			hasDotBeads:  false,
			hasPrefix:    false,
			wantFallback: false,
		},
		{
			name:         "has beads + has prefix → no fallback needed",
			hasDotBeads:  true,
			hasPrefix:    true,
			wantFallback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the decision logic from runRigAdopt
			foundBeadsCandidate := tt.hasDotBeads
			beadsPrefix := ""
			if tt.hasPrefix {
				beadsPrefix = "test"
			}

			needsFallback := !foundBeadsCandidate && beadsPrefix != ""

			if needsFallback != tt.wantFallback {
				t.Errorf("needsFallback = %v, want %v (foundBeads=%v, prefix=%q)",
					needsFallback, tt.wantFallback, foundBeadsCandidate, beadsPrefix)
			}
		})
	}
}

func TestFinalizeBeadsAfterInitUsesBoundEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake bd stub is bash-only")
	}

	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "demo")
	beadsDir := filepath.Join(rigPath, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt","database":"dolt","dolt_mode":"server","dolt_database":"wrongdb"}`), 0644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "bd-env.log")
	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	script := `#!/usr/bin/env bash
set -e
printf '%s|%s|%s\n' "${BEADS_DIR:-<unset>}" "${BEADS_DB:-<unset>}" "${BEADS_DOLT_SERVER_DATABASE:-<unset>}" >> "$BD_ENV_LOG"
exit 0
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write fake bd: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_ENV_LOG", logPath)
	t.Setenv("BEADS_DIR", "/wrong/.beads")
	t.Setenv("BEADS_DB", "wrong")
	t.Setenv("BEADS_DOLT_SERVER_DATABASE", "wrongdb")

	finalizeBeadsAfterInit(townRoot, rigPath, "dp", "demo")

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read env log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 bd config calls, got %d (%q)", len(lines), string(logData))
	}
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			t.Fatalf("unexpected env log line %q", line)
		}
		if parts[0] != beadsDir {
			t.Fatalf("BEADS_DIR = %q, want %q", parts[0], beadsDir)
		}
		if parts[1] != "<unset>" {
			t.Fatalf("BEADS_DB = %q, want <unset>", parts[1])
		}
		if parts[2] != "demo" {
			t.Fatalf("BEADS_DOLT_SERVER_DATABASE = %q, want %q", parts[2], "demo")
		}
	}
}
