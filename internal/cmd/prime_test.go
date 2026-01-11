package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func writeTestRoutes(t *testing.T, townRoot string, routes []beads.Route) {
	t.Helper()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}
}

func TestGetAgentBeadID_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	})

	cases := []struct {
		name string
		ctx  RoleContext
		want string
	}{
		{
			name: "mayor",
			ctx: RoleContext{
				Role:     RoleMayor,
				TownRoot: townRoot,
			},
			want: "hq-mayor",
		},
		{
			name: "deacon",
			ctx: RoleContext{
				Role:     RoleDeacon,
				TownRoot: townRoot,
			},
			want: "hq-deacon",
		},
		{
			name: "witness",
			ctx: RoleContext{
				Role:     RoleWitness,
				Rig:      "beads",
				TownRoot: townRoot,
			},
			want: "bd-beads-witness",
		},
		{
			name: "refinery",
			ctx: RoleContext{
				Role:     RoleRefinery,
				Rig:      "beads",
				TownRoot: townRoot,
			},
			want: "bd-beads-refinery",
		},
		{
			name: "polecat",
			ctx: RoleContext{
				Role:     RolePolecat,
				Rig:      "beads",
				Polecat:  "lex",
				TownRoot: townRoot,
			},
			want: "bd-beads-polecat-lex",
		},
		{
			name: "crew",
			ctx: RoleContext{
				Role:     RoleCrew,
				Rig:      "beads",
				Polecat:  "lex",
				TownRoot: townRoot,
			},
			want: "bd-beads-crew-lex",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getAgentBeadID(tc.ctx)
			if got != tc.want {
				t.Fatalf("getAgentBeadID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrimeFlagCombinations(t *testing.T) {
	// Find the gt binary - we need to test CLI flag validation
	gtBin, err := exec.LookPath("gt")
	if err != nil {
		t.Skip("gt binary not found in PATH")
	}

	cases := []struct {
		name      string
		args      []string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "state_alone_is_valid",
			args:      []string{"prime", "--state"},
			wantError: false, // May fail for other reasons (not in workspace), but not flag validation
		},
		{
			name:      "state_with_hook_errors",
			args:      []string{"prime", "--state", "--hook"},
			wantError: true,
			errorMsg:  "--state cannot be combined with other flags",
		},
		{
			name:      "state_with_dry_run_errors",
			args:      []string{"prime", "--state", "--dry-run"},
			wantError: true,
			errorMsg:  "--state cannot be combined with other flags",
		},
		{
			name:      "state_with_explain_errors",
			args:      []string{"prime", "--state", "--explain"},
			wantError: true,
			errorMsg:  "--state cannot be combined with other flags",
		},
		{
			name:      "dry_run_and_explain_valid",
			args:      []string{"prime", "--dry-run", "--explain"},
			wantError: false, // May fail for other reasons, but not flag validation
		},
		{
			name:      "hook_and_dry_run_valid",
			args:      []string{"prime", "--hook", "--dry-run"},
			wantError: false, // May fail for other reasons, but not flag validation
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(gtBin, tc.args...)
			output, err := cmd.CombinedOutput()

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got success with output: %s", output)
				}
				if tc.errorMsg != "" && !strings.Contains(string(output), tc.errorMsg) {
					t.Fatalf("expected error containing %q, got: %s", tc.errorMsg, output)
				}
			}
			// For non-error cases, we don't fail on other errors (like "not in workspace")
			// because we're only testing flag validation
			if !tc.wantError && tc.errorMsg != "" && strings.Contains(string(output), tc.errorMsg) {
				t.Fatalf("unexpected error message %q in output: %s", tc.errorMsg, output)
			}
		})
	}
}
