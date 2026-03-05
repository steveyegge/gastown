package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestVerifyMergeRequestRecord(t *testing.T) {
	tests := []struct {
		name           string
		issue          *beads.Issue
		expectedBranch string
		wantErr        bool
	}{
		{
			name:           "valid MR record",
			expectedBranch: "polecat/furiosa/gt-abc@mk123",
			issue: &beads.Issue{
				Labels: []string{"gt:merge-request"},
				Description: "branch: polecat/furiosa/gt-abc@mk123\n" +
					"target: main\nsource_issue: gt-abc",
			},
			wantErr: false,
		},
		{
			name:           "nil issue",
			expectedBranch: "polecat/furiosa/gt-abc@mk123",
			issue:          nil,
			wantErr:        true,
		},
		{
			name:           "missing merge-request label",
			expectedBranch: "polecat/furiosa/gt-abc@mk123",
			issue: &beads.Issue{
				Labels:      []string{"gt:task"},
				Description: "branch: polecat/furiosa/gt-abc@mk123",
			},
			wantErr: true,
		},
		{
			name:           "missing branch metadata",
			expectedBranch: "polecat/furiosa/gt-abc@mk123",
			issue: &beads.Issue{
				Labels:      []string{"gt:merge-request"},
				Description: "target: main\nsource_issue: gt-abc",
			},
			wantErr: true,
		},
		{
			name:           "branch mismatch",
			expectedBranch: "polecat/furiosa/gt-abc@mk123",
			issue: &beads.Issue{
				Labels:      []string{"gt:merge-request"},
				Description: "branch: polecat/furiosa/gt-def@mk123\ntarget: main",
			},
			wantErr: true,
		},
		{
			name:           "branch check can be skipped",
			expectedBranch: "",
			issue: &beads.Issue{
				Labels:      []string{"gt:merge-request"},
				Description: "branch: polecat/furiosa/gt-def@mk123\ntarget: main",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyMergeRequestRecord(tt.issue, tt.expectedBranch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("verifyMergeRequestRecord() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
