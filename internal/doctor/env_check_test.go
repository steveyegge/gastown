package doctor

import (
	"errors"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// mockEnvReader implements SessionEnvReader for testing.
type mockEnvReader struct {
	sessions    []string
	sessionEnvs map[string]map[string]string
	listErr     error
	envErrs     map[string]error
}

func (m *mockEnvReader) ListSessions() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.sessions, nil
}

func (m *mockEnvReader) GetAllEnvironment(session string) (map[string]string, error) {
	if m.envErrs != nil {
		if err, ok := m.envErrs[session]; ok {
			return nil, err
		}
	}
	if m.sessionEnvs != nil {
		if env, ok := m.sessionEnvs[session]; ok {
			return env, nil
		}
	}
	return map[string]string{}, nil
}

func TestEnvVarsCheck_NoSessions(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Message != "No Gas Town sessions running" {
		t.Errorf("Message = %q, want %q", result.Message, "No Gas Town sessions running")
	}
}

func TestEnvVarsCheck_ListSessionsError(t *testing.T) {
	reader := &mockEnvReader{
		listErr: errors.New("tmux not running"),
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	// No tmux server is valid (Gas Town can be down)
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Message != "No tmux sessions running" {
		t.Errorf("Message = %q, want %q", result.Message, "No tmux sessions running")
	}
}

func TestEnvVarsCheck_NonGasTownSessions(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{"other-session", "my-dev"},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Message != "No Gas Town sessions running" {
		t.Errorf("Message = %q, want %q", result.Message, "No Gas Town sessions running")
	}
}

func TestEnvVarsCheck_MayorCorrect(t *testing.T) {
	expected := config.AgentEnvSimple("mayor", "", "")
	reader := &mockEnvReader{
		sessions: []string{"hq-mayor"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_MayorMissing(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{"hq-mayor"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor": {}, // Missing all env vars
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_WitnessCorrect(t *testing.T) {
	expected := config.AgentEnvSimple("witness", "myrig", "")
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-witness"},
		sessionEnvs: map[string]map[string]string{
			"gt-myrig-witness": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_WitnessMismatch(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-witness"},
		sessionEnvs: map[string]map[string]string{
			"gt-myrig-witness": {
				"GT_ROLE": "witness",
				"GT_RIG":  "wrongrig", // Wrong rig
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_RefineryCorrect(t *testing.T) {
	expected := config.AgentEnvSimple("refinery", "myrig", "")
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-refinery"},
		sessionEnvs: map[string]map[string]string{
			"gt-myrig-refinery": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_PolecatCorrect(t *testing.T) {
	expected := config.AgentEnvSimple("polecat", "myrig", "Toast")
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-Toast"},
		sessionEnvs: map[string]map[string]string{
			"gt-myrig-Toast": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_PolecatMissing(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-Toast"},
		sessionEnvs: map[string]map[string]string{
			"gt-myrig-Toast": {
				"GT_ROLE": "polecat",
				// Missing GT_RIG, GT_POLECAT, BD_ACTOR, GIT_AUTHOR_NAME
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_CrewCorrect(t *testing.T) {
	expected := config.AgentEnvSimple("crew", "myrig", "worker1")
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-crew-worker1"},
		sessionEnvs: map[string]map[string]string{
			"gt-myrig-crew-worker1": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_MultipleSessions(t *testing.T) {
	mayorEnv := config.AgentEnvSimple("mayor", "", "")
	witnessEnv := config.AgentEnvSimple("witness", "rig1", "")
	polecatEnv := config.AgentEnvSimple("polecat", "rig1", "Toast")

	reader := &mockEnvReader{
		sessions: []string{"hq-mayor", "gt-rig1-witness", "gt-rig1-Toast"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor":        mayorEnv,
			"gt-rig1-witness": witnessEnv,
			"gt-rig1-Toast":   polecatEnv,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Message != "All 3 session(s) have correct environment variables" {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestEnvVarsCheck_MixedCorrectAndMismatch(t *testing.T) {
	mayorEnv := config.AgentEnvSimple("mayor", "", "")

	reader := &mockEnvReader{
		sessions: []string{"hq-mayor", "gt-rig1-witness"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor": mayorEnv,
			"gt-rig1-witness": {
				"GT_ROLE": "witness",
				// Missing GT_RIG and other vars
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_DeaconSkipped(t *testing.T) {
	// Deacon should be skipped - it doesn't use standard env vars
	reader := &mockEnvReader{
		sessions: []string{"hq-deacon"},
		sessionEnvs: map[string]map[string]string{
			"hq-deacon": {}, // Empty - but should be skipped
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	// With only deacon (skipped), checkedCount=0, so should show "No Gas Town sessions"
	// Actually no - the session exists but is skipped. Let me check the logic...
	// The check filters to gt-* and hq-* first, then skips deacon. With deacon skipped,
	// checkedCount stays 0, but gtSessions has 1 entry.
	// Looking at the code, after skipping deacon, checkedCount=0 and mismatches is empty,
	// so it returns OK with "All 0 session(s) have correct environment variables"
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_GetEnvError(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{"gt-myrig-witness"},
		envErrs: map[string]error{
			"gt-myrig-witness": errors.New("session not found"),
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_HyphenatedRig(t *testing.T) {
	// Test rig name with hyphens: "foo-bar"
	expected := config.AgentEnvSimple("witness", "foo-bar", "")
	reader := &mockEnvReader{
		sessions: []string{"gt-foo-bar-witness"},
		sessionEnvs: map[string]map[string]string{
			"gt-foo-bar-witness": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}
