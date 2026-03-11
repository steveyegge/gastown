package witness

import (
	"testing"
)

func TestParseArtisanDone_Completed(t *testing.T) {
	t.Parallel()
	subject := "ARTISAN_DONE sculptor"
	body := `Exit: COMPLETED
Issue: gt-abc123
MR: gt-mr-xyz
Branch: feature-sculpt`

	payload, err := ParseArtisanDone(subject, body)
	if err != nil {
		t.Fatalf("ParseArtisanDone() error = %v", err)
	}

	if payload.Name != "sculptor" {
		t.Errorf("Name = %q, want %q", payload.Name, "sculptor")
	}
	if payload.ExitType != "COMPLETED" {
		t.Errorf("ExitType = %q, want %q", payload.ExitType, "COMPLETED")
	}
	if payload.BeadID != "gt-abc123" {
		t.Errorf("BeadID = %q, want %q", payload.BeadID, "gt-abc123")
	}
	if payload.MRID != "gt-mr-xyz" {
		t.Errorf("MRID = %q, want %q", payload.MRID, "gt-mr-xyz")
	}
	if payload.Branch != "feature-sculpt" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "feature-sculpt")
	}
}

func TestParseArtisanDone_Escalated(t *testing.T) {
	t.Parallel()
	subject := "ARTISAN_DONE weaver"
	body := `Exit: ESCALATED
Issue: gt-def456
Branch: fix-weave`

	payload, err := ParseArtisanDone(subject, body)
	if err != nil {
		t.Fatalf("ParseArtisanDone() error = %v", err)
	}

	if payload.Name != "weaver" {
		t.Errorf("Name = %q, want %q", payload.Name, "weaver")
	}
	if payload.ExitType != "ESCALATED" {
		t.Errorf("ExitType = %q, want %q", payload.ExitType, "ESCALATED")
	}
	if payload.BeadID != "gt-def456" {
		t.Errorf("BeadID = %q, want %q", payload.BeadID, "gt-def456")
	}
	if payload.MRID != "" {
		t.Errorf("MRID = %q, want empty", payload.MRID)
	}
	if payload.Branch != "fix-weave" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "fix-weave")
	}
}

func TestParseArtisanDone_PhaseComplete(t *testing.T) {
	t.Parallel()
	subject := "ARTISAN_DONE painter"
	body := `Exit: PHASE_COMPLETE
Issue: gt-ghi789
MR: gt-mr-abc
Branch: phase-one`

	payload, err := ParseArtisanDone(subject, body)
	if err != nil {
		t.Fatalf("ParseArtisanDone() error = %v", err)
	}

	if payload.Name != "painter" {
		t.Errorf("Name = %q, want %q", payload.Name, "painter")
	}
	if payload.ExitType != "PHASE_COMPLETE" {
		t.Errorf("ExitType = %q, want %q", payload.ExitType, "PHASE_COMPLETE")
	}
	if payload.BeadID != "gt-ghi789" {
		t.Errorf("BeadID = %q, want %q", payload.BeadID, "gt-ghi789")
	}
	if payload.MRID != "gt-mr-abc" {
		t.Errorf("MRID = %q, want %q", payload.MRID, "gt-mr-abc")
	}
	if payload.Branch != "phase-one" {
		t.Errorf("Branch = %q, want %q", payload.Branch, "phase-one")
	}
}

func TestParseArtisanDone_MissingName(t *testing.T) {
	t.Parallel()
	subject := "ARTISAN_DONE"
	body := "Exit: COMPLETED"

	_, err := ParseArtisanDone(subject, body)
	if err == nil {
		t.Fatal("ParseArtisanDone() expected error for missing name, got nil")
	}
}

func TestIsArtisanMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		subject string
		want    bool
	}{
		{"ARTISAN_DONE sculptor", true},
		{"ARTISAN_DONE weaver", true},
		{"ARTISAN_HELP: need guidance", true},
		{"ARTISAN_HELP: blocked on dependency", true},
		{"POLECAT_DONE nux", false},
		{"HELP: general help", false},
		{"MERGED nux", false},
		{"", false},
		{"ARTISAN", false},
		{"ARTISAN_DONE", false}, // no space after prefix, so HasPrefix with " " fails
	}

	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			got := IsArtisanMessage(tc.subject)
			if got != tc.want {
				t.Errorf("IsArtisanMessage(%q) = %v, want %v", tc.subject, got, tc.want)
			}
		})
	}
}
