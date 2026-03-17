package cmd

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/steveyegge/gastown/internal/templates"
)

// ---------------------------------------------------------------------------
// Section 14: Built-in posting content assertions (gt-3d0)
//
// Rendered output of each built-in posting must contain key behavioral strings.
// These catch template regressions and verify the posting actually constrains
// behavior.
// ---------------------------------------------------------------------------

// renderPosting loads and renders a built-in posting template, returning the
// rendered string. Fails the test on any error.
func renderPosting(t *testing.T, name string) string {
	t.Helper()
	return renderPostingForRole(t, name, "polecat")
}

// renderPostingForRole loads and renders a built-in posting template with the
// given role, returning the rendered string. Fails the test on any error.
func renderPostingForRole(t *testing.T, name, role string) string {
	t.Helper()
	result, err := templates.LoadPosting("", "", name)
	if err != nil {
		t.Fatalf("LoadPosting(%q): %v", name, err)
	}
	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse %q: %v", name, err)
	}
	data := templates.RoleData{
		Role:    role,
		RigName: "testrig",
		Polecat: "testcat",
		Posting: name,
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute %q: %v", name, err)
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// 14.1–14.6: Dispatcher key strings
// ---------------------------------------------------------------------------

func TestPostingContent_Dispatcher_WorkDistribution(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Work distribution") {
		t.Error("dispatcher posting missing 'Work distribution'")
	}
}

func TestPostingContent_Dispatcher_RouteDontHoard(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Route, don't hoard") {
		t.Error("dispatcher posting missing 'Route, don't hoard'")
	}
}

func TestPostingContent_Dispatcher_LoadBalance(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Load-balance") {
		t.Error("dispatcher posting missing 'Load-balance'")
	}
}

func TestPostingContent_Dispatcher_UnblockAggressively(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Unblock aggressively") {
		t.Error("dispatcher posting missing 'Unblock aggressively'")
	}
}

func TestPostingContent_Dispatcher_ContextIsCheap(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Context is cheap, stalls are expensive") {
		t.Error("dispatcher posting missing 'Context is cheap, stalls are expensive'")
	}
}

func TestPostingContent_Dispatcher_BdReady(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "bd ready") {
		t.Error("dispatcher posting missing 'bd ready'")
	}
}

// ---------------------------------------------------------------------------
// 14.7–14.12: Inspector key strings
// ---------------------------------------------------------------------------

func TestPostingContent_Inspector_CodeReview(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	if !strings.Contains(rendered, "Code review") {
		t.Error("inspector posting missing 'Code review'")
	}
}

func TestPostingContent_Inspector_GradeDontGatekeep(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	if !strings.Contains(rendered, "Grade, don't gatekeep") {
		t.Error("inspector posting missing 'Grade, don't gatekeep'")
	}
}

func TestPostingContent_Inspector_StructuredGrading(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	if !strings.Contains(rendered, "structured grading (A-F)") {
		t.Error("inspector posting missing 'structured grading (A-F)'")
	}
}

func TestPostingContent_Inspector_BlockOnlyCritical(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	if !strings.Contains(rendered, "Block only on CRITICAL issues") {
		t.Error("inspector posting missing 'Block only on CRITICAL issues'")
	}
}

func TestPostingContent_Inspector_ReproduceBeforeRejecting(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	if !strings.Contains(rendered, "Reproduce before rejecting") {
		t.Error("inspector posting missing 'Reproduce before rejecting'")
	}
}

func TestPostingContent_Inspector_QualityChecklist(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	if !strings.Contains(rendered, "Quality Checklist") {
		t.Error("inspector posting missing 'Quality Checklist'")
	}
}

// ---------------------------------------------------------------------------
// 14.13–14.18: Scout key strings
// ---------------------------------------------------------------------------

func TestPostingContent_Scout_BreadthBeforeDepth(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "Breadth before depth") {
		t.Error("scout posting missing 'Breadth before depth'")
	}
}

func TestPostingContent_Scout_PersistEverything(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "Persist everything") {
		t.Error("scout posting missing 'Persist everything'")
	}
}

func TestPostingContent_Scout_FindingsAreDeliverable(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "Your findings ARE the deliverable") {
		t.Error("scout posting missing 'Your findings ARE the deliverable'")
	}
}

func TestPostingContent_Scout_TimeBoxExploration(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "Time-box exploration") {
		t.Error("scout posting missing 'Time-box exploration'")
	}
}

func TestPostingContent_Scout_FileWhatYouFind(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "File what you find") {
		t.Error("scout posting missing 'File what you find'")
	}
}

func TestPostingContent_Scout_BdUpdate(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "bd update") {
		t.Error("scout posting missing 'bd update'")
	}
}

// ---------------------------------------------------------------------------
// 14.19–14.21: Dispatcher hard prohibitions
// ---------------------------------------------------------------------------

func TestPostingContent_Dispatcher_NeverWritesCode(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "NEVER write code") {
		t.Error("dispatcher posting missing hard prohibition: 'NEVER write code'")
	}
}

func TestPostingContent_Dispatcher_NeverDoesDeepWork(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "NEVER do deep work") {
		t.Error("dispatcher posting missing hard prohibition: 'NEVER do deep work'")
	}
}

func TestPostingContent_Dispatcher_OutputIsTriagedBeads(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	// The template says: "Your output is triaged and routed beads, not commits."
	if !strings.Contains(rendered, "triaged and routed beads") {
		t.Error("dispatcher posting missing 'triaged and routed beads'")
	}
}

// ---------------------------------------------------------------------------
// 14.29–14.35: Dispatcher posting selection emphasis (gt-xb6)
// ---------------------------------------------------------------------------

func TestPostingContent_Dispatcher_PostingSelectionProtocol(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Posting Selection Protocol") {
		t.Error("dispatcher posting missing 'Posting Selection Protocol' section")
	}
}

func TestPostingContent_Dispatcher_EverySlingGetsPostingDecision(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Every sling gets a posting decision") {
		t.Error("dispatcher posting missing 'Every sling gets a posting decision'")
	}
}

func TestPostingContent_Dispatcher_PostingListCommand(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "posting list --rig") {
		t.Error("dispatcher posting missing 'posting list --rig' command")
	}
}

func TestPostingContent_Dispatcher_DoNotHardcodePostingNames(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "Do NOT hardcode posting names") {
		t.Error("dispatcher posting missing 'Do NOT hardcode posting names'")
	}
}

func TestPostingContent_Dispatcher_NoPostingIsValid(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "no posting") && !strings.Contains(rendered, "Picking no posting") {
		t.Error("dispatcher posting should mention that no posting is a valid choice")
	}
}

func TestPostingContent_Dispatcher_EvaluateAgainstWorkProfile(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "work profile") {
		t.Error("dispatcher posting should mention evaluating against the bead's work profile")
	}
}

func TestPostingContent_Dispatcher_ConsciousChoice(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "conscious") {
		t.Error("dispatcher posting should emphasize conscious posting choice")
	}
}

func TestPostingContent_Dispatcher_PostingShowForUnfamiliar(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "dispatcher")
	if !strings.Contains(rendered, "posting show") {
		t.Error("dispatcher posting should mention 'posting show' for unfamiliar postings")
	}
}

// ---------------------------------------------------------------------------
// 16.17–16.20: Active polling loop crew-only scoping (gt-5wi)
//
// The active polling loop in dispatcher and inspector posting templates must
// be present for crew role and absent for polecat role.
// ---------------------------------------------------------------------------

func TestPostingContent_16_17_CrewDispatcherContainsPollingLoop(t *testing.T) {
	t.Parallel()
	rendered := renderPostingForRole(t, "dispatcher", "crew")
	if !strings.Contains(rendered, "Primary Work Loop") {
		t.Error("crew dispatcher posting missing 'Primary Work Loop' section")
	}
	if !strings.Contains(rendered, "actively polling for available work") {
		t.Error("crew dispatcher posting missing 'actively polling for available work'")
	}
}

func TestPostingContent_16_18_PolecatDispatcherOmitsPollingLoop(t *testing.T) {
	t.Parallel()
	rendered := renderPostingForRole(t, "dispatcher", "polecat")
	if strings.Contains(rendered, "Primary Work Loop") {
		t.Error("polecat dispatcher posting should NOT contain 'Primary Work Loop' section")
	}
	if strings.Contains(rendered, "actively polling for available work") {
		t.Error("polecat dispatcher posting should NOT contain 'actively polling for available work'")
	}
}

func TestPostingContent_16_19_CrewInspectorContainsPollingLoop(t *testing.T) {
	t.Parallel()
	rendered := renderPostingForRole(t, "inspector", "crew")
	if !strings.Contains(rendered, "Primary Review Loop") {
		t.Error("crew inspector posting missing 'Primary Review Loop' section")
	}
	if !strings.Contains(rendered, "actively polling for work that needs review") {
		t.Error("crew inspector posting missing 'actively polling for work that needs review'")
	}
}

func TestPostingContent_16_20_PolecatInspectorOmitsPollingLoop(t *testing.T) {
	t.Parallel()
	rendered := renderPostingForRole(t, "inspector", "polecat")
	if strings.Contains(rendered, "Primary Review Loop") {
		t.Error("polecat inspector posting should NOT contain 'Primary Review Loop' section")
	}
	if strings.Contains(rendered, "actively polling for work that needs review") {
		t.Error("polecat inspector posting should NOT contain 'actively polling for work that needs review'")
	}
}

// ---------------------------------------------------------------------------
// 14.22–14.24: Inspector hard prohibitions
// ---------------------------------------------------------------------------

func TestPostingContent_Inspector_DoesNotTriage(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	// Template says: "Do not triage or dispatch."
	if !strings.Contains(rendered, "Do not triage or dispatch") {
		t.Error("inspector posting missing hard prohibition: 'Do not triage or dispatch'")
	}
}

func TestPostingContent_Inspector_MayWriteTests(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	// Template says: "You MAY write or modify tests"
	if !strings.Contains(rendered, "MAY write") {
		t.Error("inspector posting missing test exception: 'MAY write'")
	}
}

func TestPostingContent_Inspector_ReviewOnlyNoImplement(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "inspector")
	// Template says: "Review only — do not implement."
	if !strings.Contains(rendered, "Review only") {
		t.Error("inspector posting missing hard prohibition: 'Review only'")
	}
}

// ---------------------------------------------------------------------------
// 14.25–14.28: Scout hard prohibitions
// ---------------------------------------------------------------------------

func TestPostingContent_Scout_NeverWritesCode(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "NEVER write code") {
		t.Error("scout posting missing hard prohibition: 'NEVER write code'")
	}
}

func TestPostingContent_Scout_NeverPushesCommits(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "NEVER push commits") {
		t.Error("scout posting missing hard prohibition: 'NEVER push commits'")
	}
}

func TestPostingContent_Scout_NeverDispatchesWork(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	if !strings.Contains(rendered, "NEVER dispatch work") {
		t.Error("scout posting missing hard prohibition: 'NEVER dispatch work'")
	}
}

func TestPostingContent_Scout_DoesNotMakeImplementationDecisions(t *testing.T) {
	t.Parallel()
	rendered := renderPosting(t, "scout")
	// Template says: "You do NOT make\n  implementation decisions"
	// The newline + indent is preserved in raw rendering, so check both halves.
	if !strings.Contains(rendered, "do NOT make") || !strings.Contains(rendered, "implementation decisions") {
		t.Error("scout posting missing prohibition against making implementation decisions")
	}
}
