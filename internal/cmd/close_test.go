package cmd

import (
	"testing"
)

func TestExtractBeadIDs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single bead ID",
			args: []string{"gt-abc"},
			want: []string{"gt-abc"},
		},
		{
			name: "multiple bead IDs",
			args: []string{"gt-abc", "gt-def"},
			want: []string{"gt-abc", "gt-def"},
		},
		{
			name: "bead ID with boolean flags",
			args: []string{"--force", "gt-abc", "--suggest-next"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with short boolean flag",
			args: []string{"-f", "gt-abc"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with reason flag (separate value)",
			args: []string{"gt-abc", "--reason", "Done"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with reason flag (= form)",
			args: []string{"gt-abc", "--reason=Done"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with short reason flag",
			args: []string{"-r", "Done", "gt-abc"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with comment alias",
			args: []string{"--comment", "Finished", "gt-abc"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with session flag",
			args: []string{"gt-abc", "--session", "sess-123"},
			want: []string{"gt-abc"},
		},
		{
			name: "bead ID with db flag",
			args: []string{"--db", "/path/to/db", "gt-abc"},
			want: []string{"gt-abc"},
		},
		{
			name: "no bead IDs (flags only)",
			args: []string{"--force", "--reason", "cleanup"},
			want: nil,
		},
		{
			name: "empty args",
			args: []string{},
			want: nil,
		},
		{
			name: "multiple IDs with mixed flags",
			args: []string{"--force", "gt-abc", "--reason", "Done", "hq-cv-xyz", "-v"},
			want: []string{"gt-abc", "hq-cv-xyz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBeadIDs(tt.args)
			if len(got) != len(tt.want) {
				t.Fatalf("extractBeadIDs(%v) = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractBeadIDs(%v)[%d] = %q, want %q", tt.args, i, got[i], tt.want[i])
				}
			}
		})
	}
}
