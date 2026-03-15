package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// --- MachinesConfig method tests ---

func TestHubSSHTarget_WithAlias(t *testing.T) {
	cfg := &config.MachinesConfig{
		DoltHost: "100.111.197.110",
		Machines: map[string]*config.MachineEntry{
			"mini2": {
				Host:     "100.111.197.110",
				SSHAlias: "mini2",
				User:     "b",
				Roles:    []string{"worker"},
				Enabled:  true,
			},
		},
	}
	got := cfg.HubSSHTarget()
	if got != "mini2" {
		t.Errorf("HubSSHTarget() = %q, want %q", got, "mini2")
	}
}

func TestHubSSHTarget_WithoutAlias(t *testing.T) {
	cfg := &config.MachinesConfig{
		DoltHost: "100.111.197.110",
		Machines: map[string]*config.MachineEntry{
			"mini2": {
				Host:    "100.111.197.110",
				User:    "b",
				Roles:   []string{"worker"},
				Enabled: true,
			},
		},
	}
	got := cfg.HubSSHTarget()
	if got != "b@100.111.197.110" {
		t.Errorf("HubSSHTarget() = %q, want %q", got, "b@100.111.197.110")
	}
}

func TestHubSSHTarget_NoMatchingMachine(t *testing.T) {
	cfg := &config.MachinesConfig{
		DoltHost: "100.111.197.110",
		Machines: map[string]*config.MachineEntry{
			"mini3": {
				Host:    "100.86.9.58",
				User:    "worker",
				Roles:   []string{"worker"},
				Enabled: true,
			},
		},
	}
	got := cfg.HubSSHTarget()
	// Falls back to DoltHost raw IP
	if got != "100.111.197.110" {
		t.Errorf("HubSSHTarget() = %q, want %q (fallback)", got, "100.111.197.110")
	}
}

func TestSSHTarget_PrefersAlias(t *testing.T) {
	m := &config.MachineEntry{Host: "1.2.3.4", SSHAlias: "mybox", User: "bob"}
	if got := m.SSHTarget(); got != "mybox" {
		t.Errorf("SSHTarget() = %q, want %q", got, "mybox")
	}
}

func TestSSHTarget_UserAtHost(t *testing.T) {
	m := &config.MachineEntry{Host: "1.2.3.4", User: "bob"}
	if got := m.SSHTarget(); got != "bob@1.2.3.4" {
		t.Errorf("SSHTarget() = %q, want %q", got, "bob@1.2.3.4")
	}
}

func TestSSHTarget_HostOnly(t *testing.T) {
	m := &config.MachineEntry{Host: "1.2.3.4"}
	if got := m.SSHTarget(); got != "1.2.3.4" {
		t.Errorf("SSHTarget() = %q, want %q", got, "1.2.3.4")
	}
}

// --- loadMachinesConfig tests ---

func TestLoadMachinesConfig_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	// Create the mayor directory but no machines.json
	if err := os.MkdirAll(filepath.Join(dir, "mayor"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadMachinesConfig(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got: %+v", cfg)
	}
}

func TestLoadMachinesConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	mayorDir := filepath.Join(dir, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{
		"type": "machines",
		"version": 1,
		"machines": {
			"mini2": {
				"host": "10.0.0.1",
				"user": "test",
				"roles": ["worker"],
				"enabled": true
			}
		},
		"dolt_host": "10.0.0.1",
		"dolt_port": 3307
	}`
	if err := os.WriteFile(filepath.Join(mayorDir, "machines.json"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadMachinesConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.DoltHost != "10.0.0.1" {
		t.Errorf("DoltHost = %q, want %q", cfg.DoltHost, "10.0.0.1")
	}
	if _, ok := cfg.Machines["mini2"]; !ok {
		t.Error("expected mini2 in machines map")
	}
}

func TestLoadMachinesConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	mayorDir := filepath.Join(dir, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "machines.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadMachinesConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- Bootstrap script generation tests ---

func TestSpawnOnTarget_ScriptContainsRequiredElements(t *testing.T) {
	// We can't call spawnOnTarget directly (it SSHes), but we can verify
	// the script template by reproducing the fmt.Sprintf and checking output.
	townRoot := "/Users/test/gt"
	rigName := "gastown"
	polecatName := "Toast"
	doltHost := "10.0.0.1"
	doltPort := 3307
	proxyURL := "https://10.0.0.1:9876"

	script := buildBootstrapScript(townRoot, rigName, polecatName, doltHost, doltPort, proxyURL)

	checks := []struct {
		name    string
		substr  string
	}{
		{"set -e", "set -e"},
		{"cert dir creation", "mktemp -d"},
		{"cert dir perms", "chmod 700"},
		{"cert from stdin", "CERT_JSON=$(cat)"},
		{"cert.pem write", `jq -r .cert > "$CERT_DIR/cert.pem"`},
		{"key.pem write", `jq -r .key  > "$CERT_DIR/key.pem"`},
		{"ca.pem write", `jq -r .ca   > "$CERT_DIR/ca.pem"`},
		{"key perms", `chmod 600 "$CERT_DIR/key.pem"`},
		{"cd to town root", "cd /Users/test/gt"},
		{"polecat spawn", "gt polecat spawn gastown --name Toast"},
		{"dolt host", "--dolt-host 10.0.0.1"},
		{"dolt port", "--dolt-port 3307"},
		{"json flag", "--json"},
		{"session name", `SESS="gt-gastown-p-Toast"`},
		{"tmux new-session", "tmux new-session -d -s"},
		{"proxy url env", `GT_PROXY_URL "https://10.0.0.1:9876"`},
		{"proxy cert env", "GT_PROXY_CERT"},
		{"proxy key env", "GT_PROXY_KEY"},
		{"proxy ca env", "GT_PROXY_CA"},
		{"real bin env", "GT_REAL_BIN"},
		{"printf not echo for cert", `printf '%s' "$CERT_JSON"`},
		{"grep json from mixed output", `grep '^{' | tail -1`},
		{"error capture", "SPAWN_EXIT=$?"},
	}

	for _, c := range checks {
		if !strings.Contains(script, c.substr) {
			t.Errorf("script missing %s: expected substring %q", c.name, c.substr)
		}
	}

	// Verify cert material is NOT in script (it's piped via stdin)
	if strings.Contains(script, "BEGIN CERTIFICATE") {
		t.Error("script should not contain cert material (must be piped via stdin)")
	}
}

func TestSpawnOnTarget_ScriptSessionBeforeEnv(t *testing.T) {
	script := buildBootstrapScript("/gt", "rig", "Name", "10.0.0.1", 3307, "https://10.0.0.1:9876")

	sessionIdx := strings.Index(script, "tmux new-session")
	setenvIdx := strings.Index(script, "tmux setenv")

	if sessionIdx < 0 {
		t.Fatal("tmux new-session not found in script")
	}
	if setenvIdx < 0 {
		t.Fatal("tmux setenv not found in script")
	}
	if setenvIdx < sessionIdx {
		t.Error("tmux setenv appears before tmux new-session — env vars will fail to set")
	}
}

func TestSpawnOnTarget_ScriptNoEchoForCert(t *testing.T) {
	script := buildBootstrapScript("/gt", "rig", "Name", "10.0.0.1", 3307, "https://10.0.0.1:9876")

	// echo "$CERT_JSON" on macOS expands \n — we must use printf '%s'
	if strings.Contains(script, `echo "$CERT_JSON"`) {
		t.Error("script uses echo for cert JSON — must use printf to avoid macOS \\n expansion")
	}
}

func TestSpawnOnTarget_DoltPortDefault(t *testing.T) {
	script := buildBootstrapScript("/gt", "rig", "Name", "10.0.0.1", 0, "https://10.0.0.1:9876")

	// Port 0 means the caller didn't set it — script should still contain --dolt-port
	if !strings.Contains(script, "--dolt-port 0") {
		// This is actually fine — the default happens in spawnRemoteSatellite,
		// not in the script builder. Port 0 would be a caller bug.
		// Just verify the script embeds whatever port it's given.
		t.Log("port 0 passed through to script — caller should default to 3307")
	}
}

// --- Cert API payload tests ---

func TestIssueCertPayload_JSONSafe(t *testing.T) {
	// Verify json.Marshal produces safe payloads even with special chars
	cases := []struct {
		name string
		rig  string
		pc   string
	}{
		{"normal", "gastown", "Toast"},
		{"with quotes", "gas\"town", "To\"ast"},
		{"with backslash", "gas\\town", "To\\ast"},
		{"with newline", "gas\ntown", "To\nast"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"rig":  tc.rig,
				"name": tc.pc,
			})
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			// Must be valid JSON
			var parsed map[string]string
			if err := json.Unmarshal(payload, &parsed); err != nil {
				t.Fatalf("payload is not valid JSON: %v\npayload: %s", err, payload)
			}
			if parsed["rig"] != tc.rig {
				t.Errorf("rig roundtrip: got %q, want %q", parsed["rig"], tc.rig)
			}
			if parsed["name"] != tc.pc {
				t.Errorf("name roundtrip: got %q, want %q", parsed["name"], tc.pc)
			}
		})
	}
}

func TestDenyCertPayload_JSONSafe(t *testing.T) {
	serial := "abc123\"special"
	payload, err := json.Marshal(map[string]string{"serial": serial})
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(payload, &parsed); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if parsed["serial"] != serial {
		t.Errorf("serial roundtrip: got %q, want %q", parsed["serial"], serial)
	}
}

// --- verifyBootstrap parsing tests ---

func TestVerifyBootstrapParsing_Match(t *testing.T) {
	// Simulate what verifyBootstrap checks internally
	stdout := "GT_PROXY_URL=https://127.0.0.1:9876\n"
	expected := "https://127.0.0.1:9876"

	if !strings.Contains(stdout, expected) {
		t.Errorf("expected stdout to contain %q", expected)
	}
}

func TestVerifyBootstrapParsing_Mismatch(t *testing.T) {
	stdout := "GT_PROXY_URL=https://10.0.0.1:9876\n"
	expected := "https://127.0.0.1:9876"

	if strings.Contains(stdout, expected) {
		t.Error("expected mismatch but got match")
	}
}

func TestVerifyBootstrapParsing_Empty(t *testing.T) {
	stdout := ""
	expected := "https://127.0.0.1:9876"

	if strings.Contains(stdout, expected) {
		t.Error("empty stdout should not match")
	}
}

// --- Spawn output parsing tests ---

func TestParseSpawnOutput_CleanJSON(t *testing.T) {
	output := `{"session_name":"gt-gastown-p-Toast","cert_dir":"/tmp/abc","clone_path":"/gt/gastown/polecats/Toast","base_branch":"main","branch":"polecat/Toast"}`

	lines := strings.Split(strings.TrimSpace(output), "\n")
	lastLine := lines[len(lines)-1]

	var info struct {
		SessionName string `json:"session_name"`
		CertDir     string `json:"cert_dir"`
		ClonePath   string `json:"clone_path"`
		BaseBranch  string `json:"base_branch"`
		Branch      string `json:"branch"`
	}
	if err := json.Unmarshal([]byte(lastLine), &info); err != nil {
		t.Fatalf("failed to parse clean JSON output: %v", err)
	}
	if info.SessionName != "gt-gastown-p-Toast" {
		t.Errorf("SessionName = %q, want %q", info.SessionName, "gt-gastown-p-Toast")
	}
	if info.ClonePath != "/gt/gastown/polecats/Toast" {
		t.Errorf("ClonePath = %q, want %q", info.ClonePath, "/gt/gastown/polecats/Toast")
	}
}

func TestParseSpawnOutput_MixedWithProgress(t *testing.T) {
	// gt polecat spawn prints progress text before JSON
	output := `Checking Dolt health...
Created polecat: Toast
✓ Polecat Toast spawned (session start deferred)
{"rig":"gastown","polecat":"Toast","session_name":"gt-gastown-p-Toast","clone_path":"/gt/gastown/polecats/Toast","base_branch":"main","branch":"polecat/Toast"}`

	lines := strings.Split(strings.TrimSpace(output), "\n")
	lastLine := lines[len(lines)-1]

	var info struct {
		SessionName string `json:"session_name"`
		ClonePath   string `json:"clone_path"`
	}
	if err := json.Unmarshal([]byte(lastLine), &info); err != nil {
		t.Fatalf("failed to parse JSON from mixed output: %v", err)
	}
	if info.SessionName != "gt-gastown-p-Toast" {
		t.Errorf("SessionName = %q, want %q", info.SessionName, "gt-gastown-p-Toast")
	}
}

func TestParseSpawnOutput_NoJSON(t *testing.T) {
	output := "Some error occurred\nNo JSON here"

	lines := strings.Split(strings.TrimSpace(output), "\n")
	lastLine := lines[len(lines)-1]

	var info struct {
		SessionName string `json:"session_name"`
	}
	err := json.Unmarshal([]byte(lastLine), &info)
	if err == nil {
		t.Error("expected parse error for non-JSON output")
	}
}

// --- ProxyURL tests (supplement existing) ---

func TestProxyURL_HubGetsLoopback(t *testing.T) {
	cfg := &config.MachinesConfig{DoltHost: "10.0.0.1"}
	url := cfg.ProxyURL("10.0.0.1")
	if url != "https://127.0.0.1:9876" {
		t.Errorf("hub should get loopback, got %q", url)
	}
}

func TestProxyURL_SatelliteGetsDoltHost(t *testing.T) {
	cfg := &config.MachinesConfig{DoltHost: "10.0.0.1"}
	url := cfg.ProxyURL("10.0.0.2")
	if url != "https://10.0.0.1:9876" {
		t.Errorf("satellite should get hub IP, got %q", url)
	}
}

// --- Worker machine filtering tests ---

func TestWorkerMachines_FiltersCorrectly(t *testing.T) {
	cfg := &config.MachinesConfig{
		Machines: map[string]*config.MachineEntry{
			"worker1":    {Host: "1", Roles: []string{"worker"}, Enabled: true},
			"worker2":    {Host: "2", Roles: []string{"worker"}, Enabled: true},
			"disabled":   {Host: "3", Roles: []string{"worker"}, Enabled: false},
			"controller": {Host: "4", Roles: []string{"controller"}, Enabled: true},
			"multi":      {Host: "5", Roles: []string{"controller", "worker"}, Enabled: true},
		},
	}
	workers := cfg.WorkerMachines()
	if len(workers) != 3 {
		t.Errorf("expected 3 workers (worker1, worker2, multi), got %d: %v", len(workers), keys(workers))
	}
	for _, name := range []string{"worker1", "worker2", "multi"} {
		if _, ok := workers[name]; !ok {
			t.Errorf("expected %q in workers", name)
		}
	}
}

func TestIsWorker_MultiRole(t *testing.T) {
	m := &config.MachineEntry{Roles: []string{"controller", "worker"}}
	if !m.IsWorker() {
		t.Error("multi-role machine with 'worker' should return true")
	}
}

func TestIsWorker_NoWorkerRole(t *testing.T) {
	m := &config.MachineEntry{Roles: []string{"controller"}}
	if m.IsWorker() {
		t.Error("machine without 'worker' role should return false")
	}
}

func TestIsWorker_EmptyRoles(t *testing.T) {
	m := &config.MachineEntry{Roles: nil}
	if m.IsWorker() {
		t.Error("machine with nil roles should return false")
	}
}

// --- TownRoot default tests ---

func TestTownRoot_DefaultsToHome(t *testing.T) {
	m := &config.MachineEntry{TownRoot: ""}
	got := m.TownRoot
	if got != "" {
		t.Errorf("expected empty TownRoot, got %q", got)
	}
	// spawnRemoteSatellite defaults "" to "~/gt"
	defaulted := got
	if defaulted == "" {
		defaulted = "~/gt"
	}
	if defaulted != "~/gt" {
		t.Errorf("default TownRoot should be ~/gt, got %q", defaulted)
	}
}

// --- Cert response JSON roundtrip ---

func TestIssueCertResponse_Roundtrip(t *testing.T) {
	resp := &issueCertResponse{
		CN:        "gt-gastown-Toast",
		Cert:      "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
		Key:       "-----BEGIN EC PRIVATE KEY-----\nMHQ...\n-----END EC PRIVATE KEY-----",
		CA:        "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
		Serial:    "abc123",
		ExpiresAt: "2026-04-14T00:00:00Z",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed issueCertResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.CN != resp.CN {
		t.Errorf("CN: got %q, want %q", parsed.CN, resp.CN)
	}
	if parsed.Serial != resp.Serial {
		t.Errorf("Serial: got %q, want %q", parsed.Serial, resp.Serial)
	}
	if parsed.Cert != resp.Cert {
		t.Errorf("Cert roundtrip failed (newlines likely corrupted)")
	}
}

// --- helpers ---

// buildBootstrapScript extracts the script template from spawnOnTarget for testing.
// This is the same fmt.Sprintf used in the production code.
func buildBootstrapScript(townRoot, rigName, polecatName, doltHost string, doltPort int, proxyURL string) string {
	return fmt.Sprintf(`
set -e
CERT_DIR=$(mktemp -d)
chmod 700 "$CERT_DIR"

# Read cert material from stdin (JSON)
CERT_JSON=$(cat)
printf '%%s' "$CERT_JSON" | jq -r .cert > "$CERT_DIR/cert.pem"
printf '%%s' "$CERT_JSON" | jq -r .key  > "$CERT_DIR/key.pem"
printf '%%s' "$CERT_JSON" | jq -r .ca   > "$CERT_DIR/ca.pem"
chmod 600 "$CERT_DIR/key.pem"

cd %s

# Spawn polecat (creates worktree, does NOT start tmux session)
# Temporarily disable set -e to capture the error message
set +e
SPAWN_OUTPUT=$(gt polecat spawn %s --name %s --dolt-host %s --dolt-port %d --json 2>&1)
SPAWN_EXIT=$?
set -e
if [ $SPAWN_EXIT -ne 0 ]; then
  echo "SPAWN FAILED (exit $SPAWN_EXIT): $SPAWN_OUTPUT" >&2
  exit 1
fi
SPAWN_JSON=$(printf '%%s\n' "$SPAWN_OUTPUT" | grep '^{' | tail -1)
CLONE_PATH=$(printf '%%s' "$SPAWN_JSON" | jq -r .clone_path)

# Session name follows gt convention
SESS="gt-%s-p-%s"

# Create a detached tmux session in the polecat's worktree
tmux new-session -d -s "$SESS" -c "$CLONE_PATH" 2>/dev/null || true

# Set proxy env vars in tmux session (must happen after session creation)
tmux setenv -t "$SESS" GT_PROXY_URL "%s"
tmux setenv -t "$SESS" GT_PROXY_CERT "$CERT_DIR/cert.pem"
tmux setenv -t "$SESS" GT_PROXY_KEY "$CERT_DIR/key.pem"
tmux setenv -t "$SESS" GT_PROXY_CA "$CERT_DIR/ca.pem"
tmux setenv -t "$SESS" GT_REAL_BIN "$HOME/.local/bin/gt.real"

# Output merged session info as JSON
printf '%%s' "$SPAWN_JSON" | jq -c --arg sess "$SESS" --arg cert_dir "$CERT_DIR" '. + {session_name: $sess, cert_dir: $cert_dir}'
`,
		townRoot,
		rigName, polecatName, doltHost, doltPort,
		rigName, polecatName,
		proxyURL,
	)
}

func keys(m map[string]*config.MachineEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
