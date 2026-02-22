package capacity

import (
	"strings"
	"testing"
	"time"
)

func TestFormatMetadata_AllFields(t *testing.T) {
	m := &SchedulerMetadata{
		TargetRig:        "myrig",
		Formula:          "mol-polecat-work",
		Args:             "implement feature X",
		Vars:             "a=1\nb=2",
		EnqueuedAt:       "2026-01-15T10:00:00Z",
		Merge:            "direct",
		Convoy:           "hq-cv-test",
		BaseBranch:       "develop",
		NoMerge:          true,
		Account:          "acme",
		Agent:            "gemini",
		HookRawBead:      true,
		Owned:            true,
		Mode:             "ralph",
		DispatchFailures: 2,
		LastFailure:      "sling failed: timeout",
	}

	result := FormatMetadata(m)

	// Must start with versioned delimiter
	if !strings.HasPrefix(result, MetadataDelimiter) {
		t.Fatalf("expected result to start with delimiter, got:\n%s", result)
	}

	expected := []string{
		"target_rig: myrig",
		"formula: mol-polecat-work",
		"args: implement feature X",
		"var: a=1",
		"var: b=2",
		"enqueued_at: 2026-01-15T10:00:00Z",
		"merge: direct",
		"convoy: hq-cv-test",
		"base_branch: develop",
		"no_merge: true",
		"account: acme",
		"agent: gemini",
		"hook_raw_bead: true",
		"owned: true",
		"mode: ralph",
		"dispatch_failures: 2",
		"last_failure: sling failed: timeout",
	}
	for _, want := range expected {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in output:\n%s", want, result)
		}
	}
}

func TestFormatMetadata_MinimalFields(t *testing.T) {
	m := &SchedulerMetadata{
		TargetRig:  "prod",
		EnqueuedAt: "2026-01-15T10:00:00Z",
	}

	result := FormatMetadata(m)

	if !strings.Contains(result, "target_rig: prod") {
		t.Errorf("missing target_rig in output:\n%s", result)
	}
	if !strings.Contains(result, "enqueued_at: 2026-01-15T10:00:00Z") {
		t.Errorf("missing enqueued_at in output:\n%s", result)
	}

	// Omitted fields should not appear
	for _, absent := range []string{"formula:", "args:", "var:", "merge:", "convoy:", "base_branch:", "no_merge:", "account:", "agent:", "hook_raw_bead:", "owned:", "mode:", "dispatch_failures:", "last_failure:"} {
		if strings.Contains(result, absent) {
			t.Errorf("should not contain %q when field is empty:\n%s", absent, result)
		}
	}
}

func TestFormatMetadata_BoolFields(t *testing.T) {
	// All bools true
	m := &SchedulerMetadata{
		TargetRig:   "rig1",
		EnqueuedAt:  "2026-01-15T10:00:00Z",
		NoMerge:     true,
		HookRawBead: true,
		Owned:       true,
	}
	result := FormatMetadata(m)
	for _, want := range []string{"no_merge: true", "hook_raw_bead: true", "owned: true"} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q when bool is true:\n%s", want, result)
		}
	}

	// All bools false — should be absent
	m2 := &SchedulerMetadata{
		TargetRig:  "rig1",
		EnqueuedAt: "2026-01-15T10:00:00Z",
	}
	result2 := FormatMetadata(m2)
	for _, absent := range []string{"no_merge:", "hook_raw_bead:", "owned:"} {
		if strings.Contains(result2, absent) {
			t.Errorf("should not contain %q when bool is false:\n%s", absent, result2)
		}
	}
}

func TestParseMetadata_RoundTrip(t *testing.T) {
	original := &SchedulerMetadata{
		TargetRig:        "myrig",
		Formula:          "mol-polecat-work",
		Args:             "do the thing",
		Vars:             "x=1\ny=2",
		EnqueuedAt:       "2026-01-15T10:00:00Z",
		Merge:            "mr",
		Convoy:           "hq-cv-abc",
		BaseBranch:       "main",
		NoMerge:          true,
		Account:          "test-acct",
		Agent:            "codex",
		HookRawBead:      true,
		Owned:            true,
		Mode:             "ralph",
		DispatchFailures: 1,
		LastFailure:      "sling failed: rig not found",
	}

	formatted := FormatMetadata(original)
	parsed := ParseMetadata(formatted)

	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}

	if parsed.TargetRig != original.TargetRig {
		t.Errorf("TargetRig: got %q, want %q", parsed.TargetRig, original.TargetRig)
	}
	if parsed.Formula != original.Formula {
		t.Errorf("Formula: got %q, want %q", parsed.Formula, original.Formula)
	}
	if parsed.Args != original.Args {
		t.Errorf("Args: got %q, want %q", parsed.Args, original.Args)
	}
	if parsed.Vars != original.Vars {
		t.Errorf("Vars: got %q, want %q", parsed.Vars, original.Vars)
	}
	if parsed.EnqueuedAt != original.EnqueuedAt {
		t.Errorf("EnqueuedAt: got %q, want %q", parsed.EnqueuedAt, original.EnqueuedAt)
	}
	if parsed.Merge != original.Merge {
		t.Errorf("Merge: got %q, want %q", parsed.Merge, original.Merge)
	}
	if parsed.Convoy != original.Convoy {
		t.Errorf("Convoy: got %q, want %q", parsed.Convoy, original.Convoy)
	}
	if parsed.BaseBranch != original.BaseBranch {
		t.Errorf("BaseBranch: got %q, want %q", parsed.BaseBranch, original.BaseBranch)
	}
	if parsed.NoMerge != original.NoMerge {
		t.Errorf("NoMerge: got %v, want %v", parsed.NoMerge, original.NoMerge)
	}
	if parsed.Account != original.Account {
		t.Errorf("Account: got %q, want %q", parsed.Account, original.Account)
	}
	if parsed.Agent != original.Agent {
		t.Errorf("Agent: got %q, want %q", parsed.Agent, original.Agent)
	}
	if parsed.HookRawBead != original.HookRawBead {
		t.Errorf("HookRawBead: got %v, want %v", parsed.HookRawBead, original.HookRawBead)
	}
	if parsed.Owned != original.Owned {
		t.Errorf("Owned: got %v, want %v", parsed.Owned, original.Owned)
	}
	if parsed.Mode != original.Mode {
		t.Errorf("Mode: got %q, want %q", parsed.Mode, original.Mode)
	}
	if parsed.DispatchFailures != original.DispatchFailures {
		t.Errorf("DispatchFailures: got %d, want %d", parsed.DispatchFailures, original.DispatchFailures)
	}
	if parsed.LastFailure != original.LastFailure {
		t.Errorf("LastFailure: got %q, want %q", parsed.LastFailure, original.LastFailure)
	}
}

func TestParseMetadata_NoDelimiter(t *testing.T) {
	result := ParseMetadata("Just a regular description without scheduler metadata")
	if result != nil {
		t.Fatalf("expected nil for description without delimiter, got %+v", result)
	}
}

func TestParseMetadata_WithPreamble(t *testing.T) {
	desc := "This is a task description.\nIt has multiple lines.\n---gt:scheduler:v1---\ntarget_rig: myrig\nformula: test-formula\nenqueued_at: 2026-01-15T10:00:00Z"

	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}
	if parsed.TargetRig != "myrig" {
		t.Errorf("TargetRig: got %q, want %q", parsed.TargetRig, "myrig")
	}
	if parsed.Formula != "test-formula" {
		t.Errorf("Formula: got %q, want %q", parsed.Formula, "test-formula")
	}
}

func TestParseMetadata_LegacyDelimiter(t *testing.T) {
	// Must support the old ---gt:queue:v1--- delimiter for backward compatibility
	desc := "Task desc\n---gt:queue:v1---\ntarget_rig: legacy-rig\nformula: old-formula\nenqueued_at: 2026-01-15T10:00:00Z"

	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil for legacy delimiter")
	}
	if parsed.TargetRig != "legacy-rig" {
		t.Errorf("TargetRig: got %q, want %q", parsed.TargetRig, "legacy-rig")
	}
	if parsed.Formula != "old-formula" {
		t.Errorf("Formula: got %q, want %q", parsed.Formula, "old-formula")
	}
}

func TestParseMetadata_IgnoresUnknownKeys(t *testing.T) {
	desc := "---gt:scheduler:v1---\ntarget_rig: rig1\nfuture_field: xyz\nenqueued_at: 2026-01-15T10:00:00Z\nanother_unknown: 42"

	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}
	if parsed.TargetRig != "rig1" {
		t.Errorf("TargetRig: got %q, want %q", parsed.TargetRig, "rig1")
	}
	if parsed.EnqueuedAt != "2026-01-15T10:00:00Z" {
		t.Errorf("EnqueuedAt: got %q, want %q", parsed.EnqueuedAt, "2026-01-15T10:00:00Z")
	}
}

func TestParseMetadata_StopsAtSecondDelimiter(t *testing.T) {
	desc := "---gt:scheduler:v1---\ntarget_rig: rig1\nenqueued_at: 2026-01-15T10:00:00Z\n---gt:scheduler:v1---\ntarget_rig: should-be-ignored"

	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}
	if parsed.TargetRig != "rig1" {
		t.Errorf("TargetRig: got %q, want %q (should stop at second delimiter)", parsed.TargetRig, "rig1")
	}
}

func TestParseMetadata_CorruptedDispatchFailures(t *testing.T) {
	desc := "---gt:scheduler:v1---\ntarget_rig: rig1\ndispatch_failures: not_a_number\nenqueued_at: 2026-01-15T10:00:00Z"
	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil for corrupted failures")
	}
	if parsed.DispatchFailures != 0 {
		t.Errorf("corrupted dispatch_failures should default to 0, got %d", parsed.DispatchFailures)
	}
}

func TestParseMetadata_LegacyCommaSeparatedVars(t *testing.T) {
	desc := "---gt:queue:v1---\ntarget_rig: rig1\nvars: a=1,b=2,c=3\nenqueued_at: 2026-01-15T10:00:00Z"
	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}
	if parsed.Vars != "a=1\nb=2\nc=3" {
		t.Errorf("Vars: got %q, want %q", parsed.Vars, "a=1\nb=2\nc=3")
	}
}

func TestStripMetadata_RemovesBlock(t *testing.T) {
	preamble := "Task description here"
	desc := preamble + "\n---gt:scheduler:v1---\ntarget_rig: rig1\nenqueued_at: 2026-01-15T10:00:00Z"

	stripped := StripMetadata(desc)
	if stripped != preamble {
		t.Errorf("StripMetadata: got %q, want %q", stripped, preamble)
	}
}

func TestStripMetadata_NoMetadata(t *testing.T) {
	desc := "Just a regular description"
	stripped := StripMetadata(desc)
	if stripped != desc {
		t.Errorf("StripMetadata: got %q, want %q", stripped, desc)
	}
}

func TestStripMetadata_LegacyDelimiter(t *testing.T) {
	preamble := "Task description"
	desc := preamble + "\n---gt:queue:v1---\ntarget_rig: rig1\nenqueued_at: 2026-01-15T10:00:00Z"

	stripped := StripMetadata(desc)
	if stripped != preamble {
		t.Errorf("StripMetadata with legacy delimiter: got %q, want %q", stripped, preamble)
	}
}

func TestStripMetadata_DoubleDelimiter(t *testing.T) {
	desc := "Task desc\n---gt:scheduler:v1---\ntarget_rig: rig1\n---gt:scheduler:v1---\ntarget_rig: rig2"
	stripped := StripMetadata(desc)
	if stripped != "Task desc" {
		t.Errorf("StripMetadata with double delimiter: got %q, want %q", stripped, "Task desc")
	}
}

func TestStripMetadata_DelimiterOnly(t *testing.T) {
	desc := "---gt:scheduler:v1---\ntarget_rig: rig1\nenqueued_at: 2026-01-15T10:00:00Z"
	stripped := StripMetadata(desc)
	if stripped != "" {
		t.Errorf("StripMetadata with delimiter-only: got %q, want empty", stripped)
	}
}

func TestStripMetadata_PreservesAttachmentFields(t *testing.T) {
	// Using fresh post-dispatch description preserves attachment fields
	postDispatchDesc := "Fix login timeout\n" +
		"dispatched_by: mayor/\n" +
		"args: patch release\n" +
		"attached_molecule: gt-mol-abc\n" +
		"no_merge: true\n" +
		"mode: ralph\n" +
		"---gt:scheduler:v1---\n" +
		"target_rig: gastown\n" +
		"formula: mol-polecat-work\n" +
		"args: patch release\n" +
		"mode: ralph\n" +
		"enqueued_at: 2026-02-19T10:00:00Z"

	freshResult := StripMetadata(postDispatchDesc)
	expectedFresh := "Fix login timeout\n" +
		"dispatched_by: mayor/\n" +
		"args: patch release\n" +
		"attached_molecule: gt-mol-abc\n" +
		"no_merge: true\n" +
		"mode: ralph"
	if freshResult != expectedFresh {
		t.Errorf("fresh strip:\ngot:  %q\nwant: %q", freshResult, expectedFresh)
	}
}

func TestNewMetadata_SetsTimestamp(t *testing.T) {
	before := time.Now().UTC()
	m := NewMetadata("test-rig")
	after := time.Now().UTC()

	if m.TargetRig != "test-rig" {
		t.Errorf("TargetRig: got %q, want %q", m.TargetRig, "test-rig")
	}

	ts, err := time.Parse(time.RFC3339, m.EnqueuedAt)
	if err != nil {
		t.Fatalf("EnqueuedAt is not valid RFC3339: %q, err: %v", m.EnqueuedAt, err)
	}

	if ts.Before(before.Truncate(time.Second)) || ts.After(after.Add(time.Second)) {
		t.Errorf("EnqueuedAt %v not between %v and %v", ts, before, after)
	}
}

func TestFormatMetadata_SanitizesDelimiter(t *testing.T) {
	m := &SchedulerMetadata{
		TargetRig: "test-rig",
		Args:      "inject ---gt:scheduler:v1--- target_rig: evil-rig",
	}
	formatted := FormatMetadata(m)

	// The delimiter in args must be escaped so it doesn't corrupt parsing
	if strings.Contains(formatted, "---gt:scheduler:v1---\ntarget_rig: evil-rig") {
		t.Error("FormatMetadata did not escape delimiter in Args field")
	}

	// Parse should recover the original target_rig, not the injected one
	desc := "some description\n" + formatted
	parsed := ParseMetadata(desc)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}
	if parsed.TargetRig != "test-rig" {
		t.Errorf("TargetRig: got %q, want %q (delimiter injection succeeded)", parsed.TargetRig, "test-rig")
	}
	if !strings.Contains(parsed.Args, "inject") {
		t.Errorf("Args should contain 'inject', got %q", parsed.Args)
	}
}

func TestFormatMetadata_SanitizesNewlines(t *testing.T) {
	m := &SchedulerMetadata{
		TargetRig:   "test-rig",
		LastFailure: "error line1\ntarget_rig: evil",
	}
	formatted := FormatMetadata(m)

	// Newlines in LastFailure must be replaced to prevent key injection
	parsed := ParseMetadata(formatted)
	if parsed == nil {
		t.Fatal("ParseMetadata returned nil")
	}
	if parsed.TargetRig != "test-rig" {
		t.Errorf("TargetRig: got %q, want %q (newline injection succeeded)", parsed.TargetRig, "test-rig")
	}
}
