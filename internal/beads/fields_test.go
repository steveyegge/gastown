package beads

import (
	"strings"
	"testing"
)

// --- parseIntField (not covered in beads_test.go) ---

func TestParseIntField(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"42", 42, false},
		{"0", 0, false},
		{"-1", -1, false},
		{"abc", 0, true},
		{"", 0, true},
		{"3.14", 3, false}, // Sscanf reads the integer part
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseIntField(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIntField(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseIntField(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- AttachmentFields Mode round-trip ---

func TestAttachmentFieldsModeRoundTrip(t *testing.T) {
	original := &AttachmentFields{
		AttachedMolecule: "gt-wisp-123",
		AttachedAt:       "2026-02-18T12:00:00Z",
		Mode:             "ralph",
	}

	formatted := FormatAttachmentFields(original)
	if !strings.Contains(formatted, "mode: ralph") {
		t.Errorf("FormatAttachmentFields missing mode field, got:\n%s", formatted)
	}

	issue := &Issue{Description: formatted}
	parsed := ParseAttachmentFields(issue)
	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	if parsed.Mode != "ralph" {
		t.Errorf("Mode: got %q, want %q", parsed.Mode, "ralph")
	}
	if parsed.AttachedMolecule != "gt-wisp-123" {
		t.Errorf("AttachedMolecule: got %q, want %q", parsed.AttachedMolecule, "gt-wisp-123")
	}
}

func TestSetAttachmentFieldsPreservesMode(t *testing.T) {
	issue := &Issue{
		Description: "mode: ralph\nattached_molecule: gt-wisp-old\nSome other content",
	}
	fields := &AttachmentFields{
		AttachedMolecule: "gt-wisp-new",
		Mode:             "ralph",
	}
	newDesc := SetAttachmentFields(issue, fields)
	if !strings.Contains(newDesc, "mode: ralph") {
		t.Errorf("SetAttachmentFields lost mode field, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "attached_molecule: gt-wisp-new") {
		t.Errorf("SetAttachmentFields lost attached_molecule, got:\n%s", newDesc)
	}
	if !strings.Contains(newDesc, "Some other content") {
		t.Errorf("SetAttachmentFields lost non-attachment content, got:\n%s", newDesc)
	}
}

// --- AgentFields Mode round-trip ---

func TestAgentFieldsModeRoundTrip(t *testing.T) {
	original := &AgentFields{
		RoleType:   "polecat",
		Rig:        "gastown",
		AgentState: "working",
		HookBead:   "gt-abc",
		Mode:       "ralph",
	}

	formatted := FormatAgentDescription("Polecat Test", original)
	if !strings.Contains(formatted, "mode: ralph") {
		t.Errorf("FormatAgentDescription missing mode field, got:\n%s", formatted)
	}

	parsed := ParseAgentFields(formatted)
	if parsed.Mode != "ralph" {
		t.Errorf("Mode: got %q, want %q", parsed.Mode, "ralph")
	}
	if parsed.RoleType != "polecat" {
		t.Errorf("RoleType: got %q, want %q", parsed.RoleType, "polecat")
	}
}

func TestAgentFieldsModeOmittedWhenEmpty(t *testing.T) {
	fields := &AgentFields{
		RoleType:   "polecat",
		Rig:        "gastown",
		AgentState: "working",
		// Mode intentionally empty
	}

	formatted := FormatAgentDescription("Polecat Test", fields)
	if strings.Contains(formatted, "mode:") {
		t.Errorf("FormatAgentDescription should not include mode when empty, got:\n%s", formatted)
	}
}

// --- ParseAgentFieldsFromDescription alias (not covered in beads_test.go) ---

func TestParseAgentFieldsFromDescription(t *testing.T) {
	desc := "role_type: polecat\nrig: gastown\nagent_state: working\nhook_bead: gt-abc\ncleanup_status: clean\nactive_mr: gt-mr1\nnotification_level: verbose"
	got := ParseAgentFieldsFromDescription(desc)
	if got.RoleType != "polecat" {
		t.Errorf("RoleType = %q, want %q", got.RoleType, "polecat")
	}
	if got.Rig != "gastown" {
		t.Errorf("Rig = %q, want %q", got.Rig, "gastown")
	}
	if got.AgentState != "working" {
		t.Errorf("AgentState = %q, want %q", got.AgentState, "working")
	}
	if got.HookBead != "gt-abc" {
		t.Errorf("HookBead = %q, want %q", got.HookBead, "gt-abc")
	}
	if got.CleanupStatus != "clean" {
		t.Errorf("CleanupStatus = %q, want %q", got.CleanupStatus, "clean")
	}
	if got.ActiveMR != "gt-mr1" {
		t.Errorf("ActiveMR = %q, want %q", got.ActiveMR, "gt-mr1")
	}
	if got.NotificationLevel != "verbose" {
		t.Errorf("NotificationLevel = %q, want %q", got.NotificationLevel, "verbose")
	}
}
