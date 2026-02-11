package beads

import (
	"testing"
)

// --- SynthesisFields (not covered in beads_test.go) ---

func TestParseSynthesisFields(t *testing.T) {
	tests := []struct {
		name string
		desc string
		want *SynthesisFields
	}{
		{
			name: "nil issue",
			desc: "",
			want: nil,
		},
		{
			name: "no synthesis fields",
			desc: "No structured fields here",
			want: nil,
		},
		{
			name: "all fields",
			desc: "convoy: gt-conv1\nreview_id: rev-001\noutput_path: /tmp/synthesis.md\nformula: code-review",
			want: &SynthesisFields{
				ConvoyID:   "gt-conv1",
				ReviewID:   "rev-001",
				OutputPath: "/tmp/synthesis.md",
				Formula:    "code-review",
			},
		},
		{
			name: "hyphenated keys",
			desc: "convoy-id: gt-conv2\nreview-id: rev-002\noutput-path: /tmp/out.md",
			want: &SynthesisFields{
				ConvoyID:   "gt-conv2",
				ReviewID:   "rev-002",
				OutputPath: "/tmp/out.md",
			},
		},
		{
			name: "convoy_id key variant",
			desc: "convoy_id: gt-conv3",
			want: &SynthesisFields{
				ConvoyID: "gt-conv3",
			},
		},
		{
			name: "empty values ignored",
			desc: "convoy: gt-conv4\nreview_id: \nformula: deep-review",
			want: &SynthesisFields{
				ConvoyID: "gt-conv4",
				Formula:  "deep-review",
			},
		},
		{
			name: "mixed with prose",
			desc: "Synthesis step for code review\n\nconvoy: gt-conv5\nformula: code-review\n\nThis synthesizes findings.",
			want: &SynthesisFields{
				ConvoyID: "gt-conv5",
				Formula:  "code-review",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var issue *Issue
			if tt.desc != "" {
				issue = &Issue{Description: tt.desc}
			}
			got := ParseSynthesisFields(issue)
			if tt.want == nil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil")
			}
			if got.ConvoyID != tt.want.ConvoyID {
				t.Errorf("ConvoyID = %q, want %q", got.ConvoyID, tt.want.ConvoyID)
			}
			if got.ReviewID != tt.want.ReviewID {
				t.Errorf("ReviewID = %q, want %q", got.ReviewID, tt.want.ReviewID)
			}
			if got.OutputPath != tt.want.OutputPath {
				t.Errorf("OutputPath = %q, want %q", got.OutputPath, tt.want.OutputPath)
			}
			if got.Formula != tt.want.Formula {
				t.Errorf("Formula = %q, want %q", got.Formula, tt.want.Formula)
			}
		})
	}
}

func TestFormatSynthesisFields(t *testing.T) {
	tests := []struct {
		name   string
		fields *SynthesisFields
		check  func(t *testing.T, got string)
	}{
		{
			name:   "nil",
			fields: nil,
			check: func(t *testing.T, got string) {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
			},
		},
		{
			name: "all fields",
			fields: &SynthesisFields{
				ConvoyID:   "gt-conv1",
				ReviewID:   "rev-001",
				OutputPath: "/tmp/out.md",
				Formula:    "code-review",
			},
			check: func(t *testing.T, got string) {
				for _, want := range []string{
					"convoy: gt-conv1",
					"review_id: rev-001",
					"output_path: /tmp/out.md",
					"formula: code-review",
				} {
					if !contains(got, want) {
						t.Errorf("missing %q in output:\n%s", want, got)
					}
				}
			},
		},
		{
			name:   "empty fields",
			fields: &SynthesisFields{},
			check: func(t *testing.T, got string) {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
			},
		},
		{
			name: "partial fields",
			fields: &SynthesisFields{
				ConvoyID: "gt-conv2",
			},
			check: func(t *testing.T, got string) {
				if !contains(got, "convoy: gt-conv2") {
					t.Errorf("missing convoy field, got: %q", got)
				}
				if contains(got, "review_id") {
					t.Errorf("unexpected review_id in output: %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSynthesisFields(tt.fields)
			tt.check(t, got)
		})
	}
}

func TestSynthesisFieldsRoundTrip(t *testing.T) {
	original := &SynthesisFields{
		ConvoyID:   "gt-conv-rt",
		ReviewID:   "rev-rt",
		OutputPath: "/tmp/synthesis-roundtrip.md",
		Formula:    "deep-review",
	}

	formatted := FormatSynthesisFields(original)
	issue := &Issue{Description: formatted}
	parsed := ParseSynthesisFields(issue)

	if parsed == nil {
		t.Fatal("round-trip parse returned nil")
	}
	if parsed.ConvoyID != original.ConvoyID {
		t.Errorf("ConvoyID: got %q, want %q", parsed.ConvoyID, original.ConvoyID)
	}
	if parsed.ReviewID != original.ReviewID {
		t.Errorf("ReviewID: got %q, want %q", parsed.ReviewID, original.ReviewID)
	}
	if parsed.OutputPath != original.OutputPath {
		t.Errorf("OutputPath: got %q, want %q", parsed.OutputPath, original.OutputPath)
	}
	if parsed.Formula != original.Formula {
		t.Errorf("Formula: got %q, want %q", parsed.Formula, original.Formula)
	}
}

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

// helper - strings.Contains alias for readability in checks
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexSubstring(s, substr) >= 0)
}

func indexSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
