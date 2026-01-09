package polecat

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestBuildPolecatBranchName(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		polecatName string
		rigName     string
		wantPrefix  string // We can't test exact match due to timestamp
		wantSuffix  string
	}{
		{
			name:        "default template",
			template:    "",
			polecatName: "Toast",
			rigName:     "gastown",
			wantPrefix:  "polecat/Toast-",
			wantSuffix:  "",
		},
		{
			name:        "explicit default template",
			template:    config.DefaultPolecatBranchTemplate,
			polecatName: "Nux",
			rigName:     "gastown",
			wantPrefix:  "polecat/Nux-",
			wantSuffix:  "",
		},
		{
			name:        "custom template with rig",
			template:    "work/{rig}/{name}-{timestamp}",
			polecatName: "Toast",
			rigName:     "gastown",
			wantPrefix:  "work/gastown/Toast-",
			wantSuffix:  "",
		},
		{
			name:        "template without timestamp",
			template:    "feature/{name}",
			polecatName: "Toast",
			rigName:     "gastown",
			wantPrefix:  "feature/Toast",
			wantSuffix:  "",
		},
		{
			name:        "template with all variables",
			template:    "{rig}/polecats/{name}/{timestamp}",
			polecatName: "Nux",
			rigName:     "beads",
			wantPrefix:  "beads/polecats/Nux/",
			wantSuffix:  "",
		},
		{
			name:        "template with only timestamp",
			template:    "work/{timestamp}",
			polecatName: "Toast",
			rigName:     "gastown",
			wantPrefix:  "work/",
			wantSuffix:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPolecatBranchName(tt.template, tt.polecatName, tt.rigName)

			if tt.wantPrefix != "" && !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("BuildPolecatBranchName() = %q, want prefix %q", got, tt.wantPrefix)
			}

			if tt.wantSuffix != "" && !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("BuildPolecatBranchName() = %q, want suffix %q", got, tt.wantSuffix)
			}

			// Branch name should not be empty
			if got == "" {
				t.Error("BuildPolecatBranchName() returned empty string")
			}
		})
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		wantErr    bool
	}{
		{
			name:       "valid simple branch",
			branchName: "feature/toast",
			wantErr:    false,
		},
		{
			name:       "valid polecat branch",
			branchName: "polecat/Toast-1abc2de",
			wantErr:    false,
		},
		{
			name:       "valid nested branch",
			branchName: "work/gastown/Toast-1abc2de",
			wantErr:    false,
		},
		{
			name:       "empty branch name",
			branchName: "",
			wantErr:    true,
		},
		{
			name:       "branch with tilde",
			branchName: "feature~toast",
			wantErr:    true,
		},
		{
			name:       "branch with caret",
			branchName: "feature^toast",
			wantErr:    true,
		},
		{
			name:       "branch with colon",
			branchName: "feature:toast",
			wantErr:    true,
		},
		{
			name:       "branch with space",
			branchName: "feature toast",
			wantErr:    true,
		},
		{
			name:       "branch with backslash",
			branchName: "feature\\toast",
			wantErr:    true,
		},
		{
			name:       "branch with double dot",
			branchName: "feature..toast",
			wantErr:    true,
		},
		{
			name:       "branch ending with .lock",
			branchName: "feature.lock",
			wantErr:    true,
		},
		{
			name:       "branch starting with slash",
			branchName: "/feature/toast",
			wantErr:    true,
		},
		{
			name:       "branch ending with slash",
			branchName: "feature/toast/",
			wantErr:    true,
		},
		{
			name:       "branch starting with dot",
			branchName: ".feature/toast",
			wantErr:    true,
		},
		{
			name:       "branch ending with dot",
			branchName: "feature/toast.",
			wantErr:    true,
		},
		{
			name:       "branch with consecutive slashes",
			branchName: "feature//toast",
			wantErr:    true,
		},
		{
			name:       "branch with @{",
			branchName: "feature@{toast}",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.branchName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBranchName(%q) error = %v, wantErr %v", tt.branchName, err, tt.wantErr)
			}
		})
	}
}
