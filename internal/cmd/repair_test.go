package cmd

import (
	"fmt"
	"slices"
	"testing"

	"github.com/steveyegge/gastown/internal/doctor"
)

func checkNames(checks []doctor.Check) []string {
	names := make([]string, 0, len(checks))
	for _, check := range checks {
		names = append(names, check.Name())
	}
	return names
}

type fakeRepairCheck struct {
	name      string
	status    doctor.CheckStatus
	canFix    bool
	fixStatus doctor.CheckStatus
	runs      int
	fixed     bool
}

func (f *fakeRepairCheck) Name() string        { return f.name }
func (f *fakeRepairCheck) Description() string { return f.name }
func (f *fakeRepairCheck) CanFix() bool        { return f.canFix }
func (f *fakeRepairCheck) Fix(ctx *doctor.CheckContext) error {
	f.fixed = true
	return nil
}
func (f *fakeRepairCheck) Run(ctx *doctor.CheckContext) *doctor.CheckResult {
	f.runs++
	status := f.status
	if f.fixed {
		status = f.fixStatus
	}
	return &doctor.CheckResult{Name: f.name, Status: status, Message: fmt.Sprintf("status=%s", status.String())}
}

func TestRunRepairChecks_SucceedsWhenFixConverges(t *testing.T) {
	check := &fakeRepairCheck{
		name:      "fixable",
		status:    doctor.StatusError,
		canFix:    true,
		fixStatus: doctor.StatusOK,
	}

	if err := runRepairChecks(t.TempDir(), "", "", check); err != nil {
		t.Fatalf("runRepairChecks returned error: %v", err)
	}
	if check.runs < 2 {
		t.Fatalf("expected check to rerun after fix, got %d runs", check.runs)
	}
}

func TestRunRepairChecks_FailsWhenBlockingIssueRemains(t *testing.T) {
	check := &fakeRepairCheck{
		name:      "persistent-error",
		status:    doctor.StatusError,
		canFix:    true,
		fixStatus: doctor.StatusError,
	}

	if err := runRepairChecks(t.TempDir(), "", "", check); err == nil {
		t.Fatal("expected runRepairChecks to return blocking error")
	}
}

func TestBootstrapRepairChecks_HQOnlySurface(t *testing.T) {
	names := checkNames(bootstrapRepairChecks())

	for _, required := range []string{"town-config-exists", "town-config-valid", "rigs-registry-exists", "mayor-exists", "town-beads-config", "routes-config", "tmux-global-env", "stale-dolt-port", "database-prefix"} {
		if !slices.Contains(names, required) {
			t.Fatalf("bootstrapRepairChecks missing %q: %v", required, names)
		}
	}
	for _, forbidden := range []string{"rigs-registry-valid", "rig-config-sync", "stale-beads-redirect", "beads-redirect-target", "rig-beads-exist", "agent-beads-exist"} {
		if slices.Contains(names, forbidden) {
			t.Fatalf("bootstrapRepairChecks unexpectedly includes %q: %v", forbidden, names)
		}
	}
}

func TestRigRepairChecks_IncludeRedirectAndAgentInvariantCoverage(t *testing.T) {
	names := checkNames(rigRepairChecks())

	for _, required := range []string{"rig-config-sync", "routes-config", "database-prefix", "stale-beads-redirect", "beads-redirect-target", "rig-beads-exist", "agent-beads-exist", "stale-dolt-port"} {
		if !slices.Contains(names, required) {
			t.Fatalf("rigRepairChecks missing %q: %v", required, names)
		}
	}
}
