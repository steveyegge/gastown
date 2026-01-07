//go:build integration

package daemon

import (
	"io"
	"log"
	"os/exec"
	"strings"
	"testing"
)

func runBd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("bd", args...) //nolint:gosec // bd is a trusted internal tool in this repo
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return string(out)
}

func TestGetRoleConfigForIdentity_PrefersTownRoleBead(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed")
	}

	townRoot := t.TempDir()
	runBd(t, townRoot, "init", "--quiet", "--prefix", "hq")

	// Create canonical role bead.
	runBd(t, townRoot, "create",
		"--id", "hq-witness-role",
		"--type", "role",
		"--title", "Witness Role",
		"--description", "start_command: exec echo hq\n",
	)

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
	}

	cfg, parsed, err := d.getRoleConfigForIdentity("myrig-witness")
	if err != nil {
		t.Fatalf("getRoleConfigForIdentity: %v", err)
	}
	if parsed == nil || parsed.RoleType != "witness" {
		t.Fatalf("parsed = %#v, want roleType witness", parsed)
	}
	if cfg == nil || cfg.StartCommand != "exec echo hq" {
		t.Fatalf("cfg.StartCommand = %#v, want %q", cfg, "exec echo hq")
	}
}

func TestGetRoleConfigForIdentity_FallsBackToLegacyRoleBead(t *testing.T) {
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd not installed")
	}

	townRoot := t.TempDir()
	runBd(t, townRoot, "init", "--quiet", "--prefix", "gt")

	// Only legacy role bead exists.
	runBd(t, townRoot, "create",
		"--id", "gt-witness-role",
		"--type", "role",
		"--title", "Witness Role (legacy)",
		"--description", "start_command: exec echo gt\n",
	)

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
	}

	cfg, parsed, err := d.getRoleConfigForIdentity("myrig-witness")
	if err != nil {
		t.Fatalf("getRoleConfigForIdentity: %v", err)
	}
	if parsed == nil || parsed.RoleType != "witness" {
		t.Fatalf("parsed = %#v, want roleType witness", parsed)
	}
	if cfg == nil || cfg.StartCommand != "exec echo gt" {
		t.Fatalf("cfg.StartCommand = %#v, want %q", cfg, "exec echo gt")
	}
}
