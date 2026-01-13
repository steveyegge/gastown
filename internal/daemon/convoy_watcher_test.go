package daemon

import (
	"encoding/json"
	"testing"
)

func TestBdActivityEventParsing(t *testing.T) {
	testCases := []struct {
		name        string
		line        string
		wantType    string
		wantIssueID string
		wantNew     string
	}{
		{
			name:        "status change to closed",
			line:        `{"timestamp":"2026-01-12T02:50:35.778328-08:00","type":"status","issue_id":"gt-uoc64","symbol":"✓","message":"gt-uoc64 completed","old_status":"in_progress","new_status":"closed"}`,
			wantType:    "status",
			wantIssueID: "gt-uoc64",
			wantNew:     "closed",
		},
		{
			name:        "status change to in_progress",
			line:        `{"timestamp":"2026-01-12T02:43:04.467992-08:00","type":"status","issue_id":"gt-uoc64","symbol":"→","message":"gt-uoc64 started","old_status":"open","new_status":"in_progress","actor":"gastown/crew/george"}`,
			wantType:    "status",
			wantIssueID: "gt-uoc64",
			wantNew:     "in_progress",
		},
		{
			name:        "create event",
			line:        `{"timestamp":"2026-01-12T01:19:01.753578-08:00","type":"create","issue_id":"gt-dgbwk","symbol":"+","message":"gt-dgbwk created"}`,
			wantType:    "create",
			wantIssueID: "gt-dgbwk",
			wantNew:     "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var event bdActivityEvent
			err := json.Unmarshal([]byte(tc.line), &event)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			if event.Type != tc.wantType {
				t.Errorf("type = %q, want %q", event.Type, tc.wantType)
			}
			if event.IssueID != tc.wantIssueID {
				t.Errorf("issue_id = %q, want %q", event.IssueID, tc.wantIssueID)
			}
			if event.NewStatus != tc.wantNew {
				t.Errorf("new_status = %q, want %q", event.NewStatus, tc.wantNew)
			}
		})
	}
}

func TestIsCloseEvent(t *testing.T) {
	closedEvent := bdActivityEvent{
		Type:      "status",
		IssueID:   "gt-test",
		NewStatus: "closed",
	}

	if closedEvent.Type != "status" || closedEvent.NewStatus != "closed" {
		t.Error("should detect close event")
	}

	inProgressEvent := bdActivityEvent{
		Type:      "status",
		IssueID:   "gt-test",
		NewStatus: "in_progress",
	}

	if inProgressEvent.Type == "status" && inProgressEvent.NewStatus == "closed" {
		t.Error("should not detect in_progress as close")
	}

	createEvent := bdActivityEvent{
		Type:    "create",
		IssueID: "gt-test",
	}

	if createEvent.Type == "status" && createEvent.NewStatus == "closed" {
		t.Error("should not detect create as close")
	}
}
