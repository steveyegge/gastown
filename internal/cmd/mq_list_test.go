package cmd

import "testing"

func TestBuildMQListColumns_IncludesTarget(t *testing.T) {
	tests := []struct {
		name          string
		verify        bool
		wantColumnSeq []string
	}{
		{
			name:   "without verify",
			verify: false,
			wantColumnSeq: []string{
				"ID", "SCORE", "PRI", "CONVOY", "BRANCH", "TARGET", "STATUS", "AGE",
			},
		},
		{
			name:   "with verify",
			verify: true,
			wantColumnSeq: []string{
				"ID", "SCORE", "PRI", "CONVOY", "BRANCH", "TARGET", "STATUS", "GIT", "AGE",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols := buildMQListColumns(tt.verify)
			if len(cols) != len(tt.wantColumnSeq) {
				t.Fatalf("len(columns) = %d, want %d", len(cols), len(tt.wantColumnSeq))
			}
			for i, want := range tt.wantColumnSeq {
				if cols[i].Name != want {
					t.Fatalf("column[%d] = %q, want %q", i, cols[i].Name, want)
				}
			}
		})
	}
}
