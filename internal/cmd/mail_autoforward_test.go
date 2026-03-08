package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForwardPattern(t *testing.T) {
	tests := []struct {
		subject     string
		wantMatch   bool
		wantTarget  string
		wantSubject string
	}{
		{"forward to melania:correction on wix_client_web client id", true, "melania", "correction on wix_client_web client id"},
		{"foward to melania:velo corrected headless client ids", true, "melania", "velo corrected headless client ids"},
		{"forward to dallas:why am i not seeing the apps", true, "dallas", "why am i not seeing the apps"},
		{"forward to  PMs:models update", true, "PMs", "models update"},
		{"Forward To Melania: some message", true, "Melania", "some message"},
		{"FORWARD TO PMs: urgent stuff", true, "PMs", "urgent stuff"},
		{"forward to all PMs: announcement", true, "all PMs", "announcement"},
		// Non-matching
		{"Status update from melania", false, "", ""},
		{"RE: forward to melania:test", false, "", ""},
		{"some random subject", false, "", ""},
	}

	for _, tt := range tests {
		matches := forwardPattern.FindStringSubmatch(tt.subject)
		if tt.wantMatch {
			if matches == nil {
				t.Errorf("expected match for %q, got nil", tt.subject)
				continue
			}
			if matches[1] != tt.wantTarget {
				t.Errorf("for %q: target = %q, want %q", tt.subject, matches[1], tt.wantTarget)
			}
			gotSubject := matches[2]
			if gotSubject != tt.wantSubject {
				t.Errorf("for %q: subject = %q, want %q", tt.subject, gotSubject, tt.wantSubject)
			}
		} else {
			if matches != nil {
				t.Errorf("expected no match for %q, got %v", tt.subject, matches)
			}
		}
	}
}

func TestResolveForwardTarget(t *testing.T) {
	// Create a temp workspace with rig/crew structure
	townRoot := t.TempDir()

	// Create rig with crew
	os.MkdirAll(filepath.Join(townRoot, "cfutons", "crew", "melania"), 0755)
	os.MkdirAll(filepath.Join(townRoot, "cfutons_mobile", "crew", "dallas"), 0755)
	os.MkdirAll(filepath.Join(townRoot, "gastown", "crew", "zhora"), 0755)
	// Skip dirs
	os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755)
	os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755)

	t.Run("bare name", func(t *testing.T) {
		addrs, err := resolveForwardTarget("melania", townRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addrs) != 1 || addrs[0] != "cfutons/melania" {
			t.Errorf("got %v, want [cfutons/melania]", addrs)
		}
	})

	t.Run("PMs", func(t *testing.T) {
		addrs, err := resolveForwardTarget("PMs", townRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addrs) != 3 {
			t.Errorf("got %d addresses, want 3: %v", len(addrs), addrs)
		}
	})

	t.Run("all PMs", func(t *testing.T) {
		addrs, err := resolveForwardTarget("all PMs", townRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addrs) != 3 {
			t.Errorf("got %d addresses, want 3: %v", len(addrs), addrs)
		}
	})

	t.Run("direct address", func(t *testing.T) {
		addrs, err := resolveForwardTarget("cfutons/melania", townRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addrs) != 1 || addrs[0] != "cfutons/melania" {
			t.Errorf("got %v, want [cfutons/melania]", addrs)
		}
	})

	t.Run("unknown name", func(t *testing.T) {
		_, err := resolveForwardTarget("unknown_agent", townRoot)
		if err == nil {
			t.Error("expected error for unknown agent")
		}
	})
}

func TestFindAgentByName(t *testing.T) {
	townRoot := t.TempDir()

	// Agent in crew
	os.MkdirAll(filepath.Join(townRoot, "myrig", "crew", "alice"), 0755)
	// Agent in polecats
	os.MkdirAll(filepath.Join(townRoot, "myrig", "polecats", "bob"), 0755)
	// Skip dirs
	os.MkdirAll(filepath.Join(townRoot, ".dolt-data"), 0755)

	t.Run("crew agent", func(t *testing.T) {
		addrs, err := findAgentByName("alice", townRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addrs) != 1 || addrs[0] != "myrig/alice" {
			t.Errorf("got %v, want [myrig/alice]", addrs)
		}
	})

	t.Run("polecat agent", func(t *testing.T) {
		addrs, err := findAgentByName("bob", townRoot)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(addrs) != 1 || addrs[0] != "myrig/bob" {
			t.Errorf("got %v, want [myrig/bob]", addrs)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := findAgentByName("nobody", townRoot)
		if err == nil {
			t.Error("expected error for missing agent")
		}
	})
}
