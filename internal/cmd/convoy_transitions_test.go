package cmd

import "testing"

func TestEnsureKnownConvoyStatus(t *testing.T) {
	t.Parallel()

	if err := ensureKnownConvoyStatus("open"); err != nil {
		t.Fatalf("expected open to be accepted: %v", err)
	}
	if err := ensureKnownConvoyStatus(" closed "); err != nil {
		t.Fatalf("expected closed to be accepted: %v", err)
	}
	if err := ensureKnownConvoyStatus("in_progress"); err == nil {
		t.Fatal("expected unknown status to be rejected")
	}
}

func TestValidateConvoyStatusTransition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		target  string
		wantErr bool
	}{
		{name: "open to closed", current: "open", target: "closed", wantErr: false},
		{name: "closed to open", current: "closed", target: "open", wantErr: false},
		{name: "same open", current: "open", target: "open", wantErr: false},
		{name: "same closed", current: "closed", target: "closed", wantErr: false},
		{name: "unknown current", current: "in_progress", target: "closed", wantErr: true},
		{name: "unknown target", current: "open", target: "archived", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateConvoyStatusTransition(tc.current, tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for transition %q -> %q", tc.current, tc.target)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %q -> %q to pass, got %v", tc.current, tc.target, err)
			}
		})
	}
}
