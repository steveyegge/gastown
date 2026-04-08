package cmd

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gastown/internal/doctor"
)

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
