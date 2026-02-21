package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// ===================================================================
// BD_BRANCH Architecture Test — Callsite Registry (#1796)
//
// Scans internal/cmd/ for BD_BRANCH-relevant callsites:
//   - beads.New()            — Go wrapper constructors
//   - beads.NewWithBeadsDir() — Go wrapper with explicit beads dir
//   - exec.Command("bd",...)  — direct bd subprocess invocations
//   - exec.LookPath("bd")     — proxy for syscall.Exec bd invocations
//
// When a callsite is added or removed, this test fails — forcing the
// developer to classify it for BD_BRANCH safety.
//
// For polecat read paths:  add .OnMain() or StripBdBranch()
// For write/non-polecat:   update the expected count in the registry
// ===================================================================

// bdCallsiteCounts tracks BD_BRANCH-relevant patterns in a Go source file.
type bdCallsiteCounts struct {
	BeadsNew          int // beads.New( calls
	NewWithBeadsDir   int // beads.NewWithBeadsDir( calls
	OnMain            int // .OnMain() calls (0-arg method)
	StripBdBranch     int // beads.StripBdBranch( calls
	ExecCommandBd     int // exec.Command("bd", ...) calls
	LookPathBd        int // exec.LookPath("bd") calls — proxy for syscall.Exec bd invocations
}

// countBdCallsites walks a Go AST and counts BD_BRANCH-relevant call patterns.
//
// Limitations:
//   - Only detects default import name "beads", not aliased imports
//   - Only detects "bd" as double-quoted string literal, not backtick or variable
//   - Does not detect indirect bd invocations via helper functions that wrap bd calls
func countBdCallsites(file *ast.File) *bdCallsiteCounts {
	counts := &bdCallsiteCounts{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if ident, ok := sel.X.(*ast.Ident); ok {
			switch {
			case ident.Name == "beads" && sel.Sel.Name == "New":
				counts.BeadsNew++
			case ident.Name == "beads" && sel.Sel.Name == "NewWithBeadsDir":
				counts.NewWithBeadsDir++
			case ident.Name == "beads" && sel.Sel.Name == "StripBdBranch":
				counts.StripBdBranch++
			case ident.Name == "exec" && sel.Sel.Name == "Command":
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok &&
						lit.Kind == token.STRING && lit.Value == `"bd"` {
						counts.ExecCommandBd++
					}
				}
			case ident.Name == "exec" && sel.Sel.Name == "LookPath":
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok &&
						lit.Kind == token.STRING && lit.Value == `"bd"` {
						counts.LookPathBd++
					}
				}
			}
		}

		if sel.Sel.Name == "OnMain" && len(call.Args) == 0 {
			counts.OnMain++
		}

		return true
	})
	return counts
}

// parseBdTestSource parses a Go source string into an AST for scanner tests.
func parseBdTestSource(t *testing.T, src string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}
	return f
}

// bdArchTestDir returns the directory containing this test file (internal/cmd/).
func bdArchTestDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	return filepath.Dir(filename)
}

// scanCmdDir parses all non-test Go files in dir and returns per-file counts.
func scanCmdDir(t *testing.T, dir string) map[string]*bdCallsiteCounts {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("failed to parse directory %s: %v", dir, err)
	}

	result := make(map[string]*bdCallsiteCounts)
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			basename := filepath.Base(filename)
			counts := countBdCallsites(file)
			if counts.BeadsNew > 0 || counts.NewWithBeadsDir > 0 || counts.ExecCommandBd > 0 || counts.LookPathBd > 0 {
				result[basename] = counts
			}
		}
	}
	return result
}

// ===================================================================
// Unit tests: scanner verification (happy path)
// ===================================================================

func TestCountBdCallsites_BeadsNew(t *testing.T) {
	src := `package foo
import "beads"
func bar() {
	b := beads.New(dir)
	c := beads.New(otherDir)
	_ = b; _ = c
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 2 {
		t.Errorf("BeadsNew = %d, want 2", counts.BeadsNew)
	}
}

func TestCountBdCallsites_ChainedOnMain(t *testing.T) {
	src := `package foo
import "beads"
func bar() {
	b := beads.New(dir).OnMain()
	_ = b
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 1 {
		t.Errorf("BeadsNew = %d, want 1", counts.BeadsNew)
	}
	if counts.OnMain != 1 {
		t.Errorf("OnMain = %d, want 1", counts.OnMain)
	}
}

func TestCountBdCallsites_StripBdBranch(t *testing.T) {
	src := `package foo
import "beads"
func bar() {
	cmd.Env = beads.StripBdBranch(os.Environ())
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.StripBdBranch != 1 {
		t.Errorf("StripBdBranch = %d, want 1", counts.StripBdBranch)
	}
}

func TestCountBdCallsites_ExecCommandBd(t *testing.T) {
	src := `package foo
import "os/exec"
func bar() {
	cmd := exec.Command("bd", "show", id)
	_ = cmd
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.ExecCommandBd != 1 {
		t.Errorf("ExecCommandBd = %d, want 1", counts.ExecCommandBd)
	}
}

func TestCountBdCallsites_ComplexScenario(t *testing.T) {
	src := `package foo
import (
	"beads"
	"os/exec"
)
func findWork() {
	b := beads.New(dir).OnMain()
	ab := beads.New(agentDir).OnMain()
	hb := beads.New(hookDir).OnMain()
	_ = b; _ = ab; _ = hb
}
func runPrime() {
	cmd := exec.Command("bd", "prime")
	cmd.Env = beads.StripBdBranch(os.Environ())
	_ = cmd
}
func writeStuff() {
	b := beads.New(dir)
	_ = b
	cmd := exec.Command("bd", "update", id)
	_ = cmd
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 4 {
		t.Errorf("BeadsNew = %d, want 4", counts.BeadsNew)
	}
	if counts.OnMain != 3 {
		t.Errorf("OnMain = %d, want 3", counts.OnMain)
	}
	if counts.StripBdBranch != 1 {
		t.Errorf("StripBdBranch = %d, want 1", counts.StripBdBranch)
	}
	if counts.ExecCommandBd != 2 {
		t.Errorf("ExecCommandBd = %d, want 2", counts.ExecCommandBd)
	}
}

func TestCountBdCallsites_MultipleOnMainChains(t *testing.T) {
	src := `package foo
import "beads"
func bar() {
	a := beads.New(d1).OnMain()
	b := beads.New(d2).OnMain()
	c := beads.New(d3).OnMain()
	_ = a; _ = b; _ = c
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 3 {
		t.Errorf("BeadsNew = %d, want 3", counts.BeadsNew)
	}
	if counts.OnMain != 3 {
		t.Errorf("OnMain = %d, want 3", counts.OnMain)
	}
}

func TestCountBdCallsites_NewWithBeadsDir(t *testing.T) {
	src := `package foo
import "beads"
func bar() {
	bd := beads.NewWithBeadsDir(townRoot, beadsDir)
	_ = bd
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.NewWithBeadsDir != 1 {
		t.Errorf("NewWithBeadsDir = %d, want 1", counts.NewWithBeadsDir)
	}
	if counts.BeadsNew != 0 {
		t.Errorf("BeadsNew = %d, want 0 (NewWithBeadsDir is separate)", counts.BeadsNew)
	}
}

func TestCountBdCallsites_LookPathBd(t *testing.T) {
	src := `package foo
import "os/exec"
func bar() {
	bdPath, _ := exec.LookPath("bd")
	_ = bdPath
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.LookPathBd != 1 {
		t.Errorf("LookPathBd = %d, want 1", counts.LookPathBd)
	}
	if counts.ExecCommandBd != 0 {
		t.Errorf("ExecCommandBd = %d, want 0 (LookPath is separate from Command)", counts.ExecCommandBd)
	}
}

func TestCountBdCallsites_MixedNewAndNewWithBeadsDir(t *testing.T) {
	src := `package foo
import "beads"
func bar() {
	a := beads.New(dir)
	b := beads.NewWithBeadsDir(townRoot, beadsDir)
	c := beads.New(otherDir).OnMain()
	d := beads.NewWithBeadsDir(townRoot, beadsDir2)
	_ = a; _ = b; _ = c; _ = d
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 2 {
		t.Errorf("BeadsNew = %d, want 2", counts.BeadsNew)
	}
	if counts.NewWithBeadsDir != 2 {
		t.Errorf("NewWithBeadsDir = %d, want 2", counts.NewWithBeadsDir)
	}
	if counts.OnMain != 1 {
		t.Errorf("OnMain = %d, want 1", counts.OnMain)
	}
}

func TestCountBdCallsites_EmptyFile(t *testing.T) {
	src := `package foo`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 0 || counts.NewWithBeadsDir != 0 || counts.OnMain != 0 || counts.StripBdBranch != 0 || counts.ExecCommandBd != 0 || counts.LookPathBd != 0 {
		t.Errorf("expected all zeros, got %+v", counts)
	}
}

// ===================================================================
// Negative tests: patterns the scanner must NOT match
// ===================================================================

func TestCountBdCallsites_IgnoresNewIsolated(t *testing.T) {
	src := `package foo
import "beads"
func bar() { b := beads.NewIsolated(dir); _ = b }`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 0 {
		t.Errorf("BeadsNew = %d, want 0 (NewIsolated != New)", counts.BeadsNew)
	}
}

func TestCountBdCallsites_IgnoresNewWithBeadsDir(t *testing.T) {
	src := `package foo
import "beads"
func bar() { b := beads.NewWithBeadsDir(dir, bd); _ = b }`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.BeadsNew != 0 {
		t.Errorf("BeadsNew = %d, want 0 (NewWithBeadsDir != New)", counts.BeadsNew)
	}
}

func TestCountBdCallsites_IgnoresOnMainWithArgs(t *testing.T) {
	src := `package foo
func bar() { b := something.OnMain(true); _ = b }`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.OnMain != 0 {
		t.Errorf("OnMain = %d, want 0 (OnMain with args should not count)", counts.OnMain)
	}
}

func TestCountBdCallsites_IgnoresNonBdCommand(t *testing.T) {
	src := `package foo
import "os/exec"
func bar() {
	a := exec.Command("git", "status")
	b := exec.Command("dolt", "sql")
	_ = a; _ = b
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.ExecCommandBd != 0 {
		t.Errorf("ExecCommandBd = %d, want 0", counts.ExecCommandBd)
	}
}

func TestCountBdCallsites_IgnoresBdVariable(t *testing.T) {
	// exec.Command with "bd" in a variable, not a string literal
	src := `package foo
import "os/exec"
func bar() { prog := "bd"; cmd := exec.Command(prog, "show"); _ = cmd }`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.ExecCommandBd != 0 {
		t.Errorf("ExecCommandBd = %d, want 0 (variable arg, not literal)", counts.ExecCommandBd)
	}
}

func TestCountBdCallsites_IgnoresLookPathNonBd(t *testing.T) {
	src := `package foo
import "os/exec"
func bar() {
	a, _ := exec.LookPath("git")
	b, _ := exec.LookPath("tmux")
	_ = a; _ = b
}`
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.LookPathBd != 0 {
		t.Errorf("LookPathBd = %d, want 0 (non-bd LookPath)", counts.LookPathBd)
	}
}

func TestCountBdCallsites_IgnoresBdBacktick(t *testing.T) {
	src := "package foo\nimport \"os/exec\"\nfunc bar() { cmd := exec.Command(`bd`, \"show\"); _ = cmd }"
	counts := countBdCallsites(parseBdTestSource(t, src))
	if counts.ExecCommandBd != 0 {
		t.Errorf("ExecCommandBd = %d, want 0 (backtick string, not double-quoted)", counts.ExecCommandBd)
	}
}

// ===================================================================
// Architecture test: callsite registry
//
// Every beads.New() and exec.Command("bd",...) callsite in internal/cmd/
// must be registered here. When the count changes, the developer must
// review the new callsite for BD_BRANCH safety (#1796).
// ===================================================================

// expectedBeadsNewCounts maps filename → expected beads.New() call count.
// Update this map when adding or removing beads.New() calls.
var expectedBeadsNewCounts = map[string]int{
	"audit.go":                     1,
	"callbacks.go":                 1,
	"checkpoint_cmd.go":            2,
	"compact.go":                   1,
	"compact_report.go":            1,
	"crew_add.go":                  1,
	"dnd.go":                       1,
	"dog.go":                       3,
	"done.go":                      10,
	"escalate_impl.go":             6,
	"feed.go":                      1,
	"hook.go":                      3,
	"install.go":                   1,
	"mail_channel.go":              7,
	"mail_directory.go":            1,
	"mail_group.go":                6,
	"mail_send.go":                 1,
	"molecule_attach.go":           3,
	"molecule_attach_from_mail.go": 1,
	"molecule_dag.go":              1,
	"molecule_lifecycle.go":        2,
	"molecule_status.go":           6,
	"molecule_step.go":             2,
	"mq_integration.go":            3,
	"mq_list.go":                   1,
	"mq_next.go":                   1,
	"mq_status.go":                 1,
	"mq_submit.go":                 1,
	"notify.go":                    1,
	"nudge.go":                     1,
	"polecat.go":                   2,
	"polecat_helpers.go":           2,
	"polecat_identity.go":          6,
	"polecat_spawn.go":             2,
	"prime.go":                     3,
	"prime_molecule.go":            1,
	"prime_output.go":              2,
	"prime_session.go":             3,
	"ready.go":                     2,
	"refinery.go":                  1,
	"release.go":                   1,
	"rig.go":                       3,
	"rig_dock.go":                  3,
	"signal_stop.go":               1,
	"sling_convoy.go":              1,
	"sling_helpers.go":             3,
	"status.go":                    4,
	"statusline.go":                2,
	"unsling.go":                   3,
	"up.go":                        1,
}

// expectedNewWithBeadsDirCounts maps filename → expected beads.NewWithBeadsDir() call count.
// NewWithBeadsDir creates Beads wrappers with explicit beads directory — these also
// inherit BD_BRANCH and need the same safety review as beads.New().
var expectedNewWithBeadsDirCounts = map[string]int{
	"compact.go":    2,
	"mail_queue.go": 5,
	"rig.go":        1,
	"rig_config.go": 2,
}

// expectedLookPathBdCounts maps filename → expected exec.LookPath("bd") call count.
// LookPath("bd") typically precedes syscall.Exec — a bd invocation pattern invisible
// to exec.Command tracking. Each callsite needs BD_BRANCH safety review.
var expectedLookPathBdCounts = map[string]int{
	"init.go": 1,
	"show.go": 1,
}

// expectedExecBdCounts maps filename → expected exec.Command("bd",...) call count.
var expectedExecBdCounts = map[string]int{
	"agent_state.go":         2,
	"bead.go":                4,
	"boot.go":                3,
	"cat.go":                 1,
	"close.go":               1,
	"compact_report.go":      3,
	"convoy.go":              24,
	"costs.go":               9,
	"crew_lifecycle.go":      4,
	"deacon.go":              2,
	"dolt.go":                1,
	"formula.go":             9,
	"handoff.go":             6,
	"hook.go":                2,
	"init.go":                1,
	"install.go":             6,
	"mail_announce.go":       1,
	"mail_channel.go":        1,
	"mail_queue.go":          4,
	"migrate_bead_labels.go": 2,
	"molecule_await_signal.go": 4,
	"molecule_step.go":       3,
	"patrol.go":              5,
	"patrol_helpers.go":      4,
	"polecat.go":             1,
	"polecat_identity.go":    1,
	"prime.go":               3,
	"prime_molecule.go":      1,
	"rig.go":                 1,
	"sling.go":               3,
	"sling_convoy.go":        7,
	"sling_formula.go":       4,
	"sling_helpers.go":       9,
	"swarm.go":               16,
	"synthesis.go":           5,
}

func TestBdBranchCallsiteRegistry(t *testing.T) {
	dir := bdArchTestDir(t)
	actual := scanCmdDir(t, dir)

	// Check beads.New() counts
	checkRegistry(t, "beads.New()", actual, expectedBeadsNewCounts,
		func(c *bdCallsiteCounts) int { return c.BeadsNew })

	// Check beads.NewWithBeadsDir() counts
	checkRegistry(t, "beads.NewWithBeadsDir()", actual, expectedNewWithBeadsDirCounts,
		func(c *bdCallsiteCounts) int { return c.NewWithBeadsDir })

	// Check exec.Command("bd",...) counts
	checkRegistry(t, `exec.Command("bd",...)`, actual, expectedExecBdCounts,
		func(c *bdCallsiteCounts) int { return c.ExecCommandBd })

	// Check exec.LookPath("bd") counts — proxy for syscall.Exec bd invocations
	checkRegistry(t, `exec.LookPath("bd")`, actual, expectedLookPathBdCounts,
		func(c *bdCallsiteCounts) int { return c.LookPathBd })
}

func checkRegistry(t *testing.T, label string, actual map[string]*bdCallsiteCounts, expected map[string]int, getter func(*bdCallsiteCounts) int) {
	t.Helper()

	// Check counts match for registered files
	for file, want := range expected {
		counts, ok := actual[file]
		got := 0
		if ok {
			got = getter(counts)
		}
		if got != want {
			t.Errorf("%s: %s count changed: got %d, want %d — review for BD_BRANCH protection (#1796)",
				file, label, got, want)
		}
	}

	// Check for unregistered files
	unregistered := make(map[string]int)
	for file, counts := range actual {
		count := getter(counts)
		if count > 0 {
			if _, ok := expected[file]; !ok {
				unregistered[file] = count
			}
		}
	}
	if len(unregistered) > 0 {
		files := make([]string, 0, len(unregistered))
		for f := range unregistered {
			files = append(files, f)
		}
		sort.Strings(files)
		for _, f := range files {
			t.Errorf("%s: %d unregistered %s calls — add to registry", f, unregistered[f], label)
		}
	}
}

// ===================================================================
// Architecture test: protection coverage
//
// Verifies that polecat-context files maintain their BD_BRANCH
// protection patterns. If OnMain or StripBdBranch counts decrease,
// protection was removed.
//
// Scope: Only monitors files that were identified in W-006 (#1796) as
// needing BD_BRANCH protection. The registry test above catches NEW
// callsites in any file; this test ensures EXISTING protections are
// not removed. Files are added here when they receive OnMain/StripBdBranch
// fixes.
// ===================================================================

func TestBdBranchProtectionCoverage(t *testing.T) {
	dir := bdArchTestDir(t)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("failed to parse directory: %v", err)
	}

	fileCounts := make(map[string]*bdCallsiteCounts)
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			fileCounts[filepath.Base(filename)] = countBdCallsites(file)
		}
	}

	tests := []struct {
		file        string
		wantOnMain  int
		wantStripBd int
	}{
		{"prime.go", 3, 3},
		{"prime_session.go", 3, 0},
		{"hook.go", 2, 0},
		{"statusline.go", 1, 0},
		{"molecule_status.go", 1, 0},
		{"sling_helpers.go", 0, 3},
		{"show.go", 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			counts, ok := fileCounts[tt.file]
			if !ok {
				t.Fatalf("file %s not found", tt.file)
			}
			if counts.OnMain != tt.wantOnMain {
				t.Errorf("OnMain() count = %d, want %d — BD_BRANCH protection removed (#1796)",
					counts.OnMain, tt.wantOnMain)
			}
			if counts.StripBdBranch != tt.wantStripBd {
				t.Errorf("StripBdBranch() count = %d, want %d — BD_BRANCH protection removed (#1796)",
					counts.StripBdBranch, tt.wantStripBd)
			}
		})
	}
}
