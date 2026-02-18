package mail

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/session"
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
		{"overseer", true},
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
	// Set up prefix registry for test
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("bd", "beads")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	defer session.SetDefaultRegistry(old)

	tests := []struct {
		address string
		want    []string
	}{
		// Overseer (human operator) - single session
		{"overseer", []string{"hq-overseer"}},

		// Town-level addresses - single session
		{"mayor", []string{"hq-mayor"}},
		{"mayor/", []string{"hq-mayor"}},
		{"deacon", []string{"hq-deacon"}},

		// Rig singletons - single session (no crew/polecat ambiguity)
		{"gastown/refinery", []string{"gt-refinery"}},
		{"beads/witness", []string{"bd-witness"}},

		// Ambiguous addresses - try both crew and polecat variants
		{"gastown/Toast", []string{"gt-crew-Toast", "gt-Toast"}},
		{"beads/ruby", []string{"bd-crew-ruby", "bd-ruby"}},

		// Explicit crew/polecat - single session
		{"gastown/crew/max", []string{"gt-crew-max"}},
		{"gastown/polecats/nux", []string{"gt-nux"}},

		// Invalid addresses - empty result
		{"gastown/", nil},  // Empty target
		{"gastown", nil},   // No slash
		{"", nil},          // Empty address
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := AddressToSessionIDs(tt.address)
			if len(got) != len(tt.want) {
				t.Errorf("AddressToSessionIDs(%q) = %v, want %v", tt.address, got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("AddressToSessionIDs(%q)[%d] = %q, want %q", tt.address, i, v, tt.want[i])
				}
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
		{"gastown/crew/max", "gastown/max", true},
		{"gastown/max", "gastown/crew/max", true},
		{"gastown/polecats/Toast", "gastown/Toast", true},
		{"gastown/Toast", "gastown/polecats/Toast", true},
		{"gastown/crew/max", "gastown/polecats/max", true},
		{"mayor/", "deacon/", false},
		{"gastown/Toast", "gastown/Nux", false},
		{"gastown/crew/max", "gastown/crew/nux", false},
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
			name: "non-gt prefix with description",
			bead: &agentBead{
				ID:          "bd-beads-crew-beavis",
				Description: "Crew worker beavis in beads.\n\nrole_type: crew\nrig: beads\nagent_state: idle",
			},
			want: "beads/beavis",
		},
		{
			name: "non-gt prefix singleton with description",
			bead: &agentBead{
				ID:          "bd-beads-witness",
				Description: "Witness for beads.\n\nrole_type: witness\nrig: beads\nagent_state: idle",
			},
			want: "beads/witness",
		},
		{
			name: "non-gt prefix no description fallback crew",
			bead: &agentBead{ID: "bd-beads-crew-beavis"},
			want: "beads/beavis",
		},
		{
			name: "non-gt prefix no description fallback witness",
			bead: &agentBead{ID: "bd-beads-witness"},
			want: "beads/witness",
		},
		{
			name: "non-gt prefix no description fallback refinery",
			bead: &agentBead{ID: "db-debt_buying-refinery"},
			want: "debt_buying/refinery",
		},
		{
			name: "non-gt prefix no description fallback polecat",
			bead: &agentBead{ID: "ppf-pyspark_pipeline_framework-polecat-Toast"},
			want: "pyspark_pipeline_framework/Toast",
		},
		{
			name: "malformed singleton witness with name segment",
			bead: &agentBead{ID: "bd-beads-witness-extra"},
			want: "",
		},
		{
			name: "malformed singleton refinery with name segment",
			bead: &agentBead{ID: "bd-beads-refinery-extra"},
			want: "",
		},
		{
			name: "hyphenated agent name via fallback",
			bead: &agentBead{ID: "bd-beads-crew-my-agent"},
			want: "beads/my-agent",
		},
		{
			name: "empty ID",
			bead: &agentBead{ID: ""},
			want: "",
		},
		{
			name: "hq-dog with location in description",
			bead: &agentBead{
				ID:          "hq-dog-alpha",
				Description: "Dog: alpha\n\nrole_type: dog\nrig: town\nlocation: deacon/dogs/alpha",
			},
			want: "deacon/dogs/alpha",
		},
		{
			name: "hq-dog without description returns empty",
			bead: &agentBead{
				ID: "hq-dog-bravo",
			},
			want: "",
		},
		{
			name: "hq-dog with location takes priority over role_type+rig",
			bead: &agentBead{
				ID:          "hq-dog-charlie",
				Description: "Dog: charlie\n\nrole_type: dog\nrig: town\nlocation: deacon/dogs/charlie",
			},
			want: "deacon/dogs/charlie",
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

func TestParseAgentAddressFromDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want string
	}{
		{
			name: "location field returns address directly",
			desc: "Dog: alpha\n\nrole_type: dog\nrig: town\nlocation: deacon/dogs/alpha",
			want: "deacon/dogs/alpha",
		},
		{
			name: "location null falls back to role_type+rig",
			desc: "Some agent\n\nrole_type: witness\nrig: myrig\nlocation: null",
			want: "myrig/witness",
		},
		{
			name: "no location uses role_type+rig",
			desc: "Some agent\n\nrole_type: polecat\nrig: gastown",
			want: "gastown/polecat",
		},
		{
			name: "town-level agent no rig",
			desc: "Mayor\n\nrole_type: mayor\nrig: null",
			want: "mayor/",
		},
		{
			name: "empty description",
			desc: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAgentAddressFromDescription(tt.desc)
			if got != tt.want {
				t.Errorf("parseAgentAddressFromDescription(%q) = %q, want %q", tt.desc, got, tt.want)
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
	// Skip if bd CLI is not available or not functional (e.g., missing DLLs on Windows CI)
	if out, err := exec.Command("bd", "version").CombinedOutput(); err != nil {
		t.Skipf("bd CLI not functional, skipping test: %v (%s)", err, strings.TrimSpace(string(out)))
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

	// Create test agent beads using gt:agent label
	createAgent := func(id, title string) {
		cmd := exec.Command("bd", "create", title, "--labels=gt:agent", "--id="+id, "--force")
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

func setupTestRegistryForAddressTest(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("bd", "beads")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestAddressToAgentBeadID(t *testing.T) {
	setupTestRegistryForAddressTest(t)

	tests := []struct {
		name     string
		address  string
		expected string
	}{
		{
			name:     "overseer returns empty",
			address:  "overseer",
			expected: "",
		},
		{
			name:     "mayor",
			address:  "mayor/",
			expected: "hq-mayor",
		},
		{
			name:     "mayor without slash",
			address:  "mayor",
			expected: "hq-mayor",
		},
		{
			name:     "deacon",
			address:  "deacon/",
			expected: "hq-deacon",
		},
		{
			name:     "witness",
			address:  "gastown/witness",
			expected: "gt-witness",
		},
		{
			name:     "refinery",
			address:  "gastown/refinery",
			expected: "gt-refinery",
		},
		{
			name:     "crew member",
			address:  "gastown/crew/max",
			expected: "gt-crew-max",
		},
		{
			name:     "polecat (default)",
			address:  "gastown/alpha",
			expected: "gt-alpha",
		},
		{
			name:     "empty address",
			address:  "",
			expected: "",
		},
		{
			name:     "no slash non-special",
			address:  "unknown",
			expected: "",
		},
		{
			name:     "rig with empty target",
			address:  "gastown/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addressToAgentBeadID(tt.address)
			if got != tt.expected {
				t.Errorf("addressToAgentBeadID(%q) = %q, want %q", tt.address, got, tt.expected)
			}
		})
	}
}
