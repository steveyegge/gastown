//go:build integration

package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestRigAddURLValidation(t *testing.T) {
	_ = mockBdCommand(t)
	townRoot := setupTestTown(t)
	
	// We need to be in the workspace for FindFromCwdOrError
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir to townRoot: %v", err)
	}
	defer os.Chdir(oldCwd)

	gitURL := "https://github.com/org/repo.git"
	
	tests := []struct {
		name        string
		pushURL     string
		upstreamURL string
		wantErr     string
	}{
		{
			name:        "invalid push url",
			pushURL:     "/local/path",
			upstreamURL: "",
			wantErr:     "invalid push URL",
		},
		{
			name:        "invalid upstream url",
			pushURL:     "",
			upstreamURL: "not-a-url",
			wantErr:     "invalid upstream URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags (they are global vars in rig.go)
			rigAddPushURL = tt.pushURL
			rigAddUpstreamURL = tt.upstreamURL
			rigAddAdopt = false
			
			// Call runRigAdd
			err := runRigAdd(nil, []string{"myrig", gitURL})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
