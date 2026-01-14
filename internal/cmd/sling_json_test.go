package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSlingResult_JSON(t *testing.T) {
	tests := []struct {
		name     string
		result   slingResult
		expected map[string]interface{}
	}{
		{
			name: "basic slung action",
			result: slingResult{
				Action:    slingActionSlung,
				BeadID:    "gt-abc123",
				Target:    "gastown/crew/joe",
				NudgeSent: true,
			},
			expected: map[string]interface{}{
				"action":     "slung",
				"bead_id":    "gt-abc123",
				"target":     "gastown/crew/joe",
				"nudge_sent": true,
			},
		},
		{
			name: "dry run action",
			result: slingResult{
				Action: slingActionDryRun,
				BeadID: "gt-abc123",
				Target: "gastown/crew/joe",
			},
			expected: map[string]interface{}{
				"action":     "dry_run",
				"bead_id":    "gt-abc123",
				"target":     "gastown/crew/joe",
				"nudge_sent": false,
			},
		},
		{
			name: "spawned polecat action",
			result: slingResult{
				Action:         slingActionSpawned,
				BeadID:         "gt-abc123",
				Target:         "gastown/polecats/Toast",
				SpawnedPolecat: true,
				PolecatName:    "Toast",
				NudgeSent:      true,
			},
			expected: map[string]interface{}{
				"action":          "spawned",
				"bead_id":         "gt-abc123",
				"target":          "gastown/polecats/Toast",
				"spawned_polecat": true,
				"polecat_name":    "Toast",
				"nudge_sent":      true,
			},
		},
		{
			name: "formula on bead",
			result: slingResult{
				Action:         slingActionSlung,
				BeadID:         "gt-wisp-xyz",
				OriginalBeadID: "gt-abc123",
				Target:         "mayor/",
				Formula:        "mol-review",
				WispID:         "gt-wisp-xyz",
				NudgeSent:      true,
			},
			expected: map[string]interface{}{
				"action":           "slung",
				"bead_id":          "gt-wisp-xyz",
				"original_bead_id": "gt-abc123",
				"target":           "mayor/",
				"formula":          "mol-review",
				"wisp_id":          "gt-wisp-xyz",
				"nudge_sent":       true,
			},
		},
		{
			name: "with convoy",
			result: slingResult{
				Action:    slingActionSlung,
				BeadID:    "gt-abc123",
				Target:    "gastown/polecats/nux",
				ConvoyID:  "gt-convoy-456",
				NudgeSent: true,
			},
			expected: map[string]interface{}{
				"action":     "slung",
				"bead_id":    "gt-abc123",
				"target":     "gastown/polecats/nux",
				"convoy_id":  "gt-convoy-456",
				"nudge_sent": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			for key, expectedVal := range tt.expected {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("missing key %q in JSON output", key)
					continue
				}
				if gotVal != expectedVal {
					t.Errorf("key %q: got %v, want %v", key, gotVal, expectedVal)
				}
			}
		})
	}
}

func TestSlingResult_OmitEmpty(t *testing.T) {
	// Test that empty optional fields are omitted
	result := slingResult{
		Action: slingActionSlung,
		BeadID: "gt-test",
		Target: "gastown/crew/joe",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Check raw JSON doesn't contain omitted fields
	if bytes.Contains(data, []byte("formula")) {
		t.Error("JSON should not contain 'formula' when empty")
	}
	if bytes.Contains(data, []byte("wisp_id")) {
		t.Error("JSON should not contain 'wisp_id' when empty")
	}
	if bytes.Contains(data, []byte("convoy_id")) {
		t.Error("JSON should not contain 'convoy_id' when empty")
	}
	if bytes.Contains(data, []byte("original_bead_id")) {
		t.Error("JSON should not contain 'original_bead_id' when empty")
	}
	if bytes.Contains(data, []byte("polecat_name")) {
		t.Error("JSON should not contain 'polecat_name' when empty")
	}
}

func TestSlingActionConstants(t *testing.T) {
	// Verify action constants have expected values
	if slingActionSlung != "slung" {
		t.Errorf("slingActionSlung = %q, want %q", slingActionSlung, "slung")
	}
	if slingActionDryRun != "dry_run" {
		t.Errorf("slingActionDryRun = %q, want %q", slingActionDryRun, "dry_run")
	}
	if slingActionSpawned != "spawned" {
		t.Errorf("slingActionSpawned = %q, want %q", slingActionSpawned, "spawned")
	}
}
