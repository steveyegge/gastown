package status

import "testing"

func TestParseAssignee(t *testing.T) {
	tests := []struct {
		assignee    string
		wantRig     string
		wantPolecat string
		wantOK      bool
	}{
		{"gastown/polecats/angharad", "gastown", "angharad", true},
		{"roxas/polecats/dag", "roxas", "dag", true},
		{"myrig/polecats/worker", "myrig", "worker", true},
		{"invalid", "", "", false},
		{"two/parts", "", "", false},
		{"too/many/slash/parts", "", "", false},
		{"rig/notpolecats/name", "", "", false},
		{"rig/crew/name", "", "", false},
		{"", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.assignee, func(t *testing.T) {
			rig, polecat, ok := ParseAssignee(tt.assignee)
			if ok != tt.wantOK {
				t.Errorf("ParseAssignee(%q) ok = %v, want %v", tt.assignee, ok, tt.wantOK)
			}
			if rig != tt.wantRig {
				t.Errorf("ParseAssignee(%q) rig = %q, want %q", tt.assignee, rig, tt.wantRig)
			}
			if polecat != tt.wantPolecat {
				t.Errorf("ParseAssignee(%q) polecat = %q, want %q", tt.assignee, polecat, tt.wantPolecat)
			}
		})
	}
}

func TestFormatSessionName(t *testing.T) {
	tests := []struct {
		rig     string
		polecat string
		want    string
	}{
		{"gastown", "angharad", "gt-gastown-angharad"},
		{"roxas", "dag", "gt-roxas-dag"},
		{"myrig", "worker", "gt-myrig-worker"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatSessionName(tt.rig, tt.polecat); got != tt.want {
				t.Errorf("formatSessionName(%q, %q) = %q, want %q", tt.rig, tt.polecat, got, tt.want)
			}
		})
	}
}

func TestWorktreePathForPolecat(t *testing.T) {
	tests := []struct {
		townRoot string
		rig      string
		polecat  string
		want     string
	}{
		{"/home/user/gt", "gastown", "angharad", "/home/user/gt/gastown/polecats/angharad"},
		{"/tmp/town", "roxas", "dag", "/tmp/town/roxas/polecats/dag"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := WorktreePathForPolecat(tt.townRoot, tt.rig, tt.polecat); got != tt.want {
				t.Errorf("WorktreePathForPolecat() = %q, want %q", got, tt.want)
			}
		})
	}
}
