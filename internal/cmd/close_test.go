package cmd

import (
	"encoding/json"
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

func TestExtractCascadeFlag(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantCascade bool
		wantArgs    []string
	}{
		{
			name:        "no cascade flag",
			args:        []string{"gt-abc", "--force"},
			wantCascade: false,
			wantArgs:    []string{"gt-abc", "--force"},
		},
		{
			name:        "cascade flag present",
			args:        []string{"gt-abc", "--cascade"},
			wantCascade: true,
			wantArgs:    []string{"gt-abc"},
		},
		{
			name:        "cascade flag with other flags",
			args:        []string{"--cascade", "gt-abc", "--reason", "Done"},
			wantCascade: true,
			wantArgs:    []string{"gt-abc", "--reason", "Done"},
		},
		{
			name:        "empty args",
			args:        []string{},
			wantCascade: false,
			wantArgs:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCascade, gotArgs := extractCascadeFlag(tt.args)
			if gotCascade != tt.wantCascade {
				t.Errorf("extractCascadeFlag(%v) cascade = %v, want %v", tt.args, gotCascade, tt.wantCascade)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("extractCascadeFlag(%v) args = %v, want %v", tt.args, gotArgs, tt.wantArgs)
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Errorf("extractCascadeFlag(%v) args[%d] = %q, want %q", tt.args, i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestChildBeadUnmarshal(t *testing.T) {
	jsonData := `[{"id":"gt-abc","status":"open"},{"id":"gt-def","status":"closed"}]`
	var children []childBead
	if err := json.Unmarshal([]byte(jsonData), &children); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("got %d children, want 2", len(children))
	}
	if children[0].ID != "gt-abc" || children[0].Status != "open" {
		t.Errorf("child[0] = %+v, want {ID:gt-abc Status:open}", children[0])
	}
	if children[1].ID != "gt-def" || children[1].Status != "closed" {
		t.Errorf("child[1] = %+v, want {ID:gt-def Status:closed}", children[1])
	}
}
