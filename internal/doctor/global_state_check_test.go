package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasShellIntegration_DirectMarker(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("# --- Gas Town Integration (managed by gt) ---\nsource hook.sh\n# --- End Gas Town ---\n"), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration to be detected with marker in RC file")
	}
}

func TestHasShellIntegration_NoMarker(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("# nothing here\n"), 0644)

	if hasShellIntegration(rc) {
		t.Error("expected no integration detected")
	}
}

func TestHasShellIntegration_MissingFile(t *testing.T) {
	if hasShellIntegration("/nonexistent/.zshrc") {
		t.Error("expected false for missing file")
	}
}

func TestHasShellIntegration_MarkerInSourcedFile(t *testing.T) {
	dir := t.TempDir()

	// Sourced file has the marker
	sub := filepath.Join(dir, "profile.zsh")
	os.WriteFile(sub, []byte("# --- Gas Town Integration (managed by gt) ---\n"), 0644)

	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("source "+sub+"\n"), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration found via sourced file")
	}
}

func TestHasShellIntegration_HookScriptReferenceInSourcedFile(t *testing.T) {
	// Simulates the user's actual setup: .zshrc -> profile.zsh -> shell-hook.sh
	dir := t.TempDir()

	profile := filepath.Join(dir, "home.zsh")
	os.WriteFile(profile, []byte(`# Gas Town shell integration
[[ -f "`+dir+`/shell-hook.sh" ]] && source "`+dir+`/shell-hook.sh"
`), 0644)

	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("source "+profile+"\n"), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration found via shell-hook.sh reference in sourced file")
	}
}

func TestHasShellIntegration_VariableExpansion(t *testing.T) {
	dir := t.TempDir()

	sub := filepath.Join(dir, "zsh", "common.zsh")
	os.MkdirAll(filepath.Dir(sub), 0755)
	os.WriteFile(sub, []byte("# --- Gas Town Integration (managed by gt) ---\n"), 0644)

	rc := filepath.Join(dir, ".zshrc")
	content := `export DOTFILES_DIR="` + dir + `"
source "$DOTFILES_DIR/zsh/common.zsh"
`
	os.WriteFile(rc, []byte(content), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration found after variable expansion")
	}
}

func TestHasShellIntegration_GlobFallback(t *testing.T) {
	dir := t.TempDir()

	// Create multiple profile files, one with the marker
	zshDir := filepath.Join(dir, "zsh")
	os.MkdirAll(zshDir, 0755)
	os.WriteFile(filepath.Join(zshDir, "work.zsh"), []byte("# work config\n"), 0644)
	os.WriteFile(filepath.Join(zshDir, "home.zsh"), []byte("[[ -f x/shell-hook.sh ]] && source x/shell-hook.sh\n"), 0644)

	// RC file uses unresolvable variable that triggers glob
	rc := filepath.Join(dir, ".zshrc")
	content := `export DOTFILES_DIR="` + dir + `"
source "$DOTFILES_DIR/zsh/$PROFILE.zsh"
`
	os.WriteFile(rc, []byte(content), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration found via glob fallback for unresolved variable")
	}
}

func TestHasShellIntegration_TildeExpansion(t *testing.T) {
	dir := t.TempDir()
	home, _ := os.UserHomeDir()

	// Create a file under the actual home dir with the marker
	testDir := filepath.Join(home, ".gastown-test-"+filepath.Base(dir))
	os.MkdirAll(testDir, 0755)
	defer os.RemoveAll(testDir)

	sub := filepath.Join(testDir, "integration.zsh")
	os.WriteFile(sub, []byte("# --- Gas Town Integration (managed by gt) ---\n"), 0644)

	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("source ~/"+filepath.Base(testDir)+"/integration.zsh\n"), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration found via tilde expansion")
	}
}

func TestHasShellIntegration_DepthLimit(t *testing.T) {
	dir := t.TempDir()

	// Create a chain deeper than the depth limit (5)
	var prev string
	for i := 0; i < 8; i++ {
		f := filepath.Join(dir, "level"+string(rune('0'+i))+".zsh")
		if i == 7 {
			os.WriteFile(f, []byte("# --- Gas Town Integration ---\n"), 0644)
		} else if prev == "" {
			os.WriteFile(f, []byte("# root\n"), 0644)
		} else {
			os.WriteFile(f, []byte("source "+prev+"\n"), 0644)
		}
		prev = f
	}

	// Build the chain in reverse: rc -> level7 -> level6 -> ... -> level0 (marker)
	// Actually, let me rebuild this correctly
	// level0 has marker, level1 sources level0, ..., level7 sources level6
	for i := 0; i < 8; i++ {
		f := filepath.Join(dir, "level"+string(rune('0'+i))+".zsh")
		if i == 0 {
			os.WriteFile(f, []byte("# --- Gas Town Integration ---\n"), 0644)
		} else {
			prev := filepath.Join(dir, "level"+string(rune('0'+i-1))+".zsh")
			os.WriteFile(f, []byte("source "+prev+"\n"), 0644)
		}
	}

	// RC -> level7 -> level6 -> ... -> level0 (marker) = depth 8
	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("source "+filepath.Join(dir, "level7.zsh")+"\n"), 0644)

	// Depth limit is 5, so level0 at depth 8 should not be reached
	if hasShellIntegration(rc) {
		t.Error("expected depth limit to prevent finding deeply nested marker")
	}

	// But if RC sources level4 -> depth 5 -> level0 is at depth 4 from level4's perspective
	// Actually: RC(0) -> level4(1) -> level3(2) -> level2(3) -> level1(4) -> level0(5) = found at depth 5
	rc2 := filepath.Join(dir, ".zshrc2")
	os.WriteFile(rc2, []byte("source "+filepath.Join(dir, "level4.zsh")+"\n"), 0644)

	if !hasShellIntegration(rc2) {
		t.Error("expected integration found within depth limit")
	}
}

func TestHasShellIntegration_CircularSourcePrevention(t *testing.T) {
	dir := t.TempDir()

	a := filepath.Join(dir, "a.zsh")
	b := filepath.Join(dir, "b.zsh")

	os.WriteFile(a, []byte("source "+b+"\n"), 0644)
	os.WriteFile(b, []byte("source "+a+"\n"), 0644)

	// Should not infinite loop; should return false
	if hasShellIntegration(a) {
		t.Error("expected false for circular source chain without marker")
	}
}

func TestHasShellIntegration_ConditionalOrSource(t *testing.T) {
	dir := t.TempDir()

	sub := filepath.Join(dir, "p10k.zsh")
	os.WriteFile(sub, []byte("# --- Gas Town Integration ---\n"), 0644)

	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("[[ ! -f "+sub+" ]] || source "+sub+"\n"), 0644)

	if !hasShellIntegration(rc) {
		t.Error("expected integration found via || source pattern")
	}
}

func TestExtractShellVars(t *testing.T) {
	content := `export FOO="bar"
BAZ="$HOME/stuff"
COMPLEX=$(echo hi)
export NESTED="${FOO}/sub"
SINGLE='single-quoted'
BACKTICK=` + "`echo hi`" + `
="value"
FOO2="digitvar"
`
	vars := extractShellVars(content, "/home/test")

	tests := []struct {
		name, want string
	}{
		{"HOME", "/home/test"},
		{"FOO", "bar"},
		{"BAZ", "/home/test/stuff"},
		{"NESTED", "bar/sub"},
		{"SINGLE", "single-quoted"},
		{"FOO2", "digitvar"},
	}
	for _, tt := range tests {
		got := vars[tt.name]
		if got != tt.want {
			t.Errorf("vars[%q] = %q, want %q", tt.name, got, tt.want)
		}
	}

	// COMPLEX should not be extracted (command substitution)
	if _, ok := vars["COMPLEX"]; ok {
		t.Error("expected COMPLEX to be skipped (command substitution)")
	}

	// BACKTICK should not be extracted (backtick command substitution)
	if _, ok := vars["BACKTICK"]; ok {
		t.Error("expected BACKTICK to be skipped (backtick command substitution)")
	}

	// Empty LHS should not produce a var
	if _, ok := vars[""]; ok {
		t.Error("expected empty var name to be skipped")
	}
}

func TestResolveSourcePaths(t *testing.T) {
	vars := map[string]string{
		"HOME": "/home/test",
		"DIR":  "/home/test/dotfiles",
	}

	tests := []struct {
		line string
		want []string
	}{
		{"source /abs/path.zsh", []string{"/abs/path.zsh"}},
		{". /abs/path.zsh", []string{"/abs/path.zsh"}},
		{`source "$DIR/foo.zsh"`, []string{"/home/test/dotfiles/foo.zsh"}},
		{"source ~/foo.zsh", []string{"/home/test/foo.zsh"}},
		{`[[ -f "$DIR/foo.zsh" ]] && source "$DIR/foo.zsh"`, []string{"/home/test/dotfiles/foo.zsh"}},
		{`[[ ! -f /x.zsh ]] || source /x.zsh`, []string{"/x.zsh"}},
		{`[[ -f /x.zsh ]] && . /x.zsh`, []string{"/x.zsh"}},
		{"# source /commented/out.zsh", nil},
		{"echo hello", nil},
		{"source /path.zsh  # inline comment", []string{"/path.zsh"}},
		{`source "$UNKNOWN/file[1].zsh"`, nil},
	}

	for _, tt := range tests {
		got := resolveSourcePaths(tt.line, "/home/test", vars)
		if len(got) != len(tt.want) {
			t.Errorf("resolveSourcePaths(%q) = %v, want %v", tt.line, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("resolveSourcePaths(%q)[%d] = %q, want %q", tt.line, i, got[i], tt.want[i])
			}
		}
	}
}

func TestReplaceUnresolvedVars(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/home/user/dotfiles/zsh/$PROFILE.zsh", "/home/user/dotfiles/zsh/*.zsh"},
		{"${HOME}/file", "*/file"},
		{"/no/vars/here", "/no/vars/here"},
		{"$A/$B/end", "*/*/end"},
		{"${BROKEN/file", "${BROKEN/file"},
	}
	for _, tt := range tests {
		got := replaceUnresolvedVars(tt.in)
		if got != tt.want {
			t.Errorf("replaceUnresolvedVars(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
