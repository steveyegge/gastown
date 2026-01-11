package doctor

import (
	"testing"
)

// mockTmuxEnv implements TmuxEnvGetter for testing.
type mockTmuxEnv struct {
	sessions map[string]map[string]string // session -> env vars
	listErr  error
	getErr   error
}

func (m *mockTmuxEnv) ListSessions() ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	sessions := make([]string, 0, len(m.sessions))
	for s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (m *mockTmuxEnv) GetEnvironment(session, key string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	if env, ok := m.sessions[session]; ok {
		return env[key], nil
	}
	return "", nil
}

func TestGTRootCheck_NoSessions(t *testing.T) {
	mock := &mockTmuxEnv{sessions: map[string]map[string]string{}}
	check := NewGTRootCheckWithTmux(mock)

	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
	if result.Message != "No Gas Town sessions running" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestGTRootCheck_NoGasTownSessions(t *testing.T) {
	mock := &mockTmuxEnv{
		sessions: map[string]map[string]string{
			"other-session": {"SOME_VAR": "value"},
		},
	}
	check := NewGTRootCheckWithTmux(mock)

	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
	if result.Message != "No Gas Town sessions running" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestGTRootCheck_AllSessionsHaveGTRoot(t *testing.T) {
	mock := &mockTmuxEnv{
		sessions: map[string]map[string]string{
			"hq-mayor":          {"GT_ROOT": "/home/user/gt", "GT_ROLE": "mayor"},
			"hq-deacon":         {"GT_ROOT": "/home/user/gt", "GT_ROLE": "deacon"},
			"gt-myrig-witness":  {"GT_ROOT": "/home/user/gt", "GT_ROLE": "witness"},
			"gt-myrig-refinery": {"GT_ROOT": "/home/user/gt", "GT_ROLE": "refinery"},
		},
	}
	check := NewGTRootCheckWithTmux(mock)

	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
	if result.Message != "All 4 session(s) have GT_ROOT set" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestGTRootCheck_MissingGTRoot(t *testing.T) {
	mock := &mockTmuxEnv{
		sessions: map[string]map[string]string{
			"hq-mayor":          {"GT_ROOT": "/home/user/gt"},
			"gt-myrig-witness":  {"GT_ROLE": "witness"}, // Missing GT_ROOT
			"gt-myrig-refinery": {"GT_ROLE": "refinery"}, // Missing GT_ROOT
		},
	}
	check := NewGTRootCheckWithTmux(mock)

	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
	if result.Message != "2 session(s) missing GT_ROOT environment variable" {
		t.Errorf("unexpected message: %s", result.Message)
	}
	if result.FixHint != "Restart sessions to pick up GT_ROOT: gt shutdown && gt up" {
		t.Errorf("unexpected fix hint: %s", result.FixHint)
	}
}

func TestGTRootCheck_EmptyGTRoot(t *testing.T) {
	mock := &mockTmuxEnv{
		sessions: map[string]map[string]string{
			"hq-mayor": {"GT_ROOT": ""}, // Empty GT_ROOT should be treated as missing
		},
	}
	check := NewGTRootCheckWithTmux(mock)

	result := check.Run(&CheckContext{})

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
}

func TestGTRootCheck_MixedPrefixes(t *testing.T) {
	// Test that both gt-* and hq-* sessions are checked
	mock := &mockTmuxEnv{
		sessions: map[string]map[string]string{
			"hq-mayor":         {"GT_ROOT": "/home/user/gt"},
			"gt-rig-witness":   {"GT_ROOT": "/home/user/gt"},
			"other-session":    {}, // Should be ignored
			"random":           {}, // Should be ignored
		},
	}
	check := NewGTRootCheckWithTmux(mock)

	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
	// Should only count the 2 Gas Town sessions
	if result.Message != "All 2 session(s) have GT_ROOT set" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}
