package cmd

import (
	"testing"
)

func TestInferRoleFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		gtRole   string
		expected string
	}{
		{"empty", "", ""},
		{"mayor", "mayor", "mayor"},
		{"deacon", "deacon", "deacon"},
		{"boot", "deacon/boot", "boot"},
		{"witness", "gastown/witness", "witness"},
		{"refinery", "beads/refinery", "refinery"},
		{"polecat", "gastown/polecats/toast", "polecat"},
		{"crew", "gastown/crew/quartz", "crew"},
		{"unknown", "something/weird", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GT_ROLE", tt.gtRole)
			got := inferRoleFromEnv()
			if got != tt.expected {
				t.Errorf("inferRoleFromEnv() with GT_ROLE=%q = %q, want %q", tt.gtRole, got, tt.expected)
			}
		})
	}
}

func TestSplitRoleParts(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"mayor", []string{"mayor"}},
		{"gastown/witness", []string{"gastown", "witness"}},
		{"gastown/polecats/toast", []string{"gastown", "polecats", "toast"}},
		{"gastown/crew/quartz", []string{"gastown", "crew", "quartz"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitRoleParts(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("splitRoleParts(%q) = %v, want %v", tt.input, got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitRoleParts(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}
