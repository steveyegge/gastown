package cmd

import "testing"

func TestAssigneeToSessionName(t *testing.T) {
	tests := []struct {
		name           string
		assignee       string
		wantSession    string
		wantPersistent bool
	}{
		{
			name:           "two part polecat",
			assignee:       "schema_tools/nux",
			wantSession:    "gt-schema_tools-nux",
			wantPersistent: false,
		},
		{
			name:           "three part crew",
			assignee:       "schema_tools/crew/fiddler",
			wantSession:    "gt-schema_tools-crew-fiddler",
			wantPersistent: true,
		},
		{
			name:           "three part polecats",
			assignee:       "schema_tools/polecats/nux",
			wantSession:    "gt-schema_tools-nux",
			wantPersistent: false,
		},
		{
			name:           "unknown three part role",
			assignee:       "schema_tools/refinery/rig",
			wantSession:    "",
			wantPersistent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotSession, gotPersistent := assigneeToSessionName(tc.assignee)
			if gotSession != tc.wantSession {
				t.Fatalf("session = %q, want %q", gotSession, tc.wantSession)
			}
			if gotPersistent != tc.wantPersistent {
				t.Fatalf("persistent = %v, want %v", gotPersistent, tc.wantPersistent)
			}
		})
	}
}
