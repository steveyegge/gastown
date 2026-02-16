package doctor

import (
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
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

// testTownRoot is the town root used in tests.
// Tests use this fixed path so expected values match what the check generates.
const testTownRoot = "/town"

// expectedEnv generates expected env vars matching what the check generates.
func expectedEnv(role, rig, agentName string) map[string]string {
	return config.AgentEnv(config.AgentEnvConfig{
		Role:      role,
		Rig:       rig,
		AgentName: agentName,
		TownRoot:  testTownRoot,
	})
}

// testCtx returns a CheckContext with the test town root.
func testCtx() *CheckContext {
	return &CheckContext{TownRoot: testTownRoot}
}

// setupEnvTestRegistry sets up a prefix registry for env check tests.
// Maps: mr→myrig, r1→rig1, fb→foo-bar, plus standard prefixes.
func setupEnvTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("bd", "beads")
	reg.Register("mr", "myrig")
	reg.Register("r1", "rig1")
	reg.Register("fb", "foo-bar")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestEnvVarsCheck_NoSessions(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

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
	result := check.Run(testCtx())

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
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Message != "No Gas Town sessions running" {
		t.Errorf("Message = %q, want %q", result.Message, "No Gas Town sessions running")
	}
}

func TestEnvVarsCheck_MayorCorrect(t *testing.T) {
	expected := expectedEnv("mayor", "", "")
	reader := &mockEnvReader{
		sessions: []string{"hq-mayor"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

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
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_WitnessCorrect(t *testing.T) {
	setupEnvTestRegistry(t)
	expected := expectedEnv("witness", "myrig", "")
	reader := &mockEnvReader{
		sessions: []string{"mr-witness"},
		sessionEnvs: map[string]map[string]string{
			"mr-witness": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_WitnessMismatch(t *testing.T) {
	setupEnvTestRegistry(t)
	reader := &mockEnvReader{
		sessions: []string{"mr-witness"},
		sessionEnvs: map[string]map[string]string{
			"mr-witness": {
				"GT_ROLE": "witness",
				"GT_RIG":  "wrongrig", // Wrong rig
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_RefineryCorrect(t *testing.T) {
	setupEnvTestRegistry(t)
	expected := expectedEnv("refinery", "myrig", "")
	reader := &mockEnvReader{
		sessions: []string{"mr-refinery"},
		sessionEnvs: map[string]map[string]string{
			"mr-refinery": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_PolecatCorrect(t *testing.T) {
	setupEnvTestRegistry(t)
	expected := expectedEnv("polecat", "myrig", "Toast")
	reader := &mockEnvReader{
		sessions: []string{"mr-Toast"},
		sessionEnvs: map[string]map[string]string{
			"mr-Toast": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_PolecatMissing(t *testing.T) {
	setupEnvTestRegistry(t)
	reader := &mockEnvReader{
		sessions: []string{"mr-Toast"},
		sessionEnvs: map[string]map[string]string{
			"mr-Toast": {
				"GT_ROLE": "polecat",
				// Missing GT_RIG, GT_POLECAT, BD_ACTOR, GIT_AUTHOR_NAME
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_CrewCorrect(t *testing.T) {
	setupEnvTestRegistry(t)
	expected := expectedEnv("crew", "myrig", "worker1")
	reader := &mockEnvReader{
		sessions: []string{"mr-crew-worker1"},
		sessionEnvs: map[string]map[string]string{
			"mr-crew-worker1": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_MultipleSessions(t *testing.T) {
	setupEnvTestRegistry(t)
	mayorEnv := expectedEnv("mayor", "", "")
	witnessEnv := expectedEnv("witness", "rig1", "")
	polecatEnv := expectedEnv("polecat", "rig1", "Toast")

	reader := &mockEnvReader{
		sessions: []string{"hq-mayor", "r1-witness", "r1-Toast"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor":   mayorEnv,
			"r1-witness": witnessEnv,
			"r1-Toast":   polecatEnv,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
	if result.Message != "All 3 session(s) have correct environment variables" {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestEnvVarsCheck_MixedCorrectAndMismatch(t *testing.T) {
	setupEnvTestRegistry(t)
	mayorEnv := expectedEnv("mayor", "", "")

	reader := &mockEnvReader{
		sessions: []string{"hq-mayor", "r1-witness"},
		sessionEnvs: map[string]map[string]string{
			"hq-mayor": mayorEnv,
			"r1-witness": {
				"GT_ROLE": "witness",
				// Missing GT_RIG and other vars
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_DeaconCorrect(t *testing.T) {
	expected := expectedEnv("deacon", "", "")
	reader := &mockEnvReader{
		sessions: []string{"hq-deacon"},
		sessionEnvs: map[string]map[string]string{
			"hq-deacon": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_DeaconMissing(t *testing.T) {
	reader := &mockEnvReader{
		sessions: []string{"hq-deacon"},
		sessionEnvs: map[string]map[string]string{
			"hq-deacon": {}, // Missing all env vars
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_GetEnvError(t *testing.T) {
	setupEnvTestRegistry(t)
	reader := &mockEnvReader{
		sessions: []string{"mr-witness"},
		envErrs: map[string]error{
			"mr-witness": errors.New("session not found"),
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
}

func TestEnvVarsCheck_HyphenatedRig(t *testing.T) {
	setupEnvTestRegistry(t)
	// Test rig name with hyphens: "foo-bar" has prefix "fb"
	expected := expectedEnv("witness", "foo-bar", "")
	reader := &mockEnvReader{
		sessions: []string{"fb-witness"},
		sessionEnvs: map[string]map[string]string{
			"fb-witness": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_BootCorrect(t *testing.T) {
	// Boot watchdog session (hq-boot) uses "boot" role in AgentEnv,
	// even though ParseSessionName returns Role=deacon, Name="boot".
	expected := expectedEnv("boot", "", "boot")
	reader := &mockEnvReader{
		sessions: []string{"hq-boot"},
		sessionEnvs: map[string]map[string]string{
			"hq-boot": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK", result.Status)
	}
}

func TestEnvVarsCheck_BeadsDirWarning(t *testing.T) {
	setupEnvTestRegistry(t)
	// BEADS_DIR being set breaks prefix-based routing
	expected := expectedEnv("witness", "myrig", "")
	expected["BEADS_DIR"] = "/some/path/.beads" // This shouldn't be set!
	reader := &mockEnvReader{
		sessions: []string{"mr-witness"},
		sessionEnvs: map[string]map[string]string{
			"mr-witness": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
	if !strings.Contains(result.Message, "BEADS_DIR") {
		t.Errorf("Message should mention BEADS_DIR, got: %q", result.Message)
	}
	if !strings.Contains(result.FixHint, "gt shutdown") {
		t.Errorf("FixHint should mention restart, got: %q", result.FixHint)
	}
}

func TestEnvVarsCheck_BeadsDirEmptyIsOK(t *testing.T) {
	setupEnvTestRegistry(t)
	// Empty BEADS_DIR should not warn
	expected := expectedEnv("witness", "myrig", "")
	expected["BEADS_DIR"] = "" // Empty is fine
	reader := &mockEnvReader{
		sessions: []string{"mr-witness"},
		sessionEnvs: map[string]map[string]string{
			"mr-witness": expected,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want StatusOK for empty BEADS_DIR", result.Status)
	}
}

func TestEnvVarsCheck_BeadsDirMultipleSessions(t *testing.T) {
	setupEnvTestRegistry(t)
	// Multiple sessions, only one has BEADS_DIR
	witnessEnv := expectedEnv("witness", "myrig", "")
	polecatEnv := expectedEnv("polecat", "myrig", "Toast")
	polecatEnv["BEADS_DIR"] = "/bad/path" // This shouldn't be set!

	reader := &mockEnvReader{
		sessions: []string{"mr-witness", "mr-Toast"},
		sessionEnvs: map[string]map[string]string{
			"mr-witness": witnessEnv,
			"mr-Toast":   polecatEnv,
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
	if !strings.Contains(result.Message, "1 session") {
		t.Errorf("Message should mention 1 session with BEADS_DIR, got: %q", result.Message)
	}
}

func TestEnvVarsCheck_BeadsDirWithOtherMismatches(t *testing.T) {
	setupEnvTestRegistry(t)
	// Session has BEADS_DIR AND other mismatches - both should be reported
	reader := &mockEnvReader{
		sessions: []string{"mr-witness"},
		sessionEnvs: map[string]map[string]string{
			"mr-witness": {
				"GT_ROLE":   "witness",
				"GT_RIG":    "wrongrig", // Mismatch
				"BEADS_DIR": "/bad/path",
			},
		},
	}
	check := NewEnvVarsCheckWithReader(reader)
	result := check.Run(testCtx())

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want StatusWarning", result.Status)
	}
	// BEADS_DIR takes priority in message
	if !strings.Contains(result.Message, "BEADS_DIR") {
		t.Errorf("Message should prioritize BEADS_DIR, got: %q", result.Message)
	}
	// But details should include both
	detailsStr := strings.Join(result.Details, "\n")
	if !strings.Contains(detailsStr, "BEADS_DIR") {
		t.Errorf("Details should mention BEADS_DIR")
	}
	if !strings.Contains(detailsStr, "Other env var issues") {
		t.Errorf("Details should mention other issues")
	}
}
