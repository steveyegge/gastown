package swarm

import "testing"

func TestIsBeadsOnlyChanges(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{
			name:   "empty output",
			input:  "",
			expect: true,
		},
		{
			name:   "staged beads file",
			input:  "M  .beads/issues.jsonl\n",
			expect: true,
		},
		{
			name:   "working-tree-only beads file",
			input:  " M .beads/issues.jsonl\n",
			expect: true,
		},
		{
			name:   "untracked beads file",
			input:  "?? .beads/new-file\n",
			expect: true,
		},
		{
			name:   "staged code file",
			input:  "M  main.go\n",
			expect: false,
		},
		{
			name:   "working-tree-only code file",
			input:  " M main.go\n",
			expect: false,
		},
		{
			name:   "mixed beads and code",
			input:  "M  .beads/issues.jsonl\n M main.go\n",
			expect: false,
		},
		{
			name:   "multiple beads files",
			input:  "M  .beads/issues.jsonl\n M .beads/config.json\n?? .beads/new\n",
			expect: true,
		},
		{
			name:   "deleted beads file in working tree",
			input:  " D .beads/old-file\n",
			expect: true,
		},
		{
			name:   "renamed beads file",
			input:  "R  .beads/old -> .beads/new\n",
			expect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isBeadsOnlyChanges(tc.input)
			if got != tc.expect {
				t.Errorf("isBeadsOnlyChanges(%q) = %v, want %v", tc.input, got, tc.expect)
			}
		})
	}
}
