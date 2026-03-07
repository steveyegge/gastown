package daytona

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDiscoverWorkspaces_AllHealthy(t *testing.T) {
	t.Parallel()

	workspaces := []Workspace{
		{ID: "ws1", Name: "gt-abc12345-myrig-onyx", State: "running", Rig: "myrig", Polecat: "onyx"},
		{ID: "ws2", Name: "gt-abc12345-myrig-amber", State: "stopped", Rig: "myrig", Polecat: "amber"},
	}
	beads := []AgentBead{
		{ID: "gtd-myrig-polecat-onyx", Polecat: "onyx", DaytonaWorkspaceName: "gt-abc12345-myrig-onyx"},
		{ID: "gtd-myrig-polecat-amber", Polecat: "amber", DaytonaWorkspaceName: "gt-abc12345-myrig-amber"},
	}

	report := DiscoverWorkspaces("myrig", workspaces, beads)

	if report.Healthy != 2 {
		t.Errorf("Healthy = %d, want 2", report.Healthy)
	}
	if report.OrphanedWorkspaces != 0 {
		t.Errorf("OrphanedWorkspaces = %d, want 0", report.OrphanedWorkspaces)
	}
	if report.OrphanedBeads != 0 {
		t.Errorf("OrphanedBeads = %d, want 0", report.OrphanedBeads)
	}
	if len(report.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(report.Results))
	}
}

func TestDiscoverWorkspaces_OrphanedWorkspace(t *testing.T) {
	t.Parallel()

	workspaces := []Workspace{
		{ID: "ws1", Name: "gt-abc12345-myrig-onyx", State: "running", Rig: "myrig", Polecat: "onyx"},
		{ID: "ws2", Name: "gt-abc12345-myrig-ghost", State: "running", Rig: "myrig", Polecat: "ghost"},
	}
	beads := []AgentBead{
		{ID: "gtd-myrig-polecat-onyx", Polecat: "onyx", DaytonaWorkspaceName: "gt-abc12345-myrig-onyx"},
	}

	report := DiscoverWorkspaces("myrig", workspaces, beads)

	if report.Healthy != 1 {
		t.Errorf("Healthy = %d, want 1", report.Healthy)
	}
	if report.OrphanedWorkspaces != 1 {
		t.Errorf("OrphanedWorkspaces = %d, want 1", report.OrphanedWorkspaces)
	}

	// Find the orphaned workspace result.
	var orphan *DiscoveryResult
	for i := range report.Results {
		if report.Results[i].Action == ActionOrphanedWorkspace {
			orphan = &report.Results[i]
			break
		}
	}
	if orphan == nil {
		t.Fatal("expected orphaned workspace result")
	}
	if orphan.Polecat != "ghost" {
		t.Errorf("orphan.Polecat = %q, want %q", orphan.Polecat, "ghost")
	}
	if orphan.Workspace.Name != "gt-abc12345-myrig-ghost" {
		t.Errorf("orphan.Workspace.Name = %q", orphan.Workspace.Name)
	}
}

func TestDiscoverWorkspaces_OrphanedBead(t *testing.T) {
	t.Parallel()

	workspaces := []Workspace{
		{ID: "ws1", Name: "gt-abc12345-myrig-onyx", State: "running", Rig: "myrig", Polecat: "onyx"},
	}
	beads := []AgentBead{
		{ID: "gtd-myrig-polecat-onyx", Polecat: "onyx", DaytonaWorkspaceName: "gt-abc12345-myrig-onyx"},
		{ID: "gtd-myrig-polecat-vanished", Polecat: "vanished", DaytonaWorkspaceName: "gt-abc12345-myrig-vanished", CertSerial: "abc123"},
	}

	report := DiscoverWorkspaces("myrig", workspaces, beads)

	if report.Healthy != 1 {
		t.Errorf("Healthy = %d, want 1", report.Healthy)
	}
	if report.OrphanedBeads != 1 {
		t.Errorf("OrphanedBeads = %d, want 1", report.OrphanedBeads)
	}

	// Find the orphaned bead result.
	var orphan *DiscoveryResult
	for i := range report.Results {
		if report.Results[i].Action == ActionOrphanedBead {
			orphan = &report.Results[i]
			break
		}
	}
	if orphan == nil {
		t.Fatal("expected orphaned bead result")
	}
	if orphan.Polecat != "vanished" {
		t.Errorf("orphan.Polecat = %q, want %q", orphan.Polecat, "vanished")
	}
	if orphan.BeadID != "gtd-myrig-polecat-vanished" {
		t.Errorf("orphan.BeadID = %q", orphan.BeadID)
	}
	if orphan.CertSerial != "abc123" {
		t.Errorf("orphan.CertSerial = %q, want %q", orphan.CertSerial, "abc123")
	}
}

func TestDiscoverWorkspaces_Empty(t *testing.T) {
	t.Parallel()

	report := DiscoverWorkspaces("myrig", nil, nil)

	if report.Healthy != 0 || report.OrphanedWorkspaces != 0 || report.OrphanedBeads != 0 {
		t.Errorf("expected all zeros, got healthy=%d orphanWs=%d orphanBead=%d",
			report.Healthy, report.OrphanedWorkspaces, report.OrphanedBeads)
	}
}

func TestDiscoverWorkspaces_FiltersToRig(t *testing.T) {
	t.Parallel()

	// Include workspaces from different rigs.
	workspaces := []Workspace{
		{ID: "ws1", Name: "gt-abc12345-myrig-onyx", State: "running", Rig: "myrig", Polecat: "onyx"},
		{ID: "ws2", Name: "gt-abc12345-otherrig-pearl", State: "running", Rig: "otherrig", Polecat: "pearl"},
	}
	beads := []AgentBead{
		{ID: "gtd-myrig-polecat-onyx", Polecat: "onyx", DaytonaWorkspaceName: "gt-abc12345-myrig-onyx"},
	}

	report := DiscoverWorkspaces("myrig", workspaces, beads)

	// otherrig workspace should be ignored (different rig).
	if report.Healthy != 1 {
		t.Errorf("Healthy = %d, want 1", report.Healthy)
	}
	if report.OrphanedWorkspaces != 0 {
		t.Errorf("OrphanedWorkspaces = %d, want 0", report.OrphanedWorkspaces)
	}
}

func TestDiscoverWorkspaces_Mixed(t *testing.T) {
	t.Parallel()

	workspaces := []Workspace{
		{ID: "ws1", Name: "gt-abc12345-rig-alpha", State: "running", Rig: "rig", Polecat: "alpha"},
		{ID: "ws2", Name: "gt-abc12345-rig-beta", State: "stopped", Rig: "rig", Polecat: "beta"},
		{ID: "ws3", Name: "gt-abc12345-rig-orphan", State: "running", Rig: "rig", Polecat: "orphan"},
	}
	beads := []AgentBead{
		{ID: "gtd-rig-polecat-alpha", Polecat: "alpha", DaytonaWorkspaceName: "gt-abc12345-rig-alpha"},
		{ID: "gtd-rig-polecat-beta", Polecat: "beta", DaytonaWorkspaceName: "gt-abc12345-rig-beta"},
		{ID: "gtd-rig-polecat-gone", Polecat: "gone", DaytonaWorkspaceName: "gt-abc12345-rig-gone"},
	}

	report := DiscoverWorkspaces("rig", workspaces, beads)

	if report.Healthy != 2 {
		t.Errorf("Healthy = %d, want 2", report.Healthy)
	}
	if report.OrphanedWorkspaces != 1 {
		t.Errorf("OrphanedWorkspaces = %d, want 1", report.OrphanedWorkspaces)
	}
	if report.OrphanedBeads != 1 {
		t.Errorf("OrphanedBeads = %d, want 1", report.OrphanedBeads)
	}
	if len(report.Results) != 4 {
		t.Errorf("len(Results) = %d, want 4", len(report.Results))
	}
}

func TestReconcile_DryRun(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "running"},
			},
			{
				Action:  ActionOrphanedBead,
				Rig:     "myrig",
				Polecat: "vanished",
				BeadID:  "gtd-myrig-polecat-vanished",
			},
		},
		OrphanedWorkspaces: 1,
		OrphanedBeads:      1,
	}

	resetCalled := false
	beadResetter := func(beadID string) error {
		resetCalled = true
		return nil
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{DryRun: true}, beadResetter, nil, logger)

	// Dry run should not have called any daytona commands.
	if len(mock.calls) != 0 {
		t.Errorf("expected 0 daytona calls in dry-run, got %d", len(mock.calls))
	}
	if resetCalled {
		t.Error("bead resetter should not be called in dry-run")
	}
	if result.WorkspacesStopped != 0 || result.WorkspacesDeleted != 0 || result.BeadsReset != 0 {
		t.Errorf("expected all zeros in dry-run, got stopped=%d deleted=%d reset=%d",
			result.WorkspacesStopped, result.WorkspacesDeleted, result.BeadsReset)
	}
}

func TestReconcile_StopsOrphanedWorkspace(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "running"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, nil, nil, logger)

	if result.WorkspacesStopped != 1 {
		t.Errorf("WorkspacesStopped = %d, want 1", result.WorkspacesStopped)
	}
	if result.WorkspacesDeleted != 0 {
		t.Errorf("WorkspacesDeleted = %d, want 0", result.WorkspacesDeleted)
	}
	if result.WorkspacesArchived != 1 {
		t.Errorf("WorkspacesArchived = %d, want 1", result.WorkspacesArchived)
	}
	// Verify stop and archive were called.
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 calls (stop + archive), got %d", len(mock.calls))
	}
	if mock.calls[0].Args[0] != "stop" {
		t.Errorf("expected stop command, got %v", mock.calls[0].Args)
	}
	if mock.calls[1].Args[0] != "archive" {
		t.Errorf("expected archive command, got %v", mock.calls[1].Args)
	}
}

func TestReconcile_StopsAndDeletesOrphanedWorkspace(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "running"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{AutoDelete: true}, nil, nil, logger)

	if result.WorkspacesStopped != 1 {
		t.Errorf("WorkspacesStopped = %d, want 1", result.WorkspacesStopped)
	}
	if result.WorkspacesDeleted != 1 {
		t.Errorf("WorkspacesDeleted = %d, want 1", result.WorkspacesDeleted)
	}
	if result.WorkspacesArchived != 1 {
		t.Errorf("WorkspacesArchived = %d, want 1", result.WorkspacesArchived)
	}
	// Verify stop, archive, and delete were called.
	if len(mock.calls) != 3 {
		t.Fatalf("expected 3 calls (stop + archive + delete), got %d", len(mock.calls))
	}
	if mock.calls[0].Args[0] != "stop" {
		t.Errorf("expected stop command, got %v", mock.calls[0].Args)
	}
	if mock.calls[1].Args[0] != "archive" {
		t.Errorf("expected archive command, got %v", mock.calls[1].Args)
	}
	if mock.calls[2].Args[0] != "delete" {
		t.Errorf("expected delete command, got %v", mock.calls[2].Args)
	}
}

func TestReconcile_SkipsStopForAlreadyStopped(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "stopped"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, nil, nil, logger)

	// Already stopped — no stop call needed, but archive should be called.
	if result.WorkspacesStopped != 0 {
		t.Errorf("WorkspacesStopped = %d, want 0", result.WorkspacesStopped)
	}
	if result.WorkspacesArchived != 1 {
		t.Errorf("WorkspacesArchived = %d, want 1", result.WorkspacesArchived)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call (archive only) for already-stopped workspace, got %d", len(mock.calls))
	}
	if mock.calls[0].Args[0] != "archive" {
		t.Errorf("expected archive command, got %v", mock.calls[0].Args)
	}
}

func TestReconcile_ResetsOrphanedBead(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:  ActionOrphanedBead,
				Rig:     "myrig",
				Polecat: "vanished",
				BeadID:  "gtd-myrig-polecat-vanished",
			},
		},
		OrphanedBeads: 1,
	}

	var resetID string
	beadResetter := func(beadID string) error {
		resetID = beadID
		return nil
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, beadResetter, nil, logger)

	if result.BeadsReset != 1 {
		t.Errorf("BeadsReset = %d, want 1", result.BeadsReset)
	}
	if resetID != "gtd-myrig-polecat-vanished" {
		t.Errorf("resetID = %q, want %q", resetID, "gtd-myrig-polecat-vanished")
	}
}

func TestReconcile_RevokesCertBeforeBeadReset(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:     ActionOrphanedBead,
				Rig:        "myrig",
				Polecat:    "vanished",
				BeadID:     "gtd-myrig-polecat-vanished",
				CertSerial: "deadbeef42",
			},
		},
		OrphanedBeads: 1,
	}

	// Track call order to verify cert is revoked BEFORE bead is reset.
	var callOrder []string
	beadResetter := func(beadID string) error {
		callOrder = append(callOrder, "reset:"+beadID)
		return nil
	}
	certRevoker := func(ctx context.Context, serial string) error {
		callOrder = append(callOrder, "revoke:"+serial)
		return nil
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, beadResetter, certRevoker, logger)

	if result.CertsRevoked != 1 {
		t.Errorf("CertsRevoked = %d, want 1", result.CertsRevoked)
	}
	if result.BeadsReset != 1 {
		t.Errorf("BeadsReset = %d, want 1", result.BeadsReset)
	}
	if len(callOrder) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "revoke:deadbeef42" {
		t.Errorf("first call should be cert revoke, got %q", callOrder[0])
	}
	if callOrder[1] != "reset:gtd-myrig-polecat-vanished" {
		t.Errorf("second call should be bead reset, got %q", callOrder[1])
	}
}

func TestReconcile_SkipsTransitionalStates(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"creating", "starting", "stopping"} {
		state := state
		t.Run(state, func(t *testing.T) {
			t.Parallel()

			mock := &mockRunner{
				defaultResponse: mockResponse{exitCode: 0},
			}
			client := NewClientWithRunner("gt-abc12345", mock)
			logger := log.New(os.Stderr, "test: ", 0)

			report := &ReconcileReport{
				Rig: "myrig",
				Results: []DiscoveryResult{
					{
						Action:    ActionOrphanedWorkspace,
						Rig:       "myrig",
						Polecat:   "ghost",
						Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: state},
					},
				},
				OrphanedWorkspaces: 1,
			}

			result := Reconcile(context.Background(), client, report, ReconcileOptions{}, nil, nil, logger)

			if result.WorkspacesStopped != 0 {
				t.Errorf("WorkspacesStopped = %d, want 0 for state %q", result.WorkspacesStopped, state)
			}
			if result.WorkspacesSkipped != 1 {
				t.Errorf("WorkspacesSkipped = %d, want 1 for state %q", result.WorkspacesSkipped, state)
			}
			if len(mock.calls) != 0 {
				t.Errorf("expected 0 daytona calls for transitional state %q, got %d", state, len(mock.calls))
			}
		})
	}
}

func TestReconcile_HandlesErrorStateWorkspace(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "error"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, nil, nil, logger)

	// Error state workspaces should still attempt stop.
	if result.WorkspacesStopped != 1 {
		t.Errorf("WorkspacesStopped = %d, want 1", result.WorkspacesStopped)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0].Args[0] != "stop" {
		t.Errorf("expected stop command, got %v", mock.calls[0].Args)
	}
}

func TestReconcile_TransitionalStateSkipsDeleteToo(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "creating"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{AutoDelete: true}, nil, nil, logger)

	// Transitional states should be completely skipped, including delete.
	if result.WorkspacesStopped != 0 {
		t.Errorf("WorkspacesStopped = %d, want 0", result.WorkspacesStopped)
	}
	if result.WorkspacesDeleted != 0 {
		t.Errorf("WorkspacesDeleted = %d, want 0", result.WorkspacesDeleted)
	}
	if result.WorkspacesSkipped != 1 {
		t.Errorf("WorkspacesSkipped = %d, want 1", result.WorkspacesSkipped)
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(mock.calls))
	}
}

func TestReconcile_ErrorStateWithAutoDelete(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "error"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{AutoDelete: true}, nil, nil, logger)

	// Error state: stop attempted + delete attempted.
	if result.WorkspacesStopped != 1 {
		t.Errorf("WorkspacesStopped = %d, want 1", result.WorkspacesStopped)
	}
	if result.WorkspacesDeleted != 1 {
		t.Errorf("WorkspacesDeleted = %d, want 1", result.WorkspacesDeleted)
	}
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 calls (stop+delete), got %d", len(mock.calls))
	}
}

func TestDiscoverWorkspaces_SkipsSpawningBeads(t *testing.T) {
	t.Parallel()

	// Workspace doesn't exist yet (being provisioned), but bead is in spawning state.
	workspaces := []Workspace{
		{ID: "ws1", Name: "gt-abc12345-myrig-onyx", State: "running", Rig: "myrig", Polecat: "onyx"},
	}
	beads := []AgentBead{
		{ID: "gtd-myrig-polecat-onyx", Polecat: "onyx", DaytonaWorkspaceName: "gt-abc12345-myrig-onyx"},
		{ID: "gtd-myrig-polecat-newbie", Polecat: "newbie", DaytonaWorkspaceName: "gt-abc12345-myrig-newbie", AgentState: "spawning"},
	}

	report := DiscoverWorkspaces("myrig", workspaces, beads)

	if report.Healthy != 1 {
		t.Errorf("Healthy = %d, want 1", report.Healthy)
	}
	if report.OrphanedBeads != 0 {
		t.Errorf("OrphanedBeads = %d, want 0 (spawning bead should be skipped)", report.OrphanedBeads)
	}
	if report.SpawningSkipped != 1 {
		t.Errorf("SpawningSkipped = %d, want 1", report.SpawningSkipped)
	}

	// Verify no orphaned bead result was generated.
	for _, r := range report.Results {
		if r.Action == ActionOrphanedBead && r.Polecat == "newbie" {
			t.Error("spawning bead should not appear as orphaned")
		}
	}
}

func TestDiscoverWorkspaces_NonSpawningBeadStillOrphaned(t *testing.T) {
	t.Parallel()

	// Bead with non-spawning state and missing workspace should still be orphaned.
	workspaces := []Workspace{}
	beads := []AgentBead{
		{ID: "gtd-myrig-polecat-dead", Polecat: "dead", DaytonaWorkspaceName: "gt-abc12345-myrig-dead", AgentState: "working"},
	}

	report := DiscoverWorkspaces("myrig", workspaces, beads)

	if report.OrphanedBeads != 1 {
		t.Errorf("OrphanedBeads = %d, want 1 (non-spawning bead should still be orphaned)", report.OrphanedBeads)
	}
	if report.SpawningSkipped != 0 {
		t.Errorf("SpawningSkipped = %d, want 0", report.SpawningSkipped)
	}
}

func TestReconcile_PerOperationTimeoutNotShared(t *testing.T) {
	t.Parallel()

	// Track contexts passed to Stop to verify each gets its own deadline.
	var stopContexts []context.Context
	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
		interceptRun: func(ctx context.Context, name string, args ...string) {
			if len(args) > 0 && args[0] == "stop" {
				stopContexts = append(stopContexts, ctx)
			}
		},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	// Three orphaned workspaces — all should be processed with independent timeouts.
	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{Action: ActionOrphanedWorkspace, Rig: "myrig", Polecat: "a", Workspace: &Workspace{Name: "ws-a", State: "running"}},
			{Action: ActionOrphanedWorkspace, Rig: "myrig", Polecat: "b", Workspace: &Workspace{Name: "ws-b", State: "running"}},
			{Action: ActionOrphanedWorkspace, Rig: "myrig", Polecat: "c", Workspace: &Workspace{Name: "ws-c", State: "running"}},
		},
		OrphanedWorkspaces: 3,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{
		PerOperationTimeout: 5 * time.Second,
	}, nil, nil, logger)

	if result.WorkspacesStopped != 3 {
		t.Errorf("WorkspacesStopped = %d, want 3", result.WorkspacesStopped)
	}

	// Each stop call should have received a context with its own deadline,
	// derived from the parent (Background) with the per-operation timeout.
	if len(stopContexts) != 3 {
		t.Fatalf("expected 3 stop contexts, got %d", len(stopContexts))
	}
	for i, ctx := range stopContexts {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Errorf("stop context %d has no deadline (should have per-operation timeout)", i)
			continue
		}
		// Deadline should be in the future (within ~5s of now).
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > 6*time.Second {
			t.Errorf("stop context %d deadline unexpected: remaining=%v", i, remaining)
		}
	}
}

func TestReconcile_SkipsCertRevocationWhenNoSerial(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:  ActionOrphanedBead,
				Rig:     "myrig",
				Polecat: "vanished",
				BeadID:  "gtd-myrig-polecat-vanished",
				// CertSerial intentionally empty
			},
		},
		OrphanedBeads: 1,
	}

	revokeCalled := false
	certRevoker := func(ctx context.Context, serial string) error {
		revokeCalled = true
		return nil
	}
	beadResetter := func(beadID string) error { return nil }

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, beadResetter, certRevoker, logger)

	if revokeCalled {
		t.Error("certRevoker should not be called when CertSerial is empty")
	}
	if result.CertsRevoked != 0 {
		t.Errorf("CertsRevoked = %d, want 0", result.CertsRevoked)
	}
	if result.BeadsReset != 1 {
		t.Errorf("BeadsReset = %d, want 1", result.BeadsReset)
	}
}

func TestReconcile_NilLoggerDoesNotPanic(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "running"},
			},
			{
				Action:  ActionOrphanedBead,
				Rig:     "myrig",
				Polecat: "vanished",
				BeadID:  "gtd-myrig-polecat-vanished",
			},
			{
				Action:    ActionHealthy,
				Rig:       "myrig",
				Polecat:   "alive",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-alive", State: "running"},
				BeadID:    "gtd-myrig-polecat-alive",
			},
		},
		Healthy:            1,
		OrphanedWorkspaces: 1,
		OrphanedBeads:      1,
	}

	beadResetter := func(beadID string) error { return nil }

	// Passing nil logger should not panic — the function guards against it.
	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, beadResetter, nil, nil)

	if result.WorkspacesStopped != 1 {
		t.Errorf("WorkspacesStopped = %d, want 1", result.WorkspacesStopped)
	}
	if result.BeadsReset != 1 {
		t.Errorf("BeadsReset = %d, want 1", result.BeadsReset)
	}
}

func TestReconcile_BeadResetterError(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:  ActionOrphanedBead,
				Rig:     "myrig",
				Polecat: "vanished",
				BeadID:  "gtd-myrig-polecat-vanished",
			},
		},
		OrphanedBeads: 1,
	}

	beadResetter := func(beadID string) error {
		return fmt.Errorf("dolt commit failed: lock contention")
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, beadResetter, nil, logger)

	if result.BeadsReset != 0 {
		t.Errorf("BeadsReset = %d, want 0 (resetter failed)", result.BeadsReset)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if !strings.Contains(result.Errors[0].Error(), "lock contention") {
		t.Errorf("error = %q, want to contain 'lock contention'", result.Errors[0])
	}
}

func TestReconcile_CertRevokerError(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:     ActionOrphanedBead,
				Rig:        "myrig",
				Polecat:    "vanished",
				BeadID:     "gtd-myrig-polecat-vanished",
				CertSerial: "deadbeef42",
			},
		},
		OrphanedBeads: 1,
	}

	var resetCalled bool
	beadResetter := func(beadID string) error {
		resetCalled = true
		return nil
	}
	certRevoker := func(ctx context.Context, serial string) error {
		return fmt.Errorf("proxy unreachable: connection refused")
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, beadResetter, certRevoker, logger)

	if result.CertsRevoked != 0 {
		t.Errorf("CertsRevoked = %d, want 0 (revoker failed)", result.CertsRevoked)
	}
	// Bead reset should still proceed even when cert revocation fails.
	if !resetCalled {
		t.Error("bead resetter should still be called after cert revocation failure")
	}
	if result.BeadsReset != 1 {
		t.Errorf("BeadsReset = %d, want 1", result.BeadsReset)
	}
	// Should have recorded the cert revocation error.
	var foundCertErr bool
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), "proxy unreachable") {
			foundCertErr = true
		}
	}
	if !foundCertErr {
		t.Errorf("expected cert revocation error in result.Errors, got %v", result.Errors)
	}
}

func TestReconcile_ZombieDetection(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	// Workspace with last activity 2 hours ago.
	staleActivity := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	freshActivity := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionHealthy,
				Rig:       "myrig",
				Polecat:   "zombie",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-zombie", State: "running"},
				BeadID:    "gtd-myrig-polecat-zombie",
				Info:      &WorkspaceInfo{LastActivity: staleActivity},
			},
			{
				Action:    ActionHealthy,
				Rig:       "myrig",
				Polecat:   "alive",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-alive", State: "running"},
				BeadID:    "gtd-myrig-polecat-alive",
				Info:      &WorkspaceInfo{LastActivity: freshActivity},
			},
		},
		Healthy: 2,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{
		ZombieThreshold: 30 * time.Minute,
	}, nil, nil, logger)

	if result.ZombiesDetected != 1 {
		t.Errorf("ZombiesDetected = %d, want 1", result.ZombiesDetected)
	}
}

func TestReconcile_ZombieDetectionDisabledByDefault(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	staleActivity := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionHealthy,
				Rig:       "myrig",
				Polecat:   "zombie",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-zombie", State: "running"},
				BeadID:    "gtd-myrig-polecat-zombie",
				Info:      &WorkspaceInfo{LastActivity: staleActivity},
			},
		},
		Healthy: 1,
	}

	// ZombieThreshold zero (default) — should not detect zombies.
	result := Reconcile(context.Background(), client, report, ReconcileOptions{}, nil, nil, logger)

	if result.ZombiesDetected != 0 {
		t.Errorf("ZombiesDetected = %d, want 0 (disabled by default)", result.ZombiesDetected)
	}
}

func TestReconcile_ZombieDetectionSkipsStoppedWorkspaces(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	staleActivity := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionHealthy,
				Rig:       "myrig",
				Polecat:   "idle",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-idle", State: "stopped"},
				BeadID:    "gtd-myrig-polecat-idle",
				Info:      &WorkspaceInfo{LastActivity: staleActivity},
			},
		},
		Healthy: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{
		ZombieThreshold: 30 * time.Minute,
	}, nil, nil, logger)

	// Stopped workspaces should not be flagged as zombies.
	if result.ZombiesDetected != 0 {
		t.Errorf("ZombiesDetected = %d, want 0 (stopped workspace)", result.ZombiesDetected)
	}
}

func TestReconcile_ZombieDetectionSkipsWithoutInfo(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{exitCode: 0},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionHealthy,
				Rig:       "myrig",
				Polecat:   "noinfo",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-noinfo", State: "running"},
				BeadID:    "gtd-myrig-polecat-noinfo",
				// Info is nil — no last_activity data available.
			},
		},
		Healthy: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{
		ZombieThreshold: 30 * time.Minute,
	}, nil, nil, logger)

	if result.ZombiesDetected != 0 {
		t.Errorf("ZombiesDetected = %d, want 0 (no Info data)", result.ZombiesDetected)
	}
}

func TestReconcile_StopFailureContinues(t *testing.T) {
	t.Parallel()

	mock := &mockRunner{
		defaultResponse: mockResponse{
			stderr:   "Error: timeout",
			exitCode: 1,
		},
	}
	client := NewClientWithRunner("gt-abc12345", mock)
	logger := log.New(os.Stderr, "test: ", 0)

	report := &ReconcileReport{
		Rig: "myrig",
		Results: []DiscoveryResult{
			{
				Action:    ActionOrphanedWorkspace,
				Rig:       "myrig",
				Polecat:   "ghost",
				Workspace: &Workspace{Name: "gt-abc12345-myrig-ghost", State: "running"},
			},
		},
		OrphanedWorkspaces: 1,
	}

	result := Reconcile(context.Background(), client, report, ReconcileOptions{AutoDelete: true}, nil, nil, logger)

	// Stop should have been attempted but failed.
	if result.WorkspacesStopped != 0 {
		t.Errorf("WorkspacesStopped = %d, want 0 (stop failed)", result.WorkspacesStopped)
	}
	// Delete is also attempted (even after stop failure) and should also fail.
	if result.WorkspacesDeleted != 0 {
		t.Errorf("WorkspacesDeleted = %d, want 0 (delete also failed)", result.WorkspacesDeleted)
	}
	// Should have errors for both stop and delete.
	if len(result.Errors) < 2 {
		t.Errorf("expected at least 2 errors (stop + delete), got %d: %v", len(result.Errors), result.Errors)
	}
}
