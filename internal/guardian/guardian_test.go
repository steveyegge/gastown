package guardian

import (
	"context"
	"fmt"
	"testing"
)

// mockReviewFunc returns a ReviewFunc that returns a canned response.
func mockReviewFunc(response string, err error) ReviewFunc {
	return func(_ context.Context, _, _ string) (string, error) {
		return response, err
	}
}

func TestReview_Disabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	g := New(cfg, t.TempDir(), "")

	result, err := g.Review(context.Background(), &MergeDiff{DiffText: "some diff"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result when disabled, got %+v", result)
	}
}

func TestReview_NilDiff(t *testing.T) {
	cfg := &Config{Enabled: true}
	g := New(cfg, t.TempDir(), "")

	result, err := g.Review(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for nil diff, got %+v", result)
	}
}

func TestReview_EmptyDiff(t *testing.T) {
	cfg := &Config{Enabled: true}
	g := New(cfg, t.TempDir(), "")

	result, err := g.Review(context.Background(), &MergeDiff{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty diff, got %+v", result)
	}
}

func TestReview_SkipDocsOnly(t *testing.T) {
	cfg := &Config{Enabled: true}
	g := New(cfg, t.TempDir(), "")

	diff := &MergeDiff{
		BeadID:   "gt-abc123",
		DiffText: "some diff content",
		Worker:   "polecat-Toast",
		Stats:    DiffStats{DocsOnly: true, FilesChanged: 1},
	}

	result, err := g.Review(context.Background(), diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for docs-only skip")
	}
	if result.Recommendation != "skip" {
		t.Errorf("expected recommendation 'skip', got %q", result.Recommendation)
	}
	if result.Score != 1.0 {
		t.Errorf("expected score 1.0 for skip, got %f", result.Score)
	}
}

func TestReview_SkipConfigOnly(t *testing.T) {
	cfg := &Config{Enabled: true}
	g := New(cfg, t.TempDir(), "")

	diff := &MergeDiff{
		BeadID:   "gt-abc123",
		DiffText: "some diff content",
		Worker:   "polecat-Toast",
		Stats:    DiffStats{ConfigOnly: true, FilesChanged: 1},
	}

	result, err := g.Review(context.Background(), diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for config-only skip")
	}
	if result.Recommendation != "skip" {
		t.Errorf("expected recommendation 'skip', got %q", result.Recommendation)
	}
}

func TestReview_NormalApprove(t *testing.T) {
	cfg := &Config{Enabled: true, TimeoutSecs: 10}
	g := New(cfg, t.TempDir(), "")

	g.ReviewFunc = mockReviewFunc(`{
		"score": 0.85,
		"recommendation": "approve",
		"issues": [
			{
				"severity": "minor",
				"category": "style",
				"description": "Consider using a constant",
				"file": "main.go",
				"line": 10
			}
		]
	}`, nil)

	diff := &MergeDiff{
		BeadID:   "gt-abc123",
		Branch:   "polecat/toast",
		Target:   "main",
		Worker:   "polecat-Toast",
		Rig:      "gastown",
		DiffText: "+++ b/main.go\n+func hello() {}\n",
		Stats:    DiffStats{FilesChanged: 1, LinesAdded: 1},
	}

	result, err := g.Review(context.Background(), diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Score != 0.85 {
		t.Errorf("expected score 0.85, got %f", result.Score)
	}
	if result.Recommendation != "approve" {
		t.Errorf("expected 'approve', got %q", result.Recommendation)
	}
	if len(result.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(result.Issues))
	}
}

func TestReview_RequestChanges(t *testing.T) {
	cfg := &Config{Enabled: true, TimeoutSecs: 10}
	g := New(cfg, t.TempDir(), "")

	g.ReviewFunc = mockReviewFunc(`{
		"score": 0.35,
		"recommendation": "request_changes",
		"issues": [
			{"severity": "critical", "category": "correctness", "description": "nil pointer dereference"}
		]
	}`, nil)

	diff := &MergeDiff{
		BeadID:   "gt-def456",
		DiffText: "+++ b/handler.go\n+func handle() {}\n",
		Worker:   "polecat-Max",
		Stats:    DiffStats{FilesChanged: 1, LinesAdded: 5},
	}

	result, err := g.Review(context.Background(), diff)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Recommendation != "request_changes" {
		t.Errorf("expected 'request_changes', got %q", result.Recommendation)
	}
	if result.Score != 0.35 {
		t.Errorf("expected score 0.35, got %f", result.Score)
	}
}

func TestReview_ReviewFuncError(t *testing.T) {
	cfg := &Config{Enabled: true, TimeoutSecs: 10}
	g := New(cfg, t.TempDir(), "")

	g.ReviewFunc = mockReviewFunc("", fmt.Errorf("model unavailable"))

	diff := &MergeDiff{
		BeadID:   "gt-err123",
		DiffText: "+++ b/file.go\n+func f() {}\n",
		Worker:   "polecat-Joe",
		Stats:    DiffStats{FilesChanged: 1, LinesAdded: 1},
	}

	_, err := g.Review(context.Background(), diff)
	if err == nil {
		t.Fatal("expected error from Review")
	}
}

func TestReview_NoReviewFunc(t *testing.T) {
	cfg := &Config{Enabled: true, TimeoutSecs: 10}
	g := New(cfg, t.TempDir(), "")
	// ReviewFunc not set

	diff := &MergeDiff{
		BeadID:   "gt-nofunc",
		DiffText: "+++ b/file.go\n+func f() {}\n",
		Worker:   "polecat-Joe",
		Stats:    DiffStats{FilesChanged: 1, LinesAdded: 1},
	}

	_, err := g.Review(context.Background(), diff)
	if err == nil {
		t.Fatal("expected error when ReviewFunc is nil")
	}
}

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		name     string
		stats    DiffStats
		expected string
	}{
		{"empty", DiffStats{}, "low"},
		{"small change", DiffStats{FilesChanged: 2, LinesAdded: 20}, "low"},
		{"medium lines", DiffStats{FilesChanged: 3, LinesAdded: 150}, "medium"},
		{"many files", DiffStats{FilesChanged: 15, LinesAdded: 50}, "medium"},
		{"core paths", DiffStats{FilesChanged: 1, LinesAdded: 5, HasCorePaths: true}, "medium"},
		{"security paths", DiffStats{FilesChanged: 1, LinesAdded: 5, HasSecurityPaths: true}, "high"},
		{"large diff", DiffStats{FilesChanged: 20, LinesAdded: 600}, "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyRisk(tt.stats); got != tt.expected {
				t.Errorf("classifyRisk(%+v) = %q, want %q", tt.stats, got, tt.expected)
			}
		})
	}
}

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		stats    DiffStats
		expected bool
	}{
		{"normal", DiffStats{FilesChanged: 1}, false},
		{"docs only", DiffStats{DocsOnly: true}, true},
		{"config only", DiffStats{ConfigOnly: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSkip(tt.stats); got != tt.expected {
				t.Errorf("shouldSkip(%+v) = %v, want %v", tt.stats, got, tt.expected)
			}
		})
	}
}

func TestParseReviewResponse(t *testing.T) {
	diff := &MergeDiff{BeadID: "gt-abc", Worker: "toast", Rig: "gastown"}

	tests := []struct {
		name           string
		response       string
		expectedScore  float64
		expectedRec    string
		expectedIssues int
		wantErr        bool
	}{
		{
			name:           "valid approve",
			response:       `{"score": 0.9, "recommendation": "approve", "issues": []}`,
			expectedScore:  0.9,
			expectedRec:    "approve",
			expectedIssues: 0,
		},
		{
			name:           "with surrounding text",
			response:       `Here is my review:\n{"score": 0.7, "recommendation": "approve", "issues": [{"severity":"minor","category":"style","description":"nit"}]}`,
			expectedScore:  0.7,
			expectedRec:    "approve",
			expectedIssues: 1,
		},
		{
			name:          "score clamped above 1",
			response:      `{"score": 1.5, "recommendation": "approve"}`,
			expectedScore: 1.0,
			expectedRec:   "approve",
		},
		{
			name:          "score clamped below 0",
			response:      `{"score": -0.5, "recommendation": "request_changes"}`,
			expectedScore: 0.0,
			expectedRec:   "request_changes",
		},
		{
			name:          "invalid recommendation defaults based on score",
			response:      `{"score": 0.8, "recommendation": "maybe"}`,
			expectedScore: 0.8,
			expectedRec:   "approve",
		},
		{
			name:          "low score invalid rec defaults to request_changes",
			response:      `{"score": 0.3, "recommendation": "idk"}`,
			expectedScore: 0.3,
			expectedRec:   "request_changes",
		},
		{
			name:    "no JSON",
			response: "This is just text without any JSON",
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			response: `{"score": }`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseReviewResponse(tt.response, diff, 100.0)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Score != tt.expectedScore {
				t.Errorf("score = %f, want %f", result.Score, tt.expectedScore)
			}
			if result.Recommendation != tt.expectedRec {
				t.Errorf("recommendation = %q, want %q", result.Recommendation, tt.expectedRec)
			}
			if len(result.Issues) != tt.expectedIssues {
				t.Errorf("issues count = %d, want %d", len(result.Issues), tt.expectedIssues)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", `{"a": 1}`, `{"a": 1}`},
		{"with prefix", `Here: {"a": 1}`, `{"a": 1}`},
		{"nested", `{"a": {"b": 2}}`, `{"a": {"b": 2}}`},
		{"no json", "just text", ""},
		{"unclosed", `{"a": 1`, ""},
		{"braces in string", `{"desc": "use { and } in code"}`, `{"desc": "use { and } in code"}`},
		{"nested braces in string", `{"issues": [{"description": "missing } brace"}]}`, `{"issues": [{"description": "missing } brace"}]}`},
		{"prefix then braces in string", `Some text {"a": "val with {braces}"}`, `{"a": "val with {braces}"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractJSON(tt.input); got != tt.expected {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestComputeDiffStats(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,5 @@
 package main
+import "fmt"
+func hello() { fmt.Println("hi") }
-func old() {}
diff --git a/internal/config/loader.go b/internal/config/loader.go
--- a/internal/config/loader.go
+++ b/internal/config/loader.go
@@ -1,1 +1,2 @@
+func newConfig() {}
`

	stats := ComputeDiffStats(diff, nil)
	if stats.FilesChanged != 2 {
		t.Errorf("FilesChanged = %d, want 2", stats.FilesChanged)
	}
	if stats.LinesAdded != 3 {
		t.Errorf("LinesAdded = %d, want 3", stats.LinesAdded)
	}
	if stats.LinesRemoved != 1 {
		t.Errorf("LinesRemoved = %d, want 1", stats.LinesRemoved)
	}
	if !stats.HasCorePaths {
		t.Error("expected HasCorePaths to be true (internal/config)")
	}
	if stats.DocsOnly {
		t.Error("DocsOnly should be false")
	}
}

func TestComputeDiffStats_DocsOnly(t *testing.T) {
	diff := `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1,2 @@
+New docs line
`

	stats := ComputeDiffStats(diff, nil)
	if !stats.DocsOnly {
		t.Error("expected DocsOnly to be true")
	}
}

func TestComputeDiffStats_SecurityPaths(t *testing.T) {
	diff := `diff --git a/internal/auth/handler.go b/internal/auth/handler.go
--- a/internal/auth/handler.go
+++ b/internal/auth/handler.go
@@ -1 +1,2 @@
+func validateToken() {}
`

	stats := ComputeDiffStats(diff, nil)
	if !stats.HasSecurityPaths {
		t.Error("expected HasSecurityPaths to be true")
	}
}

func TestComputeDiffStats_CustomPaths(t *testing.T) {
	diff := `diff --git a/pkg/payments/handler.go b/pkg/payments/handler.go
--- a/pkg/payments/handler.go
+++ b/pkg/payments/handler.go
@@ -1 +1,2 @@
+func charge() {}
`

	cfg := &Config{
		SecurityPaths: []string{"payments", "billing"},
		CorePaths:     []string{"pkg/core"},
	}
	stats := ComputeDiffStats(diff, cfg)
	if !stats.HasSecurityPaths {
		t.Error("expected HasSecurityPaths with custom pattern 'payments'")
	}
	if stats.HasCorePaths {
		t.Error("expected HasCorePaths to be false (custom patterns don't match)")
	}
}

func TestBuildReviewPrompt(t *testing.T) {
	diff := &MergeDiff{
		Branch:   "polecat/toast",
		Target:   "main",
		Worker:   "polecat-Toast",
		DiffText: "+++ b/main.go\n+func hello() {}\n",
		Stats:    DiffStats{FilesChanged: 1, LinesAdded: 1},
	}

	prompt := buildReviewPrompt(diff, "standard")
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !containsAll(prompt, "polecat/toast", "main", "STANDARD") {
		t.Errorf("prompt missing expected content")
	}

	quickPrompt := buildReviewPrompt(diff, "quick")
	if !containsAll(quickPrompt, "QUICK") {
		t.Error("quick prompt missing QUICK keyword")
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
