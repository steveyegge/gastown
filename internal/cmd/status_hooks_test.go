package cmd

import (
	"reflect"
	"sort"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
)

// TestMergeHookedByAssignee_EmptyAssigneeSkipped verifies that issues without
// an assignee are dropped (they don't belong on any agent's display row).
func TestMergeHookedByAssignee_EmptyAssigneeSkipped(t *testing.T) {
	hooked := []*beads.Issue{
		{ID: "hq-1", Assignee: ""},
		{ID: "hq-2", Assignee: "rig/witness"},
	}

	got := mergeHookedByAssignee(hooked, nil)
	if _, ok := got[""]; ok {
		t.Errorf("merged map contains entry for empty assignee")
	}
	if got["rig/witness"] == nil || got["rig/witness"].ID != "hq-2" {
		t.Errorf("expected hq-2 for rig/witness, got %+v", got["rig/witness"])
	}
}

// TestMergeHookedByAssignee_HookedBeatsInProgress verifies hooked work shadows
// in_progress work for the same assignee — the most recently-slung intent.
func TestMergeHookedByAssignee_HookedBeatsInProgress(t *testing.T) {
	hooked := []*beads.Issue{{ID: "hq-new", Assignee: "rig/refinery"}}
	inProgress := []*beads.Issue{{ID: "hq-old", Assignee: "rig/refinery"}}

	got := mergeHookedByAssignee(hooked, inProgress)
	if got["rig/refinery"].ID != "hq-new" {
		t.Errorf("got %q, want hq-new (hooked must shadow in_progress)", got["rig/refinery"].ID)
	}
}

// TestMergeHookedByAssignee_InProgressFillsWhenNoHooked verifies that
// in_progress is the fallback when an assignee has no hooked work — covers
// the "session was interrupted mid-work" case.
func TestMergeHookedByAssignee_InProgressFillsWhenNoHooked(t *testing.T) {
	hooked := []*beads.Issue{{ID: "hq-other", Assignee: "rig/witness"}}
	inProgress := []*beads.Issue{{ID: "hq-running", Assignee: "rig/refinery"}}

	got := mergeHookedByAssignee(hooked, inProgress)
	if got["rig/refinery"].ID != "hq-running" {
		t.Errorf("got %v, want hq-running for rig/refinery", got["rig/refinery"])
	}
	if got["rig/witness"].ID != "hq-other" {
		t.Errorf("got %v, want hq-other for rig/witness", got["rig/witness"])
	}
}

// TestMergeHookedByAssignee_FirstOccurrenceWins verifies that within the
// hooked list, the first issue for an assignee wins (deterministic).
func TestMergeHookedByAssignee_FirstOccurrenceWins(t *testing.T) {
	hooked := []*beads.Issue{
		{ID: "hq-first", Assignee: "rig/polecats/Toast"},
		{ID: "hq-second", Assignee: "rig/polecats/Toast"},
	}

	got := mergeHookedByAssignee(hooked, nil)
	if got["rig/polecats/Toast"].ID != "hq-first" {
		t.Errorf("got %q, want hq-first (first occurrence)", got["rig/polecats/Toast"].ID)
	}
}

// TestMergeHookedByAssignee_NilIssuesSkipped covers defensive nil-skip.
func TestMergeHookedByAssignee_NilIssuesSkipped(t *testing.T) {
	hooked := []*beads.Issue{nil, {ID: "hq-1", Assignee: "rig/witness"}, nil}
	got := mergeHookedByAssignee(hooked, []*beads.Issue{nil})
	if len(got) != 1 || got["rig/witness"].ID != "hq-1" {
		t.Errorf("unexpected merge result: %+v", got)
	}
}

// TestResolveRigHooks_PolecatsAndCrew verifies polecat and crew agents are
// emitted with their address-formatted assignees and matched against the
// rig map.
func TestResolveRigHooks_PolecatsAndCrew(t *testing.T) {
	r := &rig.Rig{Name: "myrig", Polecats: []string{"Toast", "Pickle"}}
	crews := []string{"alice"}
	rigMap := map[string]*beads.Issue{
		"myrig/Toast":      {ID: "mr-1", Title: "polecat work"},
		"myrig/crew/alice": {ID: "mr-2", Title: "crew work"},
	}

	hooks := resolveRigHooks(r, crews, rigMap, nil)

	got := indexByAgent(hooks)
	if !got["myrig/Toast"].HasWork || got["myrig/Toast"].Molecule != "mr-1" {
		t.Errorf("Toast hook missing/wrong: %+v", got["myrig/Toast"])
	}
	if got["myrig/Pickle"].HasWork {
		t.Errorf("Pickle should be empty: %+v", got["myrig/Pickle"])
	}
	if !got["myrig/crew/alice"].HasWork || got["myrig/crew/alice"].Molecule != "mr-2" {
		t.Errorf("alice hook missing/wrong: %+v", got["myrig/crew/alice"])
	}
}

// TestResolveRigHooks_RigShadowsTown verifies a rig-local hook for an agent
// shadows a town-level hook for the same address.
func TestResolveRigHooks_RigShadowsTown(t *testing.T) {
	r := &rig.Rig{Name: "myrig", HasWitness: true}
	rigMap := map[string]*beads.Issue{
		"myrig/witness": {ID: "mr-rig", Title: "rig wisp"},
	}
	townMap := map[string]*beads.Issue{
		"myrig/witness": {ID: "hq-town", Title: "town wisp"},
	}

	hooks := resolveRigHooks(r, nil, rigMap, townMap)

	got := indexByAgent(hooks)
	if got["myrig/witness"].Molecule != "mr-rig" {
		t.Errorf("got %q, want mr-rig (rig must shadow town)", got["myrig/witness"].Molecule)
	}
}

// TestResolveRigHooks_TownFallback verifies town-level hooks surface for
// rig agents when there's no rig-local match — covers Mayor→polecat slings
// where the work bead lives in town beads.
func TestResolveRigHooks_TownFallback(t *testing.T) {
	r := &rig.Rig{Name: "myrig", HasRefinery: true}
	townMap := map[string]*beads.Issue{
		"myrig/refinery": {ID: "hq-wisp-aaa", Title: "patrol"},
	}

	hooks := resolveRigHooks(r, nil, nil, townMap)

	got := indexByAgent(hooks)
	if !got["myrig/refinery"].HasWork || got["myrig/refinery"].Molecule != "hq-wisp-aaa" {
		t.Errorf("refinery should pick up town hook, got %+v", got["myrig/refinery"])
	}
}

// TestResolveRigHooks_NilTownMap verifies a nil townHookedByAssignee map is
// safe — discoverRigHooks passes nil in --fast mode.
func TestResolveRigHooks_NilTownMap(t *testing.T) {
	r := &rig.Rig{Name: "myrig", HasWitness: true}
	rigMap := map[string]*beads.Issue{
		"myrig/witness": {ID: "mr-1", Title: "patrol"},
	}

	hooks := resolveRigHooks(r, nil, rigMap, nil)

	got := indexByAgent(hooks)
	if !got["myrig/witness"].HasWork || got["myrig/witness"].Molecule != "mr-1" {
		t.Errorf("witness should resolve from rigMap with nil townMap: %+v", got["myrig/witness"])
	}
}

// TestResolveRigHooks_RoleEmittedExactlyOnce verifies the order and roles of
// emitted entries: polecats first, then crew, then witness, then refinery.
// Order matters for the `gt status` rendering loop.
func TestResolveRigHooks_RoleEmittedExactlyOnce(t *testing.T) {
	r := &rig.Rig{
		Name:        "myrig",
		Polecats:    []string{"a", "b"},
		HasWitness:  true,
		HasRefinery: true,
	}
	hooks := resolveRigHooks(r, []string{"crew1"}, nil, nil)

	gotAddrs := make([]string, 0, len(hooks))
	gotRoles := make([]string, 0, len(hooks))
	for _, h := range hooks {
		gotAddrs = append(gotAddrs, h.Agent)
		gotRoles = append(gotRoles, h.Role)
	}

	wantAddrs := []string{"myrig/a", "myrig/b", "myrig/crew/crew1", "myrig/witness", "myrig/refinery"}
	wantRoles := []string{
		constants.RolePolecat,
		constants.RolePolecat,
		constants.RoleCrew,
		constants.RoleWitness,
		constants.RoleRefinery,
	}

	if !reflect.DeepEqual(gotAddrs, wantAddrs) {
		t.Errorf("addresses = %v, want %v", gotAddrs, wantAddrs)
	}
	if !reflect.DeepEqual(gotRoles, wantRoles) {
		t.Errorf("roles = %v, want %v", gotRoles, wantRoles)
	}
}

// TestResolveRigHooks_NoAgentsNoHooks verifies an empty rig produces no
// AgentHookInfo entries — the rendering loop relies on len(hooks)==0 to
// skip the section entirely.
func TestResolveRigHooks_NoAgentsNoHooks(t *testing.T) {
	r := &rig.Rig{Name: "empty"}
	hooks := resolveRigHooks(r, nil, nil, nil)
	if len(hooks) != 0 {
		t.Errorf("expected no hooks for empty rig, got %d: %+v", len(hooks), hooks)
	}
}

// indexByAgent maps an AgentHookInfo slice by Agent for easier assertions.
func indexByAgent(hooks []AgentHookInfo) map[string]AgentHookInfo {
	m := make(map[string]AgentHookInfo, len(hooks))
	for _, h := range hooks {
		m[h.Agent] = h
	}
	return m
}

// keys returns the sorted keys of a map[string]X for deterministic assertion.
func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var _ = keys[int] // keep generic helper available
