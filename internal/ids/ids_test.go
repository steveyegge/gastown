package ids

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// AgentID.String() Tests
// =============================================================================

func TestAgentID_String_TownLevelSingleton(t *testing.T) {
	tests := []struct {
		name string
		id   AgentID
		want string
	}{
		{"mayor", AgentID{Role: "mayor"}, "mayor"},
		{"deacon", AgentID{Role: "deacon"}, "deacon"},
		{"boot", AgentID{Role: "boot"}, "boot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.id.String())
		})
	}
}

func TestAgentID_String_RigLevelSingleton(t *testing.T) {
	tests := []struct {
		name string
		id   AgentID
		want string
	}{
		{"witness", AgentID{Role: "witness", Rig: "gastown"}, "gastown/witness"},
		{"refinery", AgentID{Role: "refinery", Rig: "gastown"}, "gastown/refinery"},
		{"witness hyphenated rig", AgentID{Role: "witness", Rig: "foo-bar"}, "foo-bar/witness"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.id.String())
		})
	}
}

func TestAgentID_String_NamedWorker(t *testing.T) {
	tests := []struct {
		name string
		id   AgentID
		want string
	}{
		{"polecat", AgentID{Role: "polecat", Rig: "gastown", Worker: "Toast"}, "gastown/polecat/Toast"},
		{"crew", AgentID{Role: "crew", Rig: "gastown", Worker: "max"}, "gastown/crew/max"},
		{"hyphenated rig and worker", AgentID{Role: "polecat", Rig: "foo-bar", Worker: "joe-bob"}, "foo-bar/polecat/joe-bob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.id.String())
		})
	}
}

func TestAgentID_String_Empty(t *testing.T) {
	id := AgentID{}
	assert.Equal(t, "", id.String())
}

// =============================================================================
// AgentID.Parse() Tests
// =============================================================================

func TestAgentID_Parse_TownLevel(t *testing.T) {
	id := AgentID{Role: "mayor"}
	role, rig, worker := id.Parse()
	assert.Equal(t, "mayor", role)
	assert.Equal(t, "", rig)
	assert.Equal(t, "", worker)
}

func TestAgentID_Parse_RigLevel(t *testing.T) {
	id := AgentID{Role: "witness", Rig: "gastown"}
	role, rig, worker := id.Parse()
	assert.Equal(t, "witness", role)
	assert.Equal(t, "gastown", rig)
	assert.Equal(t, "", worker)
}

func TestAgentID_Parse_NamedWorker(t *testing.T) {
	id := AgentID{Role: "polecat", Rig: "gastown", Worker: "Toast"}
	role, rig, worker := id.Parse()
	assert.Equal(t, "polecat", role)
	assert.Equal(t, "gastown", rig)
	assert.Equal(t, "Toast", worker)
}

func TestAgentID_Parse_Empty(t *testing.T) {
	id := AgentID{}
	role, rig, worker := id.Parse()
	assert.Equal(t, "", role)
	assert.Equal(t, "", rig)
	assert.Equal(t, "", worker)
}

// =============================================================================
// Singleton Address Tests
// =============================================================================

func TestSingletonAddresses(t *testing.T) {
	assert.Equal(t, AgentID{Role: "mayor"}, MayorAddress)
	assert.Equal(t, AgentID{Role: "deacon"}, DeaconAddress)
	assert.Equal(t, AgentID{Role: "boot"}, BootAddress)
}

// =============================================================================
// Address Constructor Tests
// =============================================================================

func TestWitnessAddress(t *testing.T) {
	id := WitnessAddress("gastown")
	assert.Equal(t, "witness", id.Role)
	assert.Equal(t, "gastown", id.Rig)
	assert.Equal(t, "", id.Worker)
}

func TestRefineryAddress(t *testing.T) {
	id := RefineryAddress("gastown")
	assert.Equal(t, "refinery", id.Role)
	assert.Equal(t, "gastown", id.Rig)
	assert.Equal(t, "", id.Worker)
}

func TestPolecatAddress(t *testing.T) {
	id := PolecatAddress("gastown", "Toast")
	assert.Equal(t, "polecat", id.Role)
	assert.Equal(t, "gastown", id.Rig)
	assert.Equal(t, "Toast", id.Worker)
}

func TestCrewAddress(t *testing.T) {
	id := CrewAddress("gastown", "max")
	assert.Equal(t, "crew", id.Role)
	assert.Equal(t, "gastown", id.Rig)
	assert.Equal(t, "max", id.Worker)
}

// =============================================================================
// ParseAddress() Tests
// =============================================================================

func TestParseAddress_TownLevelSingleton(t *testing.T) {
	tests := []struct {
		addr string
		want AgentID
	}{
		{"mayor", AgentID{Role: "mayor"}},
		{"deacon", AgentID{Role: "deacon"}},
		{"boot", AgentID{Role: "boot"}},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			got := ParseAddress(tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAddress_RigLevelSingleton(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want AgentID
	}{
		{"witness", "gastown/witness", AgentID{Role: "witness", Rig: "gastown"}},
		{"refinery", "gastown/refinery", AgentID{Role: "refinery", Rig: "gastown"}},
		{"hyphenated rig", "foo-bar/witness", AgentID{Role: "witness", Rig: "foo-bar"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAddress(tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAddress_NamedWorker(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want AgentID
	}{
		{"polecat", "gastown/polecat/Toast", AgentID{Role: "polecat", Rig: "gastown", Worker: "Toast"}},
		{"crew", "gastown/crew/max", AgentID{Role: "crew", Rig: "gastown", Worker: "max"}},
		{"hyphenated", "foo-bar/polecat/joe-bob", AgentID{Role: "polecat", Rig: "foo-bar", Worker: "joe-bob"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAddress(tt.addr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseAddress_Invalid(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{"empty", ""},
		{"too many parts", "a/b/c/d"},
		{"too many parts 5", "a/b/c/d/e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAddress(tt.addr)
			// Invalid addresses return empty AgentID (or partial for some cases)
			if tt.addr == "" {
				assert.Equal(t, AgentID{Role: ""}, got)
			} else {
				assert.Equal(t, AgentID{}, got)
			}
		})
	}
}

func TestParseAddress_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		id   AgentID
	}{
		{"mayor", MayorAddress},
		{"deacon", DeaconAddress},
		{"boot", BootAddress},
		{"witness", WitnessAddress("gastown")},
		{"refinery", RefineryAddress("myproject")},
		{"polecat", PolecatAddress("gastown", "Toast")},
		{"crew", CrewAddress("gastown", "max")},
		{"hyphenated rig", WitnessAddress("foo-bar")},
		{"hyphenated worker", PolecatAddress("gastown", "joe-bob")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := tt.id.String()
			parsed := ParseAddress(addr)
			assert.Equal(t, tt.id, parsed, "round-trip failed for %q", addr)
		})
	}
}

// =============================================================================
// ParseSessionName() Tests
// =============================================================================

func TestParseSessionName_TownLevelSingleton(t *testing.T) {
	tests := []struct {
		name    string
		session string
		want    AgentID
	}{
		{"gt-mayor", "gt-mayor", AgentID{Role: "mayor"}},
		{"gt-deacon", "gt-deacon", AgentID{Role: "deacon"}},
		{"gt-boot", "gt-boot", AgentID{Role: "boot"}},
		{"hq-mayor", "hq-mayor", AgentID{Role: "mayor"}},
		{"hq-deacon", "hq-deacon", AgentID{Role: "deacon"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSessionName_RigLevelSingleton(t *testing.T) {
	tests := []struct {
		name    string
		session string
		want    AgentID
	}{
		{"witness", "gt-gastown-witness", AgentID{Role: "witness", Rig: "gastown"}},
		{"refinery", "gt-gastown-refinery", AgentID{Role: "refinery", Rig: "gastown"}},
		// Note: hyphenated rigs are tested in KnownAmbiguity tests - they don't work correctly
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSessionName_ExplicitPolecat(t *testing.T) {
	tests := []struct {
		name    string
		session string
		want    AgentID
	}{
		{"simple", "gt-gastown-polecat-Toast", AgentID{Role: "polecat", Rig: "gastown", Worker: "Toast"}},
		{"hyphenated worker", "gt-gastown-polecat-joe-bob", AgentID{Role: "polecat", Rig: "gastown", Worker: "joe-bob"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSessionName_Crew(t *testing.T) {
	tests := []struct {
		name    string
		session string
		want    AgentID
	}{
		{"simple", "gt-gastown-crew-max", AgentID{Role: "crew", Rig: "gastown", Worker: "max"}},
		{"hyphenated worker", "gt-gastown-crew-joe-bob", AgentID{Role: "crew", Rig: "gastown", Worker: "joe-bob"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSessionName_LegacyPolecat(t *testing.T) {
	// Legacy format: gt-rig-name (no explicit "polecat" keyword)
	tests := []struct {
		name    string
		session string
		want    AgentID
	}{
		{"simple", "gt-gastown-Toast", AgentID{Role: "polecat", Rig: "gastown", Worker: "Toast"}},
		{"hyphenated name", "gt-gastown-joe-bob", AgentID{Role: "polecat", Rig: "gastown", Worker: "joe-bob"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSessionName_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		session string
	}{
		{"no prefix", "gastown-witness"},
		{"wrong prefix", "foo-gastown-witness"},
		{"empty after gt", "gt-"},
		{"just prefix", "gt"},
		{"empty", ""},
		{"unknown singleton", "gt-unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, AgentID{}, got, "expected empty AgentID for %q", tt.session)
		})
	}
}

func TestParseSessionName_InvalidRigLevel(t *testing.T) {
	tests := []struct {
		name    string
		session string
	}{
		{"witness with extra parts", "gt-gastown-witness-extra"},
		{"refinery with extra parts", "gt-gastown-refinery-extra"},
		{"polecat missing name", "gt-gastown-polecat"},
		{"crew missing name", "gt-gastown-crew"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.session)
			assert.Equal(t, AgentID{}, got, "expected empty AgentID for %q", tt.session)
		})
	}
}

// =============================================================================
// Known Limitations / Ambiguity Tests
// =============================================================================

func TestParseSessionName_KnownAmbiguity_HyphenatedRig(t *testing.T) {
	// Known limitation: hyphenated rig names cause parsing ambiguity
	// gt-foo-bar-witness could be:
	// - rig="foo-bar", role="witness" (intended)
	// - rig="foo", role="bar", and "witness" is legacy polecat name (what happens)
	//
	// The current parser treats the second segment as the role, so hyphenated
	// rig names are parsed incorrectly for rig-level singletons.

	session := "gt-foo-bar-witness"
	got := ParseSessionName(session)

	// Current behavior: treats "bar" as the role. Since "bar" is not a known role
	// (witness, refinery, polecat, crew), it falls through to legacy polecat parsing.
	// The worker name becomes "bar-witness" (everything after rig).
	// This documents the limitation - NOT necessarily what we want.
	assert.Equal(t, "polecat", got.Role, "parsed as legacy polecat due to unknown role")
	assert.Equal(t, "foo", got.Rig, "only first segment treated as rig")
	assert.Equal(t, "bar-witness", got.Worker, "rest becomes worker name")
}

func TestParseSessionName_KnownAmbiguity_PolecatNamedWitness(t *testing.T) {
	// Known limitation: a polecat named "witness" is indistinguishable from
	// the witness singleton.
	//
	// gt-gastown-witness could be:
	// - rig="gastown", role="witness" (rig-level singleton)
	// - rig="gastown", role="polecat", worker="witness" (named worker)
	//
	// Parser prioritizes rig-level singletons.

	session := "gt-gastown-witness"
	got := ParseSessionName(session)

	assert.Equal(t, "witness", got.Role, "parses as witness singleton")
	assert.Equal(t, "gastown", got.Rig)
	assert.Equal(t, "", got.Worker, "no worker - treated as singleton")
}

func TestParseSessionName_KnownAmbiguity_PolecatNamedCrew(t *testing.T) {
	// Known limitation: a polecat named "crew" followed by a name looks like a crew member.
	//
	// gt-gastown-crew-max could be:
	// - rig="gastown", role="crew", worker="max" (crew member)
	// - rig="gastown", role="polecat", worker="crew-max" (legacy polecat with hyphenated name)
	//
	// Parser prioritizes explicit "crew" keyword.

	session := "gt-gastown-crew-max"
	got := ParseSessionName(session)

	assert.Equal(t, "crew", got.Role, "parses as crew, not polecat")
	assert.Equal(t, "gastown", got.Rig)
	assert.Equal(t, "max", got.Worker)
}
