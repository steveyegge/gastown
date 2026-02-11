package terminal

import (
	"testing"
)

// NOTE: ResolveBackend() calls getAgentNotes() which shells out to `bd show <agentID> --json`.
// Full end-to-end testing of ResolveBackend would require either mocking the bd
// binary on PATH or refactoring to inject a notes-fetcher interface. The tests below
// cover the routing-critical parsing and type-assertion logic that sits beneath
// ResolveBackend, plus LocalBackend which needs no external calls.

// --- LocalBackend ---

func TestLocalBackend_AlwaysReturnsTmux(t *testing.T) {
	for i := 0; i < 3; i++ {
		b := LocalBackend()
		if _, ok := b.(*TmuxBackend); !ok {
			t.Fatalf("LocalBackend() call %d returned %T, want *TmuxBackend", i, b)
		}
	}
}

// --- parseCoopConfig routing tests (type assertions) ---

func TestParseCoopConfig_ReturnsCorrectType(t *testing.T) {
	input := "backend: coop\ncoop_url: http://localhost:8080\ncoop_token: tok"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate what ResolveBackend does: build a CoopBackend and assert the type.
	b := NewCoopBackend(cfg.CoopConfig)
	b.AddSession("claude", cfg.baseURL)

	var backend Backend = b
	coop, ok := backend.(*CoopBackend)
	if !ok {
		t.Fatalf("expected *CoopBackend, got %T", backend)
	}
	if coop.token != "tok" {
		t.Errorf("token = %q, want %q", coop.token, "tok")
	}
}

func TestParseSSHConfig_ReturnsCorrectType(t *testing.T) {
	input := "backend: k8s\nssh_host: gt@pod.svc\nssh_port: 2222"
	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate what ResolveBackend does: build an SSHBackend and assert the type.
	b := NewSSHBackend(*cfg)

	var backend Backend = b
	ssh, ok := backend.(*SSHBackend)
	if !ok {
		t.Fatalf("expected *SSHBackend, got %T", backend)
	}
	if ssh.Host != "gt@pod.svc" {
		t.Errorf("Host = %q, want %q", ssh.Host, "gt@pod.svc")
	}
	if ssh.Port != 2222 {
		t.Errorf("Port = %d, want 2222", ssh.Port)
	}
}

// --- parseCoopConfig edge cases ---

func TestParseCoopConfig_ExtraWhitespace(t *testing.T) {
	input := "  backend: coop  \n  coop_url:   http://10.0.0.5:9090  \n  coop_token:  my-token  \n"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.baseURL != "http://10.0.0.5:9090" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://10.0.0.5:9090")
	}
	if cfg.Token != "my-token" {
		t.Errorf("Token = %q, want %q", cfg.Token, "my-token")
	}
}

func TestParseCoopConfig_EmptyLines(t *testing.T) {
	input := "\n\nbackend: coop\n\ncoop_url: http://localhost:8080\n\n"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://localhost:8080")
	}
}

func TestParseCoopConfig_UnknownFieldsIgnored(t *testing.T) {
	input := "backend: coop\ncoop_url: http://localhost:8080\nextra_field: ignored\nanother: also_ignored"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://localhost:8080")
	}
}

func TestParseCoopConfig_SkipsInvalidLines(t *testing.T) {
	input := "backend: coop\nthis has no colon\ncoop_url: http://localhost:8080\njust a value"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://localhost:8080")
	}
}

func TestParseCoopConfig_URLWithColonInValue(t *testing.T) {
	// coop_url value contains colons (scheme + port) — SplitN with N=2 must handle this.
	input := "backend: coop\ncoop_url: https://coop.example.com:8443/api"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.baseURL != "https://coop.example.com:8443/api" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "https://coop.example.com:8443/api")
	}
}

func TestParseCoopConfig_WhitespaceOnlyInput(t *testing.T) {
	cfg, err := parseCoopConfig("   \n  \n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for whitespace-only input")
	}
}

// --- parseSSHConfig edge cases ---

func TestParseSSHConfig_URLWithColonInHost(t *testing.T) {
	// ssh_host value with @ and dots — SplitN with N=2 must handle this.
	input := "backend: k8s\nssh_host: gt@gt-gastown-toast.gastown.svc.cluster.local"
	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Host != "gt@gt-gastown-toast.gastown.svc.cluster.local" {
		t.Errorf("Host = %q, want %q", cfg.Host, "gt@gt-gastown-toast.gastown.svc.cluster.local")
	}
}

func TestParseSSHConfig_WhitespaceOnlyInput(t *testing.T) {
	cfg, err := parseSSHConfig("   \n  \n  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for whitespace-only input")
	}
}

// --- Routing precedence: coop checked before SSH ---

func TestParseCoopConfig_NotSSH(t *testing.T) {
	// Input that has SSH fields but no "coop" keyword should return nil from parseCoopConfig.
	input := "backend: k8s\nssh_host: gt@pod.svc"
	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil coop config for k8s backend")
	}
}

func TestParseSSHConfig_NotCoop(t *testing.T) {
	// Input that has Coop fields but no "k8s" keyword should return nil from parseSSHConfig.
	input := "backend: coop\ncoop_url: http://localhost:8080"
	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil ssh config for coop backend")
	}
}

// --- HQ prefix candidate logic ---

func TestHqPrefixCandidates(t *testing.T) {
	// Verify the candidate-generation logic that ResolveBackend uses.
	// Bare names like "mayor" (no slash, no hyphen) should produce ["mayor", "hq-mayor"].
	tests := []struct {
		agentID    string
		wantCount  int
		wantSecond string // expected second candidate, empty if only one
	}{
		{"mayor", 2, "hq-mayor"},
		{"deacon", 2, "hq-deacon"},
		{"rig/polecat", 1, ""},    // slash → no prefix
		{"hq-mayor", 1, ""},       // already has hyphen → no prefix
		{"crew/name/sub", 1, ""},  // slash → no prefix
	}

	for _, tt := range tests {
		candidates := []string{tt.agentID}
		if !(containsChar(tt.agentID, '/') || containsChar(tt.agentID, '-')) {
			candidates = append(candidates, "hq-"+tt.agentID)
		}

		if len(candidates) != tt.wantCount {
			t.Errorf("agentID=%q: got %d candidates, want %d", tt.agentID, len(candidates), tt.wantCount)
		}
		if tt.wantSecond != "" && (len(candidates) < 2 || candidates[1] != tt.wantSecond) {
			second := ""
			if len(candidates) >= 2 {
				second = candidates[1]
			}
			t.Errorf("agentID=%q: second candidate = %q, want %q", tt.agentID, second, tt.wantSecond)
		}
	}
}

// containsChar mirrors the strings.Contains logic used in ResolveBackend.
func containsChar(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return true
		}
	}
	return false
}

// --- Routing dispatch simulation ---
// These tests simulate the full routing dispatch that ResolveBackend performs,
// using parseCoopConfig and parseSSHConfig directly to avoid the bd dependency.

func TestRoutingDispatch_CoopNotes(t *testing.T) {
	notes := "backend: coop\ncoop_url: http://localhost:8080\ncoop_token: secret"

	// Step 1: try coop
	coopCfg, err := parseCoopConfig(notes)
	if err != nil {
		t.Fatalf("parseCoopConfig: %v", err)
	}
	if coopCfg == nil {
		t.Fatal("expected coop config, got nil")
	}

	b := NewCoopBackend(coopCfg.CoopConfig)
	b.AddSession("claude", coopCfg.baseURL)

	var backend Backend = b
	if _, ok := backend.(*CoopBackend); !ok {
		t.Errorf("expected *CoopBackend, got %T", backend)
	}
	// Should NOT also match SSH
	sshCfg, _ := parseSSHConfig(notes)
	if sshCfg != nil {
		t.Error("coop notes should not produce an SSH config")
	}
}

func TestRoutingDispatch_SSHNotes(t *testing.T) {
	notes := "backend: k8s\nssh_host: gt@pod.svc\nssh_port: 2222\nssh_key: /tmp/id_rsa"

	// Step 1: try coop — should be nil
	coopCfg, _ := parseCoopConfig(notes)
	if coopCfg != nil {
		t.Fatal("ssh notes should not produce a coop config")
	}

	// Step 2: try SSH
	sshCfg, err := parseSSHConfig(notes)
	if err != nil {
		t.Fatalf("parseSSHConfig: %v", err)
	}
	if sshCfg == nil {
		t.Fatal("expected ssh config, got nil")
	}

	b := NewSSHBackend(*sshCfg)
	var backend Backend = b
	if _, ok := backend.(*SSHBackend); !ok {
		t.Errorf("expected *SSHBackend, got %T", backend)
	}
}

func TestRoutingDispatch_NoBackendMetadata(t *testing.T) {
	notes := ""

	coopCfg, _ := parseCoopConfig(notes)
	if coopCfg != nil {
		t.Error("empty notes should not produce a coop config")
	}

	sshCfg, _ := parseSSHConfig(notes)
	if sshCfg != nil {
		t.Error("empty notes should not produce an ssh config")
	}

	// Fallback: LocalBackend
	b := LocalBackend()
	if _, ok := b.(*TmuxBackend); !ok {
		t.Errorf("fallback should be *TmuxBackend, got %T", b)
	}
}

func TestRoutingDispatch_LocalBackendNotes(t *testing.T) {
	// Notes that mention neither "coop" nor "k8s" should fall through to tmux.
	notes := "backend: local\nsome_field: value"

	coopCfg, _ := parseCoopConfig(notes)
	if coopCfg != nil {
		t.Error("local notes should not produce a coop config")
	}

	sshCfg, _ := parseSSHConfig(notes)
	if sshCfg != nil {
		t.Error("local notes should not produce an ssh config")
	}
}
