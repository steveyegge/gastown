//go:build integration

package wasteland

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initDoltRepo creates a local dolt repo with the stamps table and seeds it
// with test data. Returns the repo directory path.
func initDoltRepo(t *testing.T) string {
	t.Helper()

	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		t.Skip("dolt not found in PATH — skipping integration test")
	}

	base := t.TempDir()
	// Use separate dirs for dolt home (config) and the repo to avoid
	// dolt config --global creating .dolt before dolt init.
	homeDir := filepath.Join(base, "home")
	repoDir := filepath.Join(base, "repo")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	doltEnv := append(os.Environ(),
		"DOLT_ROOT_PATH="+homeDir,
		"HOME="+homeDir,
	)

	// Configure dolt user (required for commits)
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(doltPath, args...)
		cmd.Dir = dir
		cmd.Env = doltEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("dolt %v failed: %v\n%s", args, err, out)
		}
	}

	run(homeDir, "config", "--global", "--add", "user.email", "test@test.com")
	run(homeDir, "config", "--global", "--add", "user.name", "test")
	run(repoDir, "init")
	dir := repoDir

	// Create stamps table matching wl_commons schema
	run(dir, "sql", "-q", `CREATE TABLE stamps (
		id VARCHAR(64) PRIMARY KEY,
		author VARCHAR(255) NOT NULL,
		subject VARCHAR(255) NOT NULL,
		valence JSON NOT NULL,
		confidence FLOAT DEFAULT 1,
		severity VARCHAR(16) DEFAULT 'leaf',
		context_id VARCHAR(64),
		context_type VARCHAR(32),
		skill_tags JSON,
		message TEXT,
		prev_stamp_hash VARCHAR(64),
		block_hash VARCHAR(64),
		hop_uri VARCHAR(512),
		created_at TIMESTAMP,
		CHECK (NOT(author = subject))
	)`)

	return dir
}

// seedStamps inserts stamp records into a dolt repo for testing.
func seedStamps(t *testing.T, doltPath, dir string, stamps []testStamp) {
	t.Helper()
	for _, s := range stamps {
		q := fmt.Sprintf(
			`INSERT INTO stamps (id, author, subject, valence, confidence) VALUES ('%s', '%s', '%s', '%s', %f)`,
			s.id, s.author, s.subject, s.valence, s.confidence,
		)
		cmd := exec.Command(doltPath, "sql", "-q", q)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("seeding stamp %s: %v\n%s", s.id, err, out)
		}
	}
}

type testStamp struct {
	id, author, subject, valence string
	confidence                   float64
}

func TestSpiderIntegration_CollusionDetected(t *testing.T) {
	dir := initDoltRepo(t)
	doltPath, _ := exec.LookPath("dolt")

	// Alice and Bob stamp each other heavily (collusion pattern).
	// Carol provides background diversity.
	stamps := []testStamp{
		{"s1", "alice", "bob", `{"quality":5}`, 0.9},
		{"s2", "alice", "bob", `{"quality":4}`, 0.8},
		{"s3", "alice", "bob", `{"quality":5}`, 0.9},
		{"s4", "bob", "alice", `{"quality":5}`, 0.9},
		{"s5", "bob", "alice", `{"quality":4}`, 0.8},
		{"s6", "bob", "alice", `{"quality":5}`, 0.9},
		// Carol stamps alice once — gives alice some non-bob stamps
		{"s7", "carol", "alice", `{"quality":3}`, 0.7},
		// Alice stamps carol once — gives alice one non-bob outgoing
		{"s8", "alice", "carol", `{"quality":3}`, 0.7},
	}
	seedStamps(t, doltPath, dir, stamps)

	cfg := SpiderConfig{
		MinStampsForCollusion:   3,
		CollusionRatioThreshold: 0.5,
		RubberStampMinCount:     100, // high threshold to suppress
		ConfidenceFloor:         1.1, // impossible to suppress
		ConfidenceMinStamps:     100, // high to suppress
	}

	signals, err := RunSpiderDetection(doltPath, dir, cfg)
	if err != nil {
		t.Fatalf("RunSpiderDetection: %v", err)
	}

	var collusionSignals []FraudSignal
	for _, s := range signals {
		if s.Kind == SignalCollusion {
			collusionSignals = append(collusionSignals, s)
		}
	}

	if len(collusionSignals) == 0 {
		t.Fatal("expected collusion signals, got none")
	}

	// Verify the signal has meaningful data
	sig := collusionSignals[0]
	if sig.Score <= 0 || sig.Score > 1.0 {
		t.Errorf("score should be (0,1], got %f", sig.Score)
	}
	if len(sig.Rigs) < 2 {
		t.Errorf("expected at least 2 rigs, got %v", sig.Rigs)
	}
	if sig.SampleSize == 0 {
		t.Error("expected non-zero SampleSize")
	}
	if sig.Detail == "" {
		t.Error("expected non-empty Detail")
	}
}

func TestSpiderIntegration_RubberStampDetected(t *testing.T) {
	dir := initDoltRepo(t)
	doltPath, _ := exec.LookPath("dolt")

	// Alice gives identical valence to many subjects — rubber-stamping.
	stamps := []testStamp{
		{"s1", "alice", "bob", `{"quality":5}`, 0.9},
		{"s2", "alice", "carol", `{"quality":5}`, 0.9},
		{"s3", "alice", "dave", `{"quality":5}`, 0.9},
		{"s4", "alice", "eve", `{"quality":5}`, 0.9},
		{"s5", "alice", "frank", `{"quality":5}`, 0.9},
	}
	seedStamps(t, doltPath, dir, stamps)

	cfg := SpiderConfig{
		MinStampsForCollusion:   100, // suppress
		CollusionRatioThreshold: 1.1, // suppress
		RubberStampMinCount:     5,
		ConfidenceFloor:         1.1, // suppress
		ConfidenceMinStamps:     100, // suppress
	}

	signals, err := RunSpiderDetection(doltPath, dir, cfg)
	if err != nil {
		t.Fatalf("RunSpiderDetection: %v", err)
	}

	var found bool
	for _, s := range signals {
		if s.Kind == SignalRubberStamp {
			found = true
			if s.Score < 0.9 {
				t.Errorf("rubber-stamp uniformity_ratio should be ~1.0, got %f", s.Score)
			}
		}
	}
	if !found {
		t.Fatal("expected rubber-stamp signal, got none")
	}
}

func TestSpiderIntegration_ConfidenceInflationDetected(t *testing.T) {
	dir := initDoltRepo(t)
	doltPath, _ := exec.LookPath("dolt")

	// Alice always gives confidence=1.0 — inflation pattern.
	stamps := []testStamp{
		{"s1", "alice", "bob", `{"quality":5}`, 1.0},
		{"s2", "alice", "carol", `{"quality":4}`, 1.0},
		{"s3", "alice", "dave", `{"quality":3}`, 1.0},
		{"s4", "alice", "eve", `{"quality":5}`, 1.0},
		{"s5", "alice", "frank", `{"quality":4}`, 1.0},
	}
	seedStamps(t, doltPath, dir, stamps)

	cfg := SpiderConfig{
		MinStampsForCollusion:   100, // suppress
		CollusionRatioThreshold: 1.1, // suppress
		RubberStampMinCount:     100, // suppress
		ConfidenceFloor:         0.95,
		ConfidenceMinStamps:     5,
	}

	signals, err := RunSpiderDetection(doltPath, dir, cfg)
	if err != nil {
		t.Fatalf("RunSpiderDetection: %v", err)
	}

	var found bool
	for _, s := range signals {
		if s.Kind == SignalConfidenceInflation {
			found = true
			if s.Score < 0.9 {
				t.Errorf("confidence inflation score should be high for avg=1.0, got %f", s.Score)
			}
		}
	}
	if !found {
		t.Fatal("expected confidence-inflation signal, got none")
	}
}

func TestSpiderIntegration_SelfLoopDetected(t *testing.T) {
	dir := initDoltRepo(t)
	doltPath, _ := exec.LookPath("dolt")

	// Alice and Bob stamp only each other — tight self-loop.
	stamps := []testStamp{
		{"s1", "alice", "bob", `{"quality":5}`, 0.9},
		{"s2", "alice", "bob", `{"quality":4}`, 0.8},
		{"s3", "bob", "alice", `{"quality":5}`, 0.9},
		{"s4", "bob", "alice", `{"quality":4}`, 0.8},
	}
	seedStamps(t, doltPath, dir, stamps)

	cfg := DefaultSpiderConfig()
	// Suppress other detectors to isolate self-loop
	cfg.MinStampsForCollusion = 100
	cfg.CollusionRatioThreshold = 1.1
	cfg.RubberStampMinCount = 100
	cfg.ConfidenceFloor = 1.1
	cfg.ConfidenceMinStamps = 100

	signals, err := RunSpiderDetection(doltPath, dir, cfg)
	if err != nil {
		t.Fatalf("RunSpiderDetection: %v", err)
	}

	var found bool
	for _, s := range signals {
		if s.Kind == SignalSelfLoop {
			found = true
			if s.Score < 0.8 {
				t.Errorf("self-loop score should be high for symmetric loop, got %f", s.Score)
			}
		}
	}
	if !found {
		t.Fatal("expected self-loop signal, got none")
	}
}

func TestSpiderIntegration_NoFraudInHealthyNetwork(t *testing.T) {
	dir := initDoltRepo(t)
	doltPath, _ := exec.LookPath("dolt")

	// Healthy network: diverse stamp patterns, varied confidence, varied valence.
	stamps := []testStamp{
		{"s1", "alice", "bob", `{"quality":4}`, 0.8},
		{"s2", "bob", "carol", `{"quality":3}`, 0.6},
		{"s3", "carol", "dave", `{"quality":5}`, 0.9},
		{"s4", "dave", "alice", `{"quality":4}`, 0.7},
		{"s5", "eve", "alice", `{"quality":3}`, 0.5},
		{"s6", "frank", "bob", `{"quality":4}`, 0.8},
		{"s7", "alice", "carol", `{"quality":2}`, 0.6},
		{"s8", "bob", "dave", `{"quality":5}`, 0.7},
	}
	seedStamps(t, doltPath, dir, stamps)

	cfg := DefaultSpiderConfig()
	signals, err := RunSpiderDetection(doltPath, dir, cfg)
	if err != nil {
		t.Fatalf("RunSpiderDetection: %v", err)
	}

	if len(signals) != 0 {
		for _, s := range signals {
			t.Errorf("unexpected signal in healthy network: %s (score=%.2f, rigs=%v)", s.Kind, s.Score, s.Rigs)
		}
	}
}

func TestSpiderIntegration_EmptyTable(t *testing.T) {
	dir := initDoltRepo(t)
	doltPath, _ := exec.LookPath("dolt")

	// No stamps at all — should return no signals and no error.
	cfg := DefaultSpiderConfig()
	signals, err := RunSpiderDetection(doltPath, dir, cfg)
	if err != nil {
		t.Fatalf("RunSpiderDetection on empty table: %v", err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals on empty table, got %d", len(signals))
	}
}

func TestSpiderIntegration_BadDoltPath(t *testing.T) {
	dir := t.TempDir()
	badPath := filepath.Join(dir, "nonexistent-dolt")

	cfg := DefaultSpiderConfig()
	signals, err := RunSpiderDetection(badPath, dir, cfg)
	if err == nil {
		t.Error("expected error with bad dolt path")
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals with bad dolt path, got %d", len(signals))
	}
}
