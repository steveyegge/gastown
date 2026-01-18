package refinery

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestManager(t *testing.T) (*Manager, *agent.Double, string) {
	t.Helper()

	// Create temp directory structure
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755); err != nil {
		t.Fatalf("mkdir .runtime: %v", err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigPath,
	}

	agents := agent.NewDouble()
	return NewManager(agents, r), agents, rigPath
}

func TestManager_GetMR(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	// Create a test MR in the pending queue
	mr := &MergeRequest{
		ID:      "gt-mr-abc123",
		Branch:  "polecat/Toast/gt-xyz",
		Worker:  "Toast",
		IssueID: "gt-xyz",
		Status:  MROpen,
		Error:   "test failure",
	}

	if err := mgr.RegisterMR(mr); err != nil {
		t.Fatalf("RegisterMR: %v", err)
	}

	t.Run("find existing MR", func(t *testing.T) {
		found, err := mgr.GetMR("gt-mr-abc123")
		if err != nil {
			t.Errorf("GetMR() unexpected error: %v", err)
		}
		if found == nil {
			t.Fatal("GetMR() returned nil")
		}
		if found.ID != mr.ID {
			t.Errorf("GetMR() ID = %s, want %s", found.ID, mr.ID)
		}
	})

	t.Run("MR not found", func(t *testing.T) {
		_, err := mgr.GetMR("nonexistent-mr")
		if err != ErrMRNotFound {
			t.Errorf("GetMR() error = %v, want %v", err, ErrMRNotFound)
		}
	})
}

func TestManager_Retry(t *testing.T) {
	t.Run("retry failed MR clears error", func(t *testing.T) {
		mgr, _, _ := setupTestManager(t)

		// Create a failed MR
		mr := &MergeRequest{
			ID:     "gt-mr-failed",
			Branch: "polecat/Toast/gt-xyz",
			Worker: "Toast",
			Status: MROpen,
			Error:  "merge conflict",
		}

		if err := mgr.RegisterMR(mr); err != nil {
			t.Fatalf("RegisterMR: %v", err)
		}

		// Retry without processing
		err := mgr.Retry("gt-mr-failed", false)
		if err != nil {
			t.Errorf("Retry() unexpected error: %v", err)
		}

		// Verify error was cleared
		found, _ := mgr.GetMR("gt-mr-failed")
		if found.Error != "" {
			t.Errorf("Retry() error not cleared, got %s", found.Error)
		}
	})

	t.Run("retry non-failed MR fails", func(t *testing.T) {
		mgr, _, _ := setupTestManager(t)

		// Create a successful MR (no error)
		mr := &MergeRequest{
			ID:     "gt-mr-success",
			Branch: "polecat/Toast/gt-abc",
			Worker: "Toast",
			Status: MROpen,
			Error:  "", // No error
		}

		if err := mgr.RegisterMR(mr); err != nil {
			t.Fatalf("RegisterMR: %v", err)
		}

		err := mgr.Retry("gt-mr-success", false)
		if err != ErrMRNotFailed {
			t.Errorf("Retry() error = %v, want %v", err, ErrMRNotFailed)
		}
	})

	t.Run("retry nonexistent MR fails", func(t *testing.T) {
		mgr, _, _ := setupTestManager(t)

		err := mgr.Retry("nonexistent", false)
		if err != ErrMRNotFound {
			t.Errorf("Retry() error = %v, want %v", err, ErrMRNotFound)
		}
	})
}

func TestManager_RegisterMR(t *testing.T) {
	mgr, _, rigPath := setupTestManager(t)

	mr := &MergeRequest{
		ID:           "gt-mr-new",
		Branch:       "polecat/Cheedo/gt-123",
		Worker:       "Cheedo",
		IssueID:      "gt-123",
		TargetBranch: "main",
		CreatedAt:    time.Now(),
		Status:       MROpen,
	}

	if err := mgr.RegisterMR(mr); err != nil {
		t.Fatalf("RegisterMR: %v", err)
	}

	// Verify it was saved to disk
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}

	var ref Refinery
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	if ref.PendingMRs == nil {
		t.Fatal("PendingMRs is nil")
	}

	saved, ok := ref.PendingMRs["gt-mr-new"]
	if !ok {
		t.Fatal("MR not found in PendingMRs")
	}

	if saved.Worker != "Cheedo" {
		t.Errorf("saved MR worker = %s, want Cheedo", saved.Worker)
	}
}

// =============================================================================
// Status Tests
// Using agent.Double for testable abstraction
//
// Note: Start/Stop operations are handled by factory.Start()/factory.Agents().Stop()
// The Manager only handles status queries and state persistence.
// =============================================================================

func setupTestManagerForStatus(t *testing.T) (*Manager, *agent.Double, string) {
	t.Helper()
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")

	// Create required directories
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, "refinery", "rig"), 0755))

	// Create minimal Claude settings
	claudeDir := filepath.Join(rigPath, "refinery", "rig", ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{}`), 0644))

	r := &rig.Rig{
		Name: "testrig",
		Path: rigPath,
	}

	agents := agent.NewDouble()
	return NewManager(agents, r), agents, rigPath
}

// --- Status() Tests ---

func TestManager_Status_WhenAgentRunning_ReportsRunning(t *testing.T) {
	mgr, agents, rigPath := setupTestManagerForStatus(t)

	// Simulate running agent
	agentID := mgr.Address()
	agents.CreateAgent(agentID)

	// Write state file that says running
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	state := Refinery{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.Marshal(state)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	status, err := mgr.Status()
	require.NoError(t, err)
	assert.Equal(t, agent.StateRunning, status.State)
}

func TestManager_Status_WhenAgentCrashed_DetectsMismatch(t *testing.T) {
	// Scenario: State says running but agent doesn't exist (crashed).
	// Status() should detect mismatch and report stopped.
	mgr, agents, rigPath := setupTestManagerForStatus(t)

	// Write state that says running (but don't create agent)
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	staleState := Refinery{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.Marshal(staleState)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	// Agent doesn't exist
	agentID := mgr.Address()
	assert.False(t, agents.Exists(agentID), "agent should not exist")

	// Status() detects the mismatch and reports stopped
	status, err := mgr.Status()
	require.NoError(t, err)
	assert.Equal(t, agent.StateStopped, status.State, "should detect crashed agent")
}

func TestManager_Status_WhenStateStopped_ReportsStopped(t *testing.T) {
	mgr, _, rigPath := setupTestManagerForStatus(t)

	// Write state that says stopped
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	state := Refinery{RigName: "testrig", State: agent.StateStopped}
	data, _ := json.Marshal(state)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	status, err := mgr.Status()
	require.NoError(t, err)
	assert.Equal(t, agent.StateStopped, status.State)
}

// --- IsRunning() Tests ---

func TestManager_IsRunning_WhenAgentExists_ReturnsTrue(t *testing.T) {
	mgr, agents, _ := setupTestManagerForStatus(t)

	agentID := mgr.Address()
	agents.CreateAgent(agentID)

	assert.True(t, mgr.IsRunning())
}

func TestManager_IsRunning_WhenAgentNotExists_ReturnsFalse(t *testing.T) {
	mgr, _, _ := setupTestManagerForStatus(t)

	assert.False(t, mgr.IsRunning())
}

// --- SessionName() Tests ---

func TestManager_SessionName_Format(t *testing.T) {
	mgr, _, _ := setupTestManagerForStatus(t)
	assert.Equal(t, "gt-testrig-refinery", mgr.SessionName())
}

// --- Address() Tests ---

func TestManager_Address_ReturnsCorrectAgentID(t *testing.T) {
	mgr, _, _ := setupTestManagerForStatus(t)
	expected := agent.RefineryAddress("testrig")
	assert.Equal(t, expected, mgr.Address())
}

// --- LoadState/SaveState Tests ---

func TestManager_LoadState_ReturnsPersistedState(t *testing.T) {
	mgr, _, rigPath := setupTestManagerForStatus(t)

	// Write a state file
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	state := Refinery{RigName: "testrig", State: agent.StateRunning}
	data, _ := json.MarshalIndent(state, "", "  ")
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	loaded, err := mgr.LoadState()
	require.NoError(t, err)
	assert.Equal(t, "testrig", loaded.RigName)
	assert.Equal(t, agent.StateRunning, loaded.State)
}

func TestManager_SaveState_PersistsState(t *testing.T) {
	mgr, _, rigPath := setupTestManagerForStatus(t)

	state := &Refinery{RigName: "testrig", State: agent.StateRunning}
	err := mgr.SaveState(state)
	require.NoError(t, err)

	// Verify file was written
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)

	var loaded Refinery
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "testrig", loaded.RigName)
	assert.Equal(t, agent.StateRunning, loaded.State)
}

func TestManager_LoadState_WhenNoFile_ReturnsDefaultState(t *testing.T) {
	mgr, _, _ := setupTestManagerForStatus(t)

	// Don't create state file - should return default
	state, err := mgr.LoadState()
	require.NoError(t, err)
	assert.Equal(t, "testrig", state.RigName)
	assert.Equal(t, agent.StateStopped, state.State)
}

func TestManager_StateFile(t *testing.T) {
	mgr, _, rigPath := setupTestManagerForStatus(t)
	expected := filepath.Join(rigPath, ".runtime", "refinery.json")
	assert.Equal(t, expected, mgr.stateManager.StateFile())
}

// =============================================================================
// Error Path Tests
// =============================================================================

func TestManager_Status_WhenLoadStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))

	// Write invalid JSON
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	require.NoError(t, os.WriteFile(stateFile, []byte("invalid json"), 0644))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	_, err := mgr.Status()
	assert.Error(t, err)
}

// =============================================================================
// Utility Function Tests
// =============================================================================

func TestManager_SetOutput(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	var buf bytes.Buffer
	mgr.SetOutput(&buf)

	// Trigger output via Retry with processNow
	mr := &MergeRequest{
		ID:     "gt-mr-test",
		Status: MROpen,
		Error:  "test error",
	}
	_ = mgr.RegisterMR(mr)
	_ = mgr.Retry("gt-mr-test", true) // processNow triggers output

	assert.Contains(t, buf.String(), "deprecated")
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"seconds", 30 * time.Second, "30s ago"},
		{"minutes", 5 * time.Minute, "5m ago"},
		{"hours", 3 * time.Hour, "3h ago"},
		{"days", 48 * time.Hour, "2d ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			then := time.Now().Add(-tt.duration)
			got := formatAge(then)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTime(t *testing.T) {
	t.Run("RFC3339 format", func(t *testing.T) {
		result := parseTime("2024-01-15T10:30:00Z")
		assert.Equal(t, 2024, result.Year())
		assert.Equal(t, time.January, result.Month())
		assert.Equal(t, 15, result.Day())
	})

	t.Run("date-only format", func(t *testing.T) {
		result := parseTime("2024-01-15")
		assert.Equal(t, 2024, result.Year())
		assert.Equal(t, time.January, result.Month())
		assert.Equal(t, 15, result.Day())
	})

	t.Run("invalid format returns zero time", func(t *testing.T) {
		result := parseTime("invalid")
		assert.True(t, result.IsZero())
	})
}

// =============================================================================
// Deprecated Function Tests (still need coverage)
// =============================================================================

func TestManager_ProcessMR_ReturnsDeprecatedError(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	mr := &MergeRequest{ID: "test-mr"}
	result := mgr.ProcessMR(mr)

	assert.NotEmpty(t, result.Error)
	assert.Contains(t, result.Error, "deprecated")
}

func TestManager_GetMergeConfig_ReturnsDefaults(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	config := mgr.getMergeConfig()

	assert.True(t, config.RunTests)
	assert.Equal(t, "go test ./...", config.TestCommand)
	assert.True(t, config.DeleteMergedBranches)
}

func TestManager_GetMergeConfig_WithSettings(t *testing.T) {
	mgr, _, rigPath := setupTestManager(t)

	// Create settings file
	settingsDir := filepath.Join(rigPath, "settings")
	require.NoError(t, os.MkdirAll(settingsDir, 0755))

	settings := `{
		"merge_queue": {
			"test_command": "npm test",
			"run_tests": false,
			"delete_merged_branches": false
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(settingsDir, "config.json"), []byte(settings), 0644))

	config := mgr.getMergeConfig()

	assert.False(t, config.RunTests)
	assert.Equal(t, "npm test", config.TestCommand)
	assert.False(t, config.DeleteMergedBranches)
}

// =============================================================================
// GetMR Tests - Additional Coverage
// =============================================================================

func TestManager_GetMR_ReturnsCurrentMR(t *testing.T) {
	mgr, _, rigPath := setupTestManager(t)

	// Set current MR in state
	currentMR := &MergeRequest{
		ID:     "gt-mr-current",
		Branch: "polecat/Worker/gt-xyz",
		Worker: "Worker",
		Status: MRInProgress,
	}

	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	state := Refinery{
		RigName:   "testrig",
		CurrentMR: currentMR,
	}
	data, _ := json.Marshal(state)
	require.NoError(t, os.WriteFile(stateFile, data, 0644))

	found, err := mgr.GetMR("gt-mr-current")
	require.NoError(t, err)
	assert.Equal(t, "gt-mr-current", found.ID)
	assert.Equal(t, MRInProgress, found.Status)
}

// =============================================================================
// Retry Tests - Additional Coverage
// =============================================================================

func TestManager_Retry_WithProcessNow_PrintsDeprecationNotice(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	// Capture output
	var buf bytes.Buffer
	mgr.SetOutput(&buf)

	// Create a failed MR
	mr := &MergeRequest{
		ID:     "gt-mr-retry",
		Status: MROpen,
		Error:  "previous error",
	}
	require.NoError(t, mgr.RegisterMR(mr))

	// Retry with processNow=true
	err := mgr.Retry("gt-mr-retry", true)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "deprecated")
}

// =============================================================================
// GetMR/Retry Edge Cases
// =============================================================================

func TestManager_GetMR_WhenLoadStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))

	// Write invalid JSON
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	require.NoError(t, os.WriteFile(stateFile, []byte("invalid"), 0644))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	_, err := mgr.GetMR("any-id")
	assert.Error(t, err)
}

func TestManager_Retry_WhenLoadStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))

	// Write invalid JSON
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	require.NoError(t, os.WriteFile(stateFile, []byte("invalid"), 0644))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	err := mgr.Retry("any-id", false)
	assert.Error(t, err)
}

func TestManager_Retry_WhenSaveStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	runtimeDir := filepath.Join(rigPath, ".runtime")
	require.NoError(t, os.MkdirAll(runtimeDir, 0755))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	// Create a failed MR
	mr := &MergeRequest{
		ID:     "gt-mr-retry-fail",
		Status: MROpen,
		Error:  "some error",
	}
	require.NoError(t, mgr.RegisterMR(mr))

	// Make the runtime directory read-only
	require.NoError(t, os.Chmod(runtimeDir, 0555))
	defer os.Chmod(runtimeDir, 0755)

	err := mgr.Retry("gt-mr-retry-fail", false)
	assert.Error(t, err)
}

func TestManager_RegisterMR_WhenLoadStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	require.NoError(t, os.MkdirAll(filepath.Join(rigPath, ".runtime"), 0755))

	// Write invalid JSON
	stateFile := filepath.Join(rigPath, ".runtime", "refinery.json")
	require.NoError(t, os.WriteFile(stateFile, []byte("invalid"), 0644))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	mr := &MergeRequest{ID: "test-mr"}
	err := mgr.RegisterMR(mr)
	assert.Error(t, err)
}

func TestManager_RegisterMR_WhenSaveStateFails_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	rigPath := filepath.Join(tmpDir, "testrig")
	runtimeDir := filepath.Join(rigPath, ".runtime")
	require.NoError(t, os.MkdirAll(runtimeDir, 0755))

	r := &rig.Rig{Name: "testrig", Path: rigPath}
	mgr := NewManager(agent.NewDouble(), r)

	// Initialize state first
	_, _ = mgr.Status()

	// Make the runtime directory read-only
	require.NoError(t, os.Chmod(runtimeDir, 0555))
	defer os.Chmod(runtimeDir, 0755)

	mr := &MergeRequest{ID: "test-mr"}
	err := mgr.RegisterMR(mr)
	assert.Error(t, err)
}

// =============================================================================
// calculateIssueScore and issueToMR Tests
// =============================================================================

func TestManager_calculateIssueScore(t *testing.T) {
	mgr, _, _ := setupTestManager(t)
	now := time.Now()

	t.Run("basic score calculation", func(t *testing.T) {
		issue := &beads.Issue{
			ID:        "gt-mr-score",
			Priority:  2,
			CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		}
		score := mgr.calculateIssueScore(issue, now)
		assert.Greater(t, score, 0.0, "score should be positive")
	})

	t.Run("higher priority gets higher score", func(t *testing.T) {
		lowPriority := &beads.Issue{
			ID:        "gt-low",
			Priority:  4,
			CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		}
		highPriority := &beads.Issue{
			ID:        "gt-high",
			Priority:  1,
			CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		}

		lowScore := mgr.calculateIssueScore(lowPriority, now)
		highScore := mgr.calculateIssueScore(highPriority, now)

		assert.Greater(t, highScore, lowScore, "higher priority should have higher score")
	})

	t.Run("with MR fields in description", func(t *testing.T) {
		issue := &beads.Issue{
			ID:          "gt-mr-fields",
			Priority:    2,
			CreatedAt:   now.Add(-1 * time.Hour).Format(time.RFC3339),
			Description: "Branch: test\nRetry_count: 3",
		}
		score := mgr.calculateIssueScore(issue, now)
		assert.Greater(t, score, 0.0, "score with retry count should be positive")
	})

	t.Run("with convoy created at", func(t *testing.T) {
		convoyTime := now.Add(-24 * time.Hour).Format(time.RFC3339)
		issue := &beads.Issue{
			ID:          "gt-mr-convoy",
			Priority:    2,
			CreatedAt:   now.Add(-1 * time.Hour).Format(time.RFC3339),
			Description: "Branch: test\nConvoy_created_at: " + convoyTime,
		}
		score := mgr.calculateIssueScore(issue, now)
		assert.Greater(t, score, 0.0, "score with convoy should be positive")
	})

	t.Run("invalid created at falls back to now", func(t *testing.T) {
		issue := &beads.Issue{
			ID:        "gt-bad-time",
			Priority:  2,
			CreatedAt: "invalid-time",
		}
		score := mgr.calculateIssueScore(issue, now)
		assert.Greater(t, score, 0.0, "score should be positive with invalid time")
	})
}

func TestManager_issueToMR(t *testing.T) {
	mgr, _, _ := setupTestManager(t)

	t.Run("nil issue returns nil", func(t *testing.T) {
		result := mgr.issueToMR(nil)
		assert.Nil(t, result)
	})

	t.Run("issue without MR fields", func(t *testing.T) {
		issue := &beads.Issue{
			ID:        "gt-123",
			Title:     "Test Issue",
			CreatedAt: "2024-01-15T10:00:00Z",
		}
		result := mgr.issueToMR(issue)
		require.NotNil(t, result)
		assert.Equal(t, "gt-123", result.ID)
		assert.Equal(t, MROpen, result.Status)
	})

	t.Run("issue with MR fields", func(t *testing.T) {
		issue := &beads.Issue{
			ID:    "gt-mr-456",
			Title: "MR for feature",
			Description: `Branch: polecat/Toast/gt-feature
Worker: Toast
Target: main
Source_issue: gt-feature`,
			CreatedAt: "2024-01-15T10:00:00Z",
		}
		result := mgr.issueToMR(issue)
		require.NotNil(t, result)
		assert.Equal(t, "polecat/Toast/gt-feature", result.Branch)
		assert.Equal(t, "Toast", result.Worker)
		assert.Equal(t, "main", result.TargetBranch)
	})

	t.Run("issue with MR fields but no target uses default", func(t *testing.T) {
		issue := &beads.Issue{
			ID:    "gt-mr-789",
			Title: "MR without target",
			Description: `Branch: polecat/Toast/gt-notarget
Worker: Toast`,
			CreatedAt: "2024-01-15T10:00:00Z",
		}
		result := mgr.issueToMR(issue)
		require.NotNil(t, result)
		// Should use rig's default branch (or empty if not set)
		assert.NotEmpty(t, result.TargetBranch)
	})
}
