package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestNewBeadsDatabaseCheck(t *testing.T) {
	check := NewBeadsDatabaseCheck()

	if check.Name() != "beads-database" {
		t.Errorf("expected name 'beads-database', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBeadsDatabaseCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_NoDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty database
	dbPath := filepath.Join(beadsDir, "issues.db")
	if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	// Create JSONL with content
	jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(`{"id":"test-1","title":"Test"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for empty db with content in jsonl, got %v", result.Status)
	}
}

func TestBeadsDatabaseCheck_PopulatedDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create database with content
	dbPath := filepath.Join(beadsDir, "issues.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsDatabaseCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for populated db, got %v", result.Status)
	}
}

func TestNewPrefixMismatchCheck(t *testing.T) {
	check := NewPrefixMismatchCheck()

	if check.Name() != "prefix-mismatch" {
		t.Errorf("expected name 'prefix-mismatch', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestPrefixMismatchCheck_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for no routes, got %v", result.Status)
	}
}

func TestPrefixMismatchCheck_NoRigsJson(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rigs.json, got %v", result.Status)
	}
}

func TestPrefixMismatchCheck_Matching(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with gt- prefix
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json with matching gt prefix
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsContent := `{
		"version": 1,
		"rigs": {
			"gastown": {
				"git_url": "https://github.com/example/gastown",
				"beads": {
					"prefix": "gt"
				}
			}
		}
	}`
	if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for matching prefixes, got %v: %s", result.Status, result.Message)
	}
}

func TestPrefixMismatchCheck_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with gt- prefix
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json with WRONG prefix (ga instead of gt)
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsContent := `{
		"version": 1,
		"rigs": {
			"gastown": {
				"git_url": "https://github.com/example/gastown",
				"beads": {
					"prefix": "ga"
				}
			}
		}
	}`
	if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for prefix mismatch, got %v: %s", result.Status, result.Message)
	}

	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d", len(result.Details))
	}
}

func TestPrefixMismatchCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with gt- prefix
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rigs.json with WRONG prefix (ga instead of gt)
	rigsPath := filepath.Join(mayorDir, "rigs.json")
	rigsContent := `{
		"version": 1,
		"rigs": {
			"gastown": {
				"git_url": "https://github.com/example/gastown",
				"beads": {
					"prefix": "ga"
				}
			}
		}
	}`
	if err := os.WriteFile(rigsPath, []byte(rigsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewPrefixMismatchCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// First verify there's a mismatch
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected mismatch before fix, got %v", result.Status)
	}

	// Fix it
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify it's now fixed
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}

	// Verify rigs.json was updated
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadRigsConfig(rigsPath)
	if err != nil {
		t.Fatalf("failed to load fixed rigs.json: %v (content: %s)", err, data)
	}
	if cfg.Rigs["gastown"].BeadsConfig.Prefix != "gt" {
		t.Errorf("expected prefix 'gt' after fix, got %q", cfg.Rigs["gastown"].BeadsConfig.Prefix)
	}
}

func TestNewRoleLabelCheck(t *testing.T) {
	check := NewRoleLabelCheck()

	if check.Name() != "role-bead-labels" {
		t.Errorf("expected name 'role-bead-labels', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestRoleLabelCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Inject empty mock to skip exec.LookPath("bd") check
	// (bd may not be installed on all CI platforms like Windows)
	mock := &mockBeadShower{beads: map[string]*beads.Issue{}}

	check := NewRoleLabelCheck()
	check.beadShower = mock
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no .beads dir, got %v", result.Status)
	}
	if result.Message != "No beads database (skipped)" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

// mockBeadShower implements beadShower for testing
type mockBeadShower struct {
	beads map[string]*beads.Issue
}

func (m *mockBeadShower) Show(id string) (*beads.Issue, error) {
	if issue, ok := m.beads[id]; ok {
		return issue, nil
	}
	return nil, beads.ErrNotFound
}

// mockLabelAdder implements labelAdder for testing
type mockLabelAdder struct {
	calls []labelAddCall
}

type labelAddCall struct {
	townRoot string
	id       string
	label    string
}

func (m *mockLabelAdder) AddLabel(townRoot, id, label string) error {
	m.calls = append(m.calls, labelAddCall{townRoot, id, label})
	return nil
}

func TestRoleLabelCheck_AllBeadsHaveLabel(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock with all role beads having gt:role label
	mock := &mockBeadShower{
		beads: map[string]*beads.Issue{
			"hq-mayor-role":    {ID: "hq-mayor-role", Labels: []string{"gt:role"}},
			"hq-deacon-role":   {ID: "hq-deacon-role", Labels: []string{"gt:role"}},
			"hq-dog-role":      {ID: "hq-dog-role", Labels: []string{"gt:role"}},
			"hq-witness-role":  {ID: "hq-witness-role", Labels: []string{"gt:role"}},
			"hq-refinery-role": {ID: "hq-refinery-role", Labels: []string{"gt:role"}},
			"hq-polecat-role":  {ID: "hq-polecat-role", Labels: []string{"gt:role"}},
			"hq-crew-role":     {ID: "hq-crew-role", Labels: []string{"gt:role"}},
		},
	}

	check := NewRoleLabelCheck()
	check.beadShower = mock
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when all beads have label, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "All role beads have gt:role label" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRoleLabelCheck_MissingLabel(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock with witness-role missing the gt:role label (the regression case)
	mock := &mockBeadShower{
		beads: map[string]*beads.Issue{
			"hq-mayor-role":    {ID: "hq-mayor-role", Labels: []string{"gt:role"}},
			"hq-deacon-role":   {ID: "hq-deacon-role", Labels: []string{"gt:role"}},
			"hq-dog-role":      {ID: "hq-dog-role", Labels: []string{"gt:role"}},
			"hq-witness-role":  {ID: "hq-witness-role", Labels: []string{}}, // Missing gt:role!
			"hq-refinery-role": {ID: "hq-refinery-role", Labels: []string{"gt:role"}},
			"hq-polecat-role":  {ID: "hq-polecat-role", Labels: []string{"gt:role"}},
			"hq-crew-role":     {ID: "hq-crew-role", Labels: []string{"gt:role"}},
		},
	}

	check := NewRoleLabelCheck()
	check.beadShower = mock
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning when label missing, got %v", result.Status)
	}
	if result.Message != "1 role bead(s) missing gt:role label" {
		t.Errorf("unexpected message: %s", result.Message)
	}
	if len(result.Details) != 1 || result.Details[0] != "hq-witness-role" {
		t.Errorf("expected details to contain hq-witness-role, got %v", result.Details)
	}
}

func TestRoleLabelCheck_MultipleMissingLabels(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock with multiple beads missing the gt:role label
	mock := &mockBeadShower{
		beads: map[string]*beads.Issue{
			"hq-mayor-role":    {ID: "hq-mayor-role", Labels: []string{}},    // Missing
			"hq-deacon-role":   {ID: "hq-deacon-role", Labels: []string{}},   // Missing
			"hq-dog-role":      {ID: "hq-dog-role", Labels: []string{"gt:role"}},
			"hq-witness-role":  {ID: "hq-witness-role", Labels: []string{}},  // Missing
			"hq-refinery-role": {ID: "hq-refinery-role", Labels: []string{}}, // Missing
			"hq-polecat-role":  {ID: "hq-polecat-role", Labels: []string{"gt:role"}},
			"hq-crew-role":     {ID: "hq-crew-role", Labels: []string{"gt:role"}},
		},
	}

	check := NewRoleLabelCheck()
	check.beadShower = mock
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning, got %v", result.Status)
	}
	if result.Message != "4 role bead(s) missing gt:role label" {
		t.Errorf("unexpected message: %s", result.Message)
	}
	if len(result.Details) != 4 {
		t.Errorf("expected 4 details, got %d: %v", len(result.Details), result.Details)
	}
}

func TestRoleLabelCheck_BeadNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock with only some beads existing (others return ErrNotFound)
	mock := &mockBeadShower{
		beads: map[string]*beads.Issue{
			"hq-mayor-role":  {ID: "hq-mayor-role", Labels: []string{"gt:role"}},
			"hq-deacon-role": {ID: "hq-deacon-role", Labels: []string{"gt:role"}},
			// Other beads don't exist - should be skipped, not reported as errors
		},
	}

	check := NewRoleLabelCheck()
	check.beadShower = mock
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should be OK - missing beads are not an error (install will create them)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when beads don't exist, got %v: %s", result.Status, result.Message)
	}
}

func TestRoleLabelCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock with witness-role missing the label
	mockShower := &mockBeadShower{
		beads: map[string]*beads.Issue{
			"hq-mayor-role":   {ID: "hq-mayor-role", Labels: []string{"gt:role"}},
			"hq-witness-role": {ID: "hq-witness-role", Labels: []string{}}, // Missing gt:role
		},
	}
	mockAdder := &mockLabelAdder{}

	check := NewRoleLabelCheck()
	check.beadShower = mockShower
	check.labelAdder = mockAdder
	ctx := &CheckContext{TownRoot: tmpDir}

	// First run to detect the issue
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v", result.Status)
	}

	// Now fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify the correct bd label add command was called
	if len(mockAdder.calls) != 1 {
		t.Fatalf("expected 1 AddLabel call, got %d", len(mockAdder.calls))
	}
	call := mockAdder.calls[0]
	if call.townRoot != tmpDir {
		t.Errorf("expected townRoot %q, got %q", tmpDir, call.townRoot)
	}
	if call.id != "hq-witness-role" {
		t.Errorf("expected id 'hq-witness-role', got %q", call.id)
	}
	if call.label != "gt:role" {
		t.Errorf("expected label 'gt:role', got %q", call.label)
	}
}

func TestRoleLabelCheck_FixMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock with multiple beads missing the label
	mockShower := &mockBeadShower{
		beads: map[string]*beads.Issue{
			"hq-mayor-role":    {ID: "hq-mayor-role", Labels: []string{}},    // Missing
			"hq-deacon-role":   {ID: "hq-deacon-role", Labels: []string{"gt:role"}},
			"hq-witness-role":  {ID: "hq-witness-role", Labels: []string{}},  // Missing
			"hq-refinery-role": {ID: "hq-refinery-role", Labels: []string{}}, // Missing
		},
	}
	mockAdder := &mockLabelAdder{}

	check := NewRoleLabelCheck()
	check.beadShower = mockShower
	check.labelAdder = mockAdder
	ctx := &CheckContext{TownRoot: tmpDir}

	// First run to detect the issues
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v", result.Status)
	}
	if len(result.Details) != 3 {
		t.Fatalf("expected 3 missing, got %d", len(result.Details))
	}

	// Now fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix() failed: %v", err)
	}

	// Verify all 3 beads got the label added
	if len(mockAdder.calls) != 3 {
		t.Fatalf("expected 3 AddLabel calls, got %d", len(mockAdder.calls))
	}

	// Verify each call has the correct label
	for _, call := range mockAdder.calls {
		if call.label != "gt:role" {
			t.Errorf("expected label 'gt:role', got %q", call.label)
		}
	}
}

func TestNewDatabasePrefixCheck(t *testing.T) {
	check := NewDatabasePrefixCheck()

	if check.Name() != "database-prefix" {
		t.Errorf("expected name 'database-prefix', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestDatabasePrefixCheck_NoRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewDatabasePrefixCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for no routes, got %v", result.Status)
	}
}

func TestDatabasePrefixCheck_EmptyRoutes(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty routes.jsonl
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	if err := os.WriteFile(routesPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewDatabasePrefixCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for empty routes, got %v", result.Status)
	}
}

func TestDatabasePrefixCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with a route to a non-existent beads directory
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	routesContent := `{"prefix":"gt-","path":"gastown/mayor/rig"}`
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewDatabasePrefixCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should be OK - no beads dir for the rig is fine
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when rig beads dir doesn't exist, got %v", result.Status)
	}
}
