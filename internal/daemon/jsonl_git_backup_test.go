package daemon

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIsTestPollution(t *testing.T) {
	tests := []struct {
		name     string
		record   map[string]interface{}
		expected bool
	}{
		{
			name:     "normal issue",
			record:   map[string]interface{}{"id": "gt-abc1", "title": "Fix login bug"},
			expected: false,
		},
		{
			name:     "title starts with Test Issue",
			record:   map[string]interface{}{"id": "gt-xyz2", "title": "Test Issue for validation"},
			expected: true,
		},
		{
			name:     "title starts with test issue lowercase",
			record:   map[string]interface{}{"id": "gt-xyz2", "title": "test issue for validation"},
			expected: true,
		},
		{
			name:     "short test id bd-1",
			record:   map[string]interface{}{"id": "bd-1", "title": "Something"},
			expected: true,
		},
		{
			name:     "short test id bd-99",
			record:   map[string]interface{}{"id": "bd-99", "title": "Something"},
			expected: true,
		},
		{
			name:     "test-style id bd-abc12",
			record:   map[string]interface{}{"id": "bd-abc12", "title": "Something"},
			expected: true,
		},
		{
			name:     "testdb prefix id",
			record:   map[string]interface{}{"id": "testdb_foo", "title": "Something"},
			expected: true,
		},
		{
			name:     "beads_t prefix id",
			record:   map[string]interface{}{"id": "beads_t123", "title": "Something"},
			expected: true,
		},
		{
			name:     "beads_pt prefix id",
			record:   map[string]interface{}{"id": "beads_pt456", "title": "Something"},
			expected: true,
		},
		{
			name:     "doctest prefix id",
			record:   map[string]interface{}{"id": "doctest_foo", "title": "Something"},
			expected: true,
		},
		{
			name:     "title starts with test_",
			record:   map[string]interface{}{"id": "gt-ok1", "title": "test_something"},
			expected: true,
		},
		{
			name:     "title starts with test space",
			record:   map[string]interface{}{"id": "gt-ok1", "title": "test something"},
			expected: true,
		},
		{
			name:     "normal id with test in middle",
			record:   map[string]interface{}{"id": "gt-test1", "title": "Normal title"},
			expected: false,
		},
		{
			name:     "longer legitimate id bd-abcde12",
			record:   map[string]interface{}{"id": "bd-abcde12", "title": "Something"},
			expected: true,
		},
		{
			name:     "legitimate bd id bd-abcdef",
			record:   map[string]interface{}{"id": "bd-abcdef", "title": "Something"},
			expected: false,
		},
		{
			name:     "empty record",
			record:   map[string]interface{}{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTestPollution(tt.record)
			if got != tt.expected {
				t.Errorf("isTestPollution(%v) = %v, want %v", tt.record, got, tt.expected)
			}
		})
	}
}

func TestFilterTestPollution(t *testing.T) {
	// Build JSONL with mix of good and bad records.
	good1, _ := json.Marshal(map[string]interface{}{"id": "gt-abc1", "title": "Fix bug"})
	good2, _ := json.Marshal(map[string]interface{}{"id": "gt-def2", "title": "Add feature"})
	bad1, _ := json.Marshal(map[string]interface{}{"id": "bd-1", "title": "test thing"})
	bad2, _ := json.Marshal(map[string]interface{}{"id": "gt-xyz3", "title": "Test Issue 42"})

	input := string(good1) + "\n" + string(bad1) + "\n" + string(good2) + "\n" + string(bad2) + "\n"

	filtered, removed := filterTestPollution([]byte(input))

	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	// Verify only good records remain.
	lines := splitNonEmpty(string(filtered))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines after filter, got %d: %v", len(lines), lines)
	}

	// Verify the good records are preserved.
	for _, line := range lines {
		var rec map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("failed to parse filtered line: %v", err)
		}
		if isTestPollution(rec) {
			t.Errorf("test pollution record survived filtering: %v", rec)
		}
	}
}

func TestFilterTestPollution_NoRemoval(t *testing.T) {
	good1, _ := json.Marshal(map[string]interface{}{"id": "gt-abc1", "title": "Fix bug"})
	good2, _ := json.Marshal(map[string]interface{}{"id": "gt-def2", "title": "Add feature"})
	input := string(good1) + "\n" + string(good2) + "\n"

	filtered, removed := filterTestPollution([]byte(input))

	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}

	lines := splitNonEmpty(string(filtered))
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestFilterTestPollution_EmptyInput(t *testing.T) {
	filtered, removed := filterTestPollution([]byte(""))
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
	if len(filtered) != 0 {
		t.Errorf("expected empty output, got %q", filtered)
	}
}

func TestSpikeThreshold(t *testing.T) {
	// nil config → default
	if got := spikeThreshold(nil); got != defaultSpikeThreshold {
		t.Errorf("expected %v, got %v", defaultSpikeThreshold, got)
	}

	// nil SpikeThreshold → default
	config := &JsonlGitBackupConfig{}
	if got := spikeThreshold(config); got != defaultSpikeThreshold {
		t.Errorf("expected %v, got %v", defaultSpikeThreshold, got)
	}

	// custom threshold
	threshold := 0.10
	config.SpikeThreshold = &threshold
	if got := spikeThreshold(config); got != 0.10 {
		t.Errorf("expected 0.10, got %v", got)
	}

	// invalid threshold (> 1.0) → default
	invalid := 1.5
	config.SpikeThreshold = &invalid
	if got := spikeThreshold(config); got != defaultSpikeThreshold {
		t.Errorf("expected default for invalid threshold, got %v", got)
	}

	// zero threshold → default
	zero := 0.0
	config.SpikeThreshold = &zero
	if got := spikeThreshold(config); got != defaultSpikeThreshold {
		t.Errorf("expected default for zero threshold, got %v", got)
	}
}

func TestFormatSpikeReport(t *testing.T) {
	spikes := []spikeInfo{
		{DB: "prod_beads", File: "prod_beads/issues.jsonl", Previous: 100, Current: 150, Delta: 0.50},
		{DB: "dev_beads", File: "dev_beads/issues.jsonl", Previous: 200, Current: 50, Delta: 0.75},
	}
	report := formatSpikeReport(spikes)
	if report == "" {
		t.Fatal("expected non-empty report")
	}
	// Verify it mentions both databases.
	if got := report; !contains(got, "prod_beads") || !contains(got, "dev_beads") {
		t.Errorf("report should mention both databases: %s", got)
	}
	if !contains(report, "JUMP") {
		t.Errorf("report should mention JUMP for increase: %s", report)
	}
	if !contains(report, "DROP") {
		t.Errorf("report should mention DROP for decrease: %s", report)
	}
}

func TestVerifyExportCounts_FirstExport(t *testing.T) {
	// Set up a git repo with no prior commits containing our file.
	gitRepo := t.TempDir()
	initGitRepo(t, gitRepo)

	d := &Daemon{logger: log.New(io.Discard, "", 0)}

	counts := map[string]int{"testdb": 100}
	spikes := d.verifyExportCounts(gitRepo, []string{"testdb"}, counts, 0.20)
	if len(spikes) != 0 {
		t.Errorf("expected no spikes on first export, got %v", spikes)
	}
}

func TestVerifyExportCounts_WithinThreshold(t *testing.T) {
	gitRepo := t.TempDir()
	initGitRepo(t, gitRepo)

	// Create a baseline: 100 lines in testdb/issues.jsonl
	dbDir := filepath.Join(gitRepo, "testdb")
	os.MkdirAll(dbDir, 0755)
	writeNLines(t, filepath.Join(dbDir, "issues.jsonl"), 100)
	commitAll(t, gitRepo, "baseline")

	d := &Daemon{logger: log.New(io.Discard, "", 0)}

	// 110 records = 10% change, under 20% threshold
	counts := map[string]int{"testdb": 110}
	spikes := d.verifyExportCounts(gitRepo, []string{"testdb"}, counts, 0.20)
	if len(spikes) != 0 {
		t.Errorf("expected no spikes for 10%% change, got %v", spikes)
	}
}

func TestVerifyExportCounts_ExceedsThreshold(t *testing.T) {
	gitRepo := t.TempDir()
	initGitRepo(t, gitRepo)

	// Create baseline: 100 lines
	dbDir := filepath.Join(gitRepo, "testdb")
	os.MkdirAll(dbDir, 0755)
	writeNLines(t, filepath.Join(dbDir, "issues.jsonl"), 100)
	commitAll(t, gitRepo, "baseline")

	d := &Daemon{logger: log.New(io.Discard, "", 0)}

	// 130 records = 30% jump, exceeds 20% threshold
	counts := map[string]int{"testdb": 130}
	spikes := d.verifyExportCounts(gitRepo, []string{"testdb"}, counts, 0.20)
	if len(spikes) != 1 {
		t.Fatalf("expected 1 spike, got %d", len(spikes))
	}
	if spikes[0].DB != "testdb" {
		t.Errorf("expected spike for testdb, got %s", spikes[0].DB)
	}
	if spikes[0].Previous != 100 || spikes[0].Current != 130 {
		t.Errorf("expected 100→130, got %d→%d", spikes[0].Previous, spikes[0].Current)
	}
}

func TestVerifyExportCounts_Drop(t *testing.T) {
	gitRepo := t.TempDir()
	initGitRepo(t, gitRepo)

	dbDir := filepath.Join(gitRepo, "testdb")
	os.MkdirAll(dbDir, 0755)
	writeNLines(t, filepath.Join(dbDir, "issues.jsonl"), 100)
	commitAll(t, gitRepo, "baseline")

	d := &Daemon{logger: log.New(io.Discard, "", 0)}

	// 60 records = 40% drop, exceeds 20% threshold
	counts := map[string]int{"testdb": 60}
	spikes := d.verifyExportCounts(gitRepo, []string{"testdb"}, counts, 0.20)
	if len(spikes) != 1 {
		t.Fatalf("expected 1 spike for drop, got %d", len(spikes))
	}
	if spikes[0].Delta < 0.3 {
		t.Errorf("expected delta > 0.3, got %f", spikes[0].Delta)
	}
}

func TestCountFileLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	writeNLines(t, path, 42)

	got, err := countFileLines(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Errorf("expected 42 lines, got %d", got)
	}
}

func TestCountFileLines_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0644)

	got, err := countFileLines(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("expected 0 lines, got %d", got)
	}
}

func TestParseLineCount(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		{"42", 42, false},
		{"  42 filename.jsonl", 42, false},
		{"  0", 0, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		got, err := parseLineCount(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseLineCount(%q): error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.expected {
			t.Errorf("parseLineCount(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

// --- helpers ---

func splitNonEmpty(s string) []string {
	var result []string
	for _, line := range splitLines(s) {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init failed: %v: %s", err, out)
		}
	}
	// Need at least one commit for HEAD to exist.
	readme := filepath.Join(dir, "README")
	os.WriteFile(readme, []byte("init\n"), 0644)
	commitAll(t, dir, "init")
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	cmds := [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", msg, "--author=Test <test@test.com>"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, out)
		}
	}
}

func writeNLines(t *testing.T, path string, n int) {
	t.Helper()
	var buf []byte
	for i := 0; i < n; i++ {
		line, _ := json.Marshal(map[string]interface{}{"id": "rec-" + itoa(i), "title": "Record " + itoa(i)})
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(path, buf, 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
