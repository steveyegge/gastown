package cmd

import "testing"

func TestIsReadyIssue_BlockingAndStatus(t *testing.T) {
	tests := []struct {
		name string
		in   trackedIssueInfo
		want bool
	}{
		{
			name: "closed issue never ready",
			in: trackedIssueInfo{
				Status:  "closed",
				Blocked: false,
			},
			want: false,
		},
		{
			name: "blocked open issue not ready",
			in: trackedIssueInfo{
				Status:  "open",
				Blocked: true,
			},
			want: false,
		},
		{
			name: "open unassigned issue ready",
			in: trackedIssueInfo{
				Status:  "open",
				Blocked: false,
			},
			want: true,
		},
		{
			name: "non-open unassigned issue treated ready for recovery",
			in: trackedIssueInfo{
				Status:  "in_progress",
				Blocked: false,
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isReadyIssue(tc.in)
			if got != tc.want {
				t.Fatalf("isReadyIssue() = %v, want %v", got, tc.want)
			}
		})
	}
}
