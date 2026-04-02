package ci

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConvertToSentryEvent_DifferentReposGetDifferentFingerprints(t *testing.T) {
	evt1 := CIEvent{
		Repo: "outdoorsea/faultline", Branch: "main", Workflow: "CI",
		Conclusion: "failure", RunID: 100, Timestamp: time.Now(),
	}
	evt2 := CIEvent{
		Repo: "outdoorsea/gastown", Branch: "main", Workflow: "CI",
		Conclusion: "failure", RunID: 200, Timestamp: time.Now(),
	}

	raw1 := ConvertToSentryEvent(evt1)
	raw2 := ConvertToSentryEvent(evt2)

	fp1 := extractFingerprint(t, raw1)
	fp2 := extractFingerprint(t, raw2)

	if fp1[0] == fp2[0] {
		t.Fatalf("different repos should get different fingerprints: %v vs %v", fp1, fp2)
	}
}

func TestConvertToSentryEvent_SameRepoSameWorkflowGroupsTogether(t *testing.T) {
	evt1 := CIEvent{
		Repo: "outdoorsea/faultline", Branch: "main", Workflow: "CI",
		Conclusion: "failure", RunID: 100, Timestamp: time.Now(),
	}
	evt2 := CIEvent{
		Repo: "outdoorsea/faultline", Branch: "main", Workflow: "CI",
		Conclusion: "failure", RunID: 200, Timestamp: time.Now(),
	}

	raw1 := ConvertToSentryEvent(evt1)
	raw2 := ConvertToSentryEvent(evt2)

	fp1 := extractFingerprint(t, raw1)
	fp2 := extractFingerprint(t, raw2)

	if fp1[0] != fp2[0] {
		t.Fatalf("same repo+branch+workflow should group: %v vs %v", fp1, fp2)
	}
}

func TestConvertToSentryEvent_DifferentBranchesGetDifferentFingerprints(t *testing.T) {
	evt1 := CIEvent{
		Repo: "outdoorsea/faultline", Branch: "main", Workflow: "CI",
		Conclusion: "failure", RunID: 100, Timestamp: time.Now(),
	}
	evt2 := CIEvent{
		Repo: "outdoorsea/faultline", Branch: "feature-x", Workflow: "CI",
		Conclusion: "failure", RunID: 200, Timestamp: time.Now(),
	}

	raw1 := ConvertToSentryEvent(evt1)
	raw2 := ConvertToSentryEvent(evt2)

	fp1 := extractFingerprint(t, raw1)
	fp2 := extractFingerprint(t, raw2)

	if fp1[0] == fp2[0] {
		t.Fatalf("different branches should get different fingerprints: %v vs %v", fp1, fp2)
	}
}

func TestConvertToSentryEvent_TitleIncludesRepo(t *testing.T) {
	evt := CIEvent{
		Repo: "outdoorsea/faultline", Branch: "main", Workflow: "CI",
		Conclusion: "failure", RunID: 100, Timestamp: time.Now(),
	}

	raw := ConvertToSentryEvent(evt)
	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Message == "" {
		t.Fatal("empty message")
	}
	// Title should include "faultline" (short repo name).
	if !contains(parsed.Message, "faultline") {
		t.Fatalf("title should include repo name, got: %s", parsed.Message)
	}
}

func extractFingerprint(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var parsed struct {
		Fingerprint []string `json:"fingerprint"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Fingerprint) == 0 {
		t.Fatal("no fingerprint in event")
	}
	return parsed.Fingerprint
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
