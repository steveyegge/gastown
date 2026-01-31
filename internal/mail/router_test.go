package mail

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectTownRoot(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	mayorDir := filepath.Join(townRoot, "mayor")
	rigDir := filepath.Join(townRoot, "gastown", "polecats", "test")

	// Create mayor/town.json marker
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		startDir string
		want     string
	}{
		{
			name:     "from town root",
			startDir: townRoot,
			want:     townRoot,
		},
		{
			name:     "from rig subdirectory",
			startDir: rigDir,
			want:     townRoot,
		},
		{
			name:     "from mayor directory",
			startDir: mayorDir,
			want:     townRoot,
		},
		{
			name:     "from non-town directory",
			startDir: tmpDir,
			want:     "", // No town.json marker above tmpDir
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectTownRoot(tt.startDir)
			if got != tt.want {
				t.Errorf("detectTownRoot(%q) = %q, want %q", tt.startDir, got, tt.want)
			}
		})
	}
}

func TestIsTownLevelAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"mayor", true},
		{"mayor/", true},
		{"deacon", true},
		{"deacon/", true},
		{"gastown/refinery", false},
		{"gastown/polecats/Toast", false},
		{"gastown/", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isTownLevelAddress(tt.address)
			if got != tt.want {
				t.Errorf("isTownLevelAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestAddressToSessionIDs(t *testing.T) {
	tests := []struct {
		address string
		want    []string
	}{
		// Town-level addresses - single session
		{"mayor", []string{"hq-mayor"}},
		{"mayor/", []string{"hq-mayor"}},
		{"deacon", []string{"hq-deacon"}},

		// Rig singletons - single session (no crew/polecat ambiguity)
		{"gastown/refinery", []string{"gt-gastown-refinery"}},
		{"beads/witness", []string{"gt-beads-witness"}},

		// Ambiguous addresses - try both crew and polecat variants
		{"gastown/Toast", []string{"gt-gastown-crew-Toast", "gt-gastown-Toast"}},
		{"beads/ruby", []string{"gt-beads-crew-ruby", "gt-beads-ruby"}},

		// Explicit crew/polecat - single session
		{"gastown/crew/max", []string{"gt-gastown-crew-max"}},
		{"gastown/polecats/nux", []string{"gt-gastown-polecats-nux"}},

		// Invalid addresses - empty result
		{"gastown/", nil},  // Empty target
		{"gastown", nil},   // No slash
		{"", nil},          // Empty address
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := addressToSessionIDs(tt.address)
			if len(got) != len(tt.want) {
				t.Errorf("addressToSessionIDs(%q) = %v, want %v", tt.address, got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("addressToSessionIDs(%q)[%d] = %q, want %q", tt.address, i, v, tt.want[i])
				}
			}
		})
	}

	// Town-namespaced tests: when town is non-empty, mayor/deacon get namespaced session names
	townTests := []struct {
		address string
		town    string
		want    []string
	}{
		{"mayor", "gt11", []string{"hq-gt11-mayor"}},
		{"mayor/", "gt11", []string{"hq-gt11-mayor"}},
		{"deacon", "gt11", []string{"hq-gt11-deacon"}},
		// Rig-level addresses are unaffected by town
		{"gastown/crew/max", "gt11", []string{"gt-gastown-crew-max"}},
		{"gastown/Toast", "gt11", []string{"gt-gastown-crew-Toast", "gt-gastown-Toast"}},
	}

	// TODO(gt-ir8f.2): townTests disabled - addressToSessionIDs signature changed from 2 args to 1.
	// These tests need updating once the function supports town-namespaced session names again.
	_ = townTests
}

func TestAddressToSessionID(t *testing.T) {
	// Deprecated wrapper - returns first candidate from addressToSessionIDs
	tests := []struct {
		address string
		want    string
	}{
		{"mayor", "hq-mayor"},
		{"mayor/", "hq-mayor"},
		{"deacon", "hq-deacon"},
		{"gastown/refinery", "gt-gastown-refinery"},
		{"gastown/Toast", "gt-gastown-crew-Toast"}, // First candidate is crew
		{"beads/witness", "gt-beads-witness"},
		{"gastown/", ""},   // Empty target
		{"gastown", ""},    // No slash
		{"", ""},           // Empty address
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := addressToSessionID(tt.address)
			if got != tt.want {
				t.Errorf("addressToSessionID(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestIsSelfMail(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		{"mayor/", "mayor/", true},
		{"mayor", "mayor/", true},
		{"mayor/", "mayor", true},
		{"gastown/Toast", "gastown/Toast", true},
		{"gastown/Toast/", "gastown/Toast", true},
		{"mayor/", "deacon/", false},
		{"gastown/Toast", "gastown/Nux", false},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			got := isSelfMail(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("isSelfMail(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestShouldBeWisp(t *testing.T) {
	r := &Router{}

	tests := []struct {
		name    string
		msg     *Message
		want    bool
	}{
		{
			name: "explicit wisp flag",
			msg:  &Message{Subject: "Regular message", Wisp: true},
			want: true,
		},
		{
			name: "POLECAT_STARTED subject",
			msg:  &Message{Subject: "POLECAT_STARTED: Toast"},
			want: true,
		},
		{
			name: "polecat_done subject (lowercase)",
			msg:  &Message{Subject: "polecat_done: work complete"},
			want: true,
		},
		{
			name: "NUDGE subject",
			msg:  &Message{Subject: "NUDGE: check your hook"},
			want: true,
		},
		{
			name: "START_WORK subject",
			msg:  &Message{Subject: "START_WORK: gt-123"},
			want: true,
		},
		{
			name: "regular message",
			msg:  &Message{Subject: "Please review this PR"},
			want: false,
		},
		{
			name: "handoff message (not auto-wisp)",
			msg:  &Message{Subject: "HANDOFF: context notes"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.shouldBeWisp(tt.msg)
			if got != tt.want {
				t.Errorf("shouldBeWisp(%v) = %v, want %v", tt.msg.Subject, got, tt.want)
			}
		})
	}
}

func TestResolveBeadsDir(t *testing.T) {
	// With town root set
	r := NewRouterWithTownRoot("/work/dir", "/home/user/gt")
	got := r.resolveBeadsDir("gastown/Toast")
	want := "/home/user/gt/.beads"
	if filepath.ToSlash(got) != want {
		t.Errorf("resolveBeadsDir with townRoot = %q, want %q", got, want)
	}

	// Without town root (fallback to workDir)
	r2 := &Router{workDir: "/work/dir", townRoot: ""}
	got2 := r2.resolveBeadsDir("mayor/")
	want2 := "/work/dir/.beads"
	if filepath.ToSlash(got2) != want2 {
		t.Errorf("resolveBeadsDir without townRoot = %q, want %q", got2, want2)
	}
}

func TestNewRouterWithTownRoot(t *testing.T) {
	r := NewRouterWithTownRoot("/work/rig", "/home/gt")
	if filepath.ToSlash(r.workDir) != "/work/rig" {
		t.Errorf("workDir = %q, want '/work/rig'", r.workDir)
	}
	if filepath.ToSlash(r.townRoot) != "/home/gt" {
		t.Errorf("townRoot = %q, want '/home/gt'", r.townRoot)
	}
}

// ============ Mailing List Tests ============

func TestIsListAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"list:oncall", true},
		{"list:cleanup/gastown", true},
		{"list:", true}, // Edge case: empty list name (will fail on expand)
		{"mayor/", false},
		{"gastown/witness", false},
		{"listoncall", false}, // Missing colon
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isListAddress(tt.address)
			if got != tt.want {
				t.Errorf("isListAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestParseListName(t *testing.T) {
	tests := []struct {
		address string
		want    string
	}{
		{"list:oncall", "oncall"},
		{"list:cleanup/gastown", "cleanup/gastown"},
		{"list:", ""},
		{"list:alerts", "alerts"},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := parseListName(tt.address)
			if got != tt.want {
				t.Errorf("parseListName(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestIsQueueAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"queue:work", true},
		{"queue:gastown/polecats", true},
		{"queue:", true}, // Edge case: empty queue name (will fail on expand)
		{"mayor/", false},
		{"gastown/witness", false},
		{"queuework", false}, // Missing colon
		{"list:oncall", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isQueueAddress(tt.address)
			if got != tt.want {
				t.Errorf("isQueueAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestParseQueueName(t *testing.T) {
	tests := []struct {
		address string
		want    string
	}{
		{"queue:work", "work"},
		{"queue:gastown/polecats", "gastown/polecats"},
		{"queue:", ""},
		{"queue:priority-high", "priority-high"},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := parseQueueName(tt.address)
			if got != tt.want {
				t.Errorf("parseQueueName(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

func TestExpandList(t *testing.T) {
	// Create temp directory with messaging config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write messaging.json with test lists
	configContent := `{
  "type": "messaging",
  "version": 1,
  "lists": {
    "oncall": ["mayor/", "gastown/witness"],
    "cleanup/gastown": ["gastown/witness", "deacon/"]
  }
}`
	if err := os.WriteFile(filepath.Join(configDir, "messaging.json"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRouterWithTownRoot(tmpDir, tmpDir)

	tests := []struct {
		name      string
		listName  string
		want      []string
		wantErr   bool
		errString string
	}{
		{
			name:     "oncall list",
			listName: "oncall",
			want:     []string{"mayor/", "gastown/witness"},
		},
		{
			name:     "cleanup/gastown list",
			listName: "cleanup/gastown",
			want:     []string{"gastown/witness", "deacon/"},
		},
		{
			name:      "unknown list",
			listName:  "nonexistent",
			wantErr:   true,
			errString: "unknown mailing list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.expandList(tt.listName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandList(%q) expected error, got nil", tt.listName)
				} else if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expandList(%q) error = %v, want containing %q", tt.listName, err, tt.errString)
				}
				return
			}
			if err != nil {
				t.Errorf("expandList(%q) unexpected error: %v", tt.listName, err)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("expandList(%q) = %v, want %v", tt.listName, got, tt.want)
				return
			}
			for i, addr := range got {
				if addr != tt.want[i] {
					t.Errorf("expandList(%q)[%d] = %q, want %q", tt.listName, i, addr, tt.want[i])
				}
			}
		})
	}
}

func TestExpandListNoTownRoot(t *testing.T) {
	r := &Router{workDir: "/tmp", townRoot: ""}
	_, err := r.expandList("oncall")
	if err == nil {
		t.Error("expandList with no townRoot should error")
	}
	if !contains(err.Error(), "no town root") {
		t.Errorf("expandList error = %v, want containing 'no town root'", err)
	}
}

func TestExpandQueue(t *testing.T) {
	// Create temp directory with messaging config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write messaging.json with test queues
	configContent := `{
  "type": "messaging",
  "version": 1,
  "queues": {
    "work/gastown": {"workers": ["gastown/polecats/*"], "max_claims": 3},
    "priority-high": {"workers": ["mayor/", "gastown/witness"]}
  }
}`
	if err := os.WriteFile(filepath.Join(configDir, "messaging.json"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRouterWithTownRoot(tmpDir, tmpDir)

	tests := []struct {
		name        string
		queueName   string
		wantWorkers []string
		wantMax     int
		wantErr     bool
		errString   string
	}{
		{
			name:        "work/gastown queue",
			queueName:   "work/gastown",
			wantWorkers: []string{"gastown/polecats/*"},
			wantMax:     3,
		},
		{
			name:        "priority-high queue",
			queueName:   "priority-high",
			wantWorkers: []string{"mayor/", "gastown/witness"},
			wantMax:     0, // Not specified, defaults to 0
		},
		{
			name:      "unknown queue",
			queueName: "nonexistent",
			wantErr:   true,
			errString: "unknown queue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.expandQueue(tt.queueName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandQueue(%q) expected error, got nil", tt.queueName)
				} else if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expandQueue(%q) error = %v, want containing %q", tt.queueName, err, tt.errString)
				}
				return
			}
			if err != nil {
				t.Errorf("expandQueue(%q) unexpected error: %v", tt.queueName, err)
				return
			}
			if len(got.Workers) != len(tt.wantWorkers) {
				t.Errorf("expandQueue(%q).Workers = %v, want %v", tt.queueName, got.Workers, tt.wantWorkers)
				return
			}
			for i, worker := range got.Workers {
				if worker != tt.wantWorkers[i] {
					t.Errorf("expandQueue(%q).Workers[%d] = %q, want %q", tt.queueName, i, worker, tt.wantWorkers[i])
				}
			}
			if got.MaxClaims != tt.wantMax {
				t.Errorf("expandQueue(%q).MaxClaims = %d, want %d", tt.queueName, got.MaxClaims, tt.wantMax)
			}
		})
	}
}

func TestExpandQueueNoTownRoot(t *testing.T) {
	r := &Router{workDir: "/tmp", townRoot: ""}
	_, err := r.expandQueue("work")
	if err == nil {
		t.Error("expandQueue with no townRoot should error")
	}
	if !contains(err.Error(), "no town root") {
		t.Errorf("expandQueue error = %v, want containing 'no town root'", err)
	}
}

// ============ Announce Address Tests ============

func TestIsAnnounceAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"announce:bulletin", true},
		{"announce:gastown/updates", true},
		{"announce:", true}, // Edge case: empty announce name (will fail on expand)
		{"mayor/", false},
		{"gastown/witness", false},
		{"announcebulletin", false}, // Missing colon
		{"list:oncall", false},
		{"queue:work", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isAnnounceAddress(tt.address)
			if got != tt.want {
				t.Errorf("isAnnounceAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestParseAnnounceName(t *testing.T) {
	tests := []struct {
		address string
		want    string
	}{
		{"announce:bulletin", "bulletin"},
		{"announce:gastown/updates", "gastown/updates"},
		{"announce:", ""},
		{"announce:priority-alerts", "priority-alerts"},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := parseAnnounceName(tt.address)
			if got != tt.want {
				t.Errorf("parseAnnounceName(%q) = %q, want %q", tt.address, got, tt.want)
			}
		})
	}
}

// contains checks if s contains substr (helper for error checking)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============ @group Address Tests ============

func TestIsGroupAddress(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{"@rig/gastown", true},
		{"@town", true},
		{"@witnesses", true},
		{"@crew/gastown", true},
		{"@dogs", true},
		{"@overseer", true},
		{"@polecats/gastown", true},
		{"mayor/", false},
		{"gastown/Toast", false},
		{"", false},
		{"rig/gastown", false}, // Missing @
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := isGroupAddress(tt.address)
			if got != tt.want {
				t.Errorf("isGroupAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestParseGroupAddress(t *testing.T) {
	tests := []struct {
		address      string
		wantType     GroupType
		wantRoleType string
		wantRig      string
		wantNil      bool
	}{
		// Special patterns
		{"@overseer", GroupTypeOverseer, "", "", false},
		{"@town", GroupTypeTown, "", "", false},

		// Role-based patterns (all agents of a role type)
		{"@witnesses", GroupTypeRole, "witness", "", false},
		{"@dogs", GroupTypeRole, "dog", "", false},
		{"@refineries", GroupTypeRole, "refinery", "", false},
		{"@deacons", GroupTypeRole, "deacon", "", false},

		// Rig pattern (all agents in a rig)
		{"@rig/gastown", GroupTypeRig, "", "gastown", false},
		{"@rig/beads", GroupTypeRig, "", "beads", false},

		// Rig+role patterns
		{"@crew/gastown", GroupTypeRigRole, "crew", "gastown", false},
		{"@polecats/gastown", GroupTypeRigRole, "polecat", "gastown", false},

		// Invalid patterns
		{"mayor/", "", "", "", true},
		{"@invalid", "", "", "", true},
		{"@crew/", "", "", "", true}, // Empty rig
		{"@rig", "", "", "", true},   // Missing rig name
		{"", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := parseGroupAddress(tt.address)

			if tt.wantNil {
				if got != nil {
					t.Errorf("parseGroupAddress(%q) = %+v, want nil", tt.address, got)
				}
				return
			}

			if got == nil {
				t.Errorf("parseGroupAddress(%q) = nil, want non-nil", tt.address)
				return
			}

			if got.Type != tt.wantType {
				t.Errorf("parseGroupAddress(%q).Type = %q, want %q", tt.address, got.Type, tt.wantType)
			}
			if got.RoleType != tt.wantRoleType {
				t.Errorf("parseGroupAddress(%q).RoleType = %q, want %q", tt.address, got.RoleType, tt.wantRoleType)
			}
			if got.Rig != tt.wantRig {
				t.Errorf("parseGroupAddress(%q).Rig = %q, want %q", tt.address, got.Rig, tt.wantRig)
			}
			if got.Original != tt.address {
				t.Errorf("parseGroupAddress(%q).Original = %q, want %q", tt.address, got.Original, tt.address)
			}
		})
	}
}

func TestAgentBeadToAddress(t *testing.T) {
	tests := []struct {
		name   string
		bead   *agentBead
		want   string
	}{
		{
			name: "nil bead",
			bead: nil,
			want: "",
		},
		{
			name: "town-level mayor",
			bead: &agentBead{ID: "gt-mayor"},
			want: "mayor/",
		},
		{
			name: "town-level deacon",
			bead: &agentBead{ID: "gt-deacon"},
			want: "deacon/",
		},
		{
			name: "rig singleton witness",
			bead: &agentBead{ID: "gt-gastown-witness"},
			want: "gastown/witness",
		},
		{
			name: "rig singleton refinery",
			bead: &agentBead{ID: "gt-gastown-refinery"},
			want: "gastown/refinery",
		},
		{
			name: "rig crew worker",
			bead: &agentBead{ID: "gt-gastown-crew-max"},
			want: "gastown/max",
		},
		{
			name: "rig polecat worker",
			bead: &agentBead{ID: "gt-gastown-polecat-Toast"},
			want: "gastown/Toast",
		},
		{
			name: "rig polecat with hyphenated name",
			bead: &agentBead{ID: "gt-gastown-polecat-my-agent"},
			want: "gastown/my-agent",
		},
		{
			name: "non-gt/hq prefix (invalid)",
			bead: &agentBead{ID: "bd-gastown-witness"},
			want: "",
		},
		{
			name: "empty ID",
			bead: &agentBead{ID: ""},
			want: "",
		},
		// hq- prefixed IDs (town-level agent beads)
		{
			name: "hq mayor",
			bead: &agentBead{ID: "hq-mayor"},
			want: "mayor/",
		},
		{
			name: "hq deacon",
			bead: &agentBead{ID: "hq-deacon"},
			want: "deacon/",
		},
		{
			name: "hq rig singleton witness",
			bead: &agentBead{ID: "hq-gastown-witness"},
			want: "gastown/witness",
		},
		{
			name: "hq rig crew worker",
			bead: &agentBead{ID: "hq-gastown-crew-max"},
			want: "gastown/max",
		},
		{
			name: "hq rig polecat worker",
			bead: &agentBead{ID: "hq-gastown-polecat-Toast"},
			want: "gastown/Toast",
		},
		{
			name: "hq rig crew with hyphenated name",
			bead: &agentBead{ID: "hq-gastown-crew-decision-notify"},
			want: "gastown/decision-notify",
		},
		{
			name: "hq dog agent",
			bead: &agentBead{ID: "hq-dog-alpha"},
			want: "dog/alpha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentBeadToAddress(tt.bead)
			if got != tt.want {
				t.Errorf("agentBeadToAddress(%+v) = %q, want %q", tt.bead, got, tt.want)
			}
		})
	}
}

func TestExpandAnnounce(t *testing.T) {
	// Create temp directory with messaging config
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write messaging.json with test announces
	configContent := `{
  "type": "messaging",
  "version": 1,
  "announces": {
    "alerts": {"readers": ["@town"], "retain_count": 10},
    "status/gastown": {"readers": ["gastown/witness", "mayor/"], "retain_count": 5}
  }
}`
	if err := os.WriteFile(filepath.Join(configDir, "messaging.json"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRouterWithTownRoot(tmpDir, tmpDir)

	tests := []struct {
		name         string
		announceName string
		wantReaders  []string
		wantRetain   int
		wantErr      bool
		errString    string
	}{
		{
			name:         "alerts announce",
			announceName: "alerts",
			wantReaders:  []string{"@town"},
			wantRetain:   10,
		},
		{
			name:         "status/gastown announce",
			announceName: "status/gastown",
			wantReaders:  []string{"gastown/witness", "mayor/"},
			wantRetain:   5,
		},
		{
			name:         "unknown announce",
			announceName: "nonexistent",
			wantErr:      true,
			errString:    "unknown announce channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.expandAnnounce(tt.announceName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expandAnnounce(%q) expected error, got nil", tt.announceName)
				} else if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expandAnnounce(%q) error = %v, want containing %q", tt.announceName, err, tt.errString)
				}
				return
			}
			if err != nil {
				t.Errorf("expandAnnounce(%q) unexpected error: %v", tt.announceName, err)
				return
			}
			if len(got.Readers) != len(tt.wantReaders) {
				t.Errorf("expandAnnounce(%q).Readers = %v, want %v", tt.announceName, got.Readers, tt.wantReaders)
				return
			}
			for i, reader := range got.Readers {
				if reader != tt.wantReaders[i] {
					t.Errorf("expandAnnounce(%q).Readers[%d] = %q, want %q", tt.announceName, i, reader, tt.wantReaders[i])
				}
			}
			if got.RetainCount != tt.wantRetain {
				t.Errorf("expandAnnounce(%q).RetainCount = %d, want %d", tt.announceName, got.RetainCount, tt.wantRetain)
			}
		})
	}
}

func TestExpandAnnounceNoTownRoot(t *testing.T) {
	r := &Router{workDir: "/tmp", townRoot: ""}
	_, err := r.expandAnnounce("alerts")
	if err == nil {
		t.Error("expandAnnounce with no townRoot should error")
	}
	if !contains(err.Error(), "no town root") {
		t.Errorf("expandAnnounce error = %v, want containing 'no town root'", err)
	}
}

// ============ Recipient Validation Tests ============

func TestValidateRecipient(t *testing.T) {
	// Skip if bd CLI is not available (e.g., in CI environment)
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping test")
	}

	// Create isolated beads environment for testing
	tmpDir := t.TempDir()
	townRoot := tmpDir

	// Create .beads directory and initialize
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("creating beads dir: %v", err)
	}

	// Initialize beads database with "gt" prefix (matches agent bead IDs)
	cmd := exec.Command("bd", "init", "gt")
	cmd.Dir = townRoot
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd init failed: %v\n%s", err, out)
	}

	// Set issue prefix to "gt" (matches agent bead ID pattern)
	cmd = exec.Command("bd", "config", "set", "issue_prefix", "gt")
	cmd.Dir = townRoot
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd config set issue_prefix failed: %v\n%s", err, out)
	}

	// Register custom types (agent, message, etc.) - required before creating agents
	cmd = exec.Command("bd", "config", "set", "types.custom", "agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request")
	cmd.Dir = townRoot
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bd config set types.custom failed: %v\n%s", err, out)
	}

	// Create test agent beads
	createAgent := func(id, title string) {
		cmd := exec.Command("bd", "create", title, "--type=agent", "--id="+id, "--force")
		cmd.Dir = townRoot
		cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("creating agent %s: %v\n%s", id, err, out)
		}
	}

	// Create agents that match expected bead ID patterns
	createAgent("gt-mayor", "Mayor agent")
	createAgent("gt-deacon", "Deacon agent")
	createAgent("gt-testrig-witness", "Test witness")
	createAgent("gt-testrig-crew-alice", "Test crew alice")
	createAgent("gt-testrig-polecat-bob", "Test polecat bob")

	r := NewRouterWithTownRoot(townRoot, townRoot)

	tests := []struct {
		name     string
		identity string
		wantErr  bool
		errMsg   string
	}{
		// Overseer is always valid (human operator, no agent bead)
		{"overseer", "overseer", false, ""},

		// Town-level agents (validated against beads)
		{"mayor", "mayor/", false, ""},
		{"deacon", "deacon/", false, ""},

		// Rig-level agents (validated against beads)
		{"witness", "testrig/witness", false, ""},
		{"crew member", "testrig/alice", false, ""},
		{"polecat", "testrig/bob", false, ""},

		// Invalid addresses - should fail
		{"bare name", "ruby", true, "no agent found"},
		{"nonexistent rig agent", "testrig/nonexistent", true, "no agent found"},
		{"wrong rig", "wrongrig/alice", true, "no agent found"},
		{"misrouted town agent", "testrig/mayor", true, "no agent found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := r.validateRecipient(tt.identity)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateRecipient(%q) expected error, got nil", tt.identity)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validateRecipient(%q) error = %v, want containing %q", tt.identity, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateRecipient(%q) unexpected error: %v", tt.identity, err)
				}
			}
		})
	}
}
