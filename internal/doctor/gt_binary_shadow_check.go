package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// GTBinaryShadowCheck verifies that a source-installed gt in ~/.local/bin is
// the one that plain `gt` resolves to on PATH.
type GTBinaryShadowCheck struct {
	BaseCheck
}

// NewGTBinaryShadowCheck creates a new gt binary shadowing check.
func NewGTBinaryShadowCheck() *GTBinaryShadowCheck {
	return &GTBinaryShadowCheck{
		BaseCheck: BaseCheck{
			CheckName:        "gt-binary-shadow",
			CheckDescription: "Check whether a canonical ~/.local/bin/gt install is shadowed on PATH",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *GTBinaryShadowCheck) Run(ctx *CheckContext) *CheckResult {
	info, err := detectGTBinaryShadow()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not inspect gt PATH shadowing",
			Details: []string{err.Error()},
		}
	}

	if !info.CanonicalExists {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("No canonical source install detected at %s", info.CanonicalDisplay),
		}
	}

	if !info.Shadowed {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("PATH resolves canonical gt install (%s)", info.CanonicalDisplay),
		}
	}

	message := fmt.Sprintf("PATH resolves gt to %s instead of canonical source install %s",
		info.PathDisplay, info.CanonicalDisplay)
	if info.PathResolved == "" {
		message = fmt.Sprintf("PATH does not resolve gt, but canonical source install exists at %s",
			info.CanonicalDisplay)
	} else if info.HomebrewShadow {
		message = fmt.Sprintf("Homebrew gt at %s shadows canonical source install %s",
			info.PathDisplay, info.CanonicalDisplay)
	}

	details := []string{
		fmt.Sprintf("PATH hit: %s", info.PathDisplay),
		fmt.Sprintf("Canonical source install: %s", info.CanonicalDisplay),
	}
	if info.RunningDisplay != "" && !sameExecutablePath(info.RunningResolved, info.PathResolved) {
		details = append(details, fmt.Sprintf("Current executable: %s", info.RunningDisplay))
	}

	fixHint := fmt.Sprintf("Run '%s shell install' or prepend 'export PATH=\"$HOME/.local/bin:$HOME/go/bin:$PATH\"' to your shell rc, then open a new shell.",
		info.CanonicalDisplay)
	if info.HomebrewShadow {
		fixHint += " If you want source builds to win, unlink or uninstall the Homebrew gt binary."
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: message,
		Details: details,
		FixHint: fixHint,
	}
}

type gtBinaryShadowInfo struct {
	CanonicalExists  bool
	CanonicalPath    string
	CanonicalDisplay string
	PathResolved     string
	PathDisplay      string
	RunningResolved  string
	RunningDisplay   string
	Shadowed         bool
	HomebrewShadow   bool
}

func detectGTBinaryShadow() (*gtBinaryShadowInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	canonicalBase := filepath.Join(home, ".local", "bin", "gt")
	canonicalPath, canonicalExists := findExistingExecutable(canonicalBase)
	pathResolved, err := exec.LookPath("gt")
	if err != nil {
		pathResolved = ""
	}
	runningResolved, err := os.Executable()
	if err != nil {
		runningResolved = ""
	}

	info := &gtBinaryShadowInfo{
		CanonicalExists:  canonicalExists,
		CanonicalPath:    canonicalPath,
		CanonicalDisplay: displayUserPath(firstNonEmpty(canonicalPath, canonicalBase)),
		PathResolved:     normalizeExecutablePath(pathResolved),
		PathDisplay:      displayLookPathResult(pathResolved),
		RunningResolved:  normalizeExecutablePath(runningResolved),
		RunningDisplay:   displayUserPath(normalizeExecutablePath(runningResolved)),
	}

	if canonicalExists {
		info.Shadowed = pathResolved == "" || !sameExecutablePath(info.CanonicalPath, info.PathResolved)
		info.HomebrewShadow = isLikelyHomebrewGT(info.PathResolved)
	}

	return info, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func displayLookPathResult(path string) string {
	if path == "" {
		return "not found"
	}
	return displayUserPath(normalizeExecutablePath(path))
}

func displayUserPath(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil {
		home = filepath.Clean(home)
		path = filepath.Clean(path)
		if path == home {
			return "~"
		}
		prefix := home + string(os.PathSeparator)
		if strings.HasPrefix(path, prefix) {
			return "~" + string(os.PathSeparator) + strings.TrimPrefix(path, prefix)
		}
	}
	return path
}

func isLikelyHomebrewGT(path string) bool {
	if path == "" {
		return false
	}
	clean := filepath.ToSlash(path)
	return clean == "/usr/local/bin/gt" ||
		clean == "/opt/homebrew/bin/gt" ||
		strings.HasPrefix(clean, "/usr/local/Cellar/") ||
		strings.HasPrefix(clean, "/opt/homebrew/Cellar/")
}

func normalizeExecutablePath(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func sameExecutablePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a = normalizeExecutablePath(a)
	b = normalizeExecutablePath(b)
	if a == b {
		return true
	}
	aInfo, aErr := os.Stat(a)
	bInfo, bErr := os.Stat(b)
	if aErr == nil && bErr == nil {
		return os.SameFile(aInfo, bInfo)
	}
	return false
}

func findExistingExecutable(basePath string) (string, bool) {
	for _, candidate := range executableCandidates(basePath) {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return normalizeExecutablePath(candidate), true
		}
	}
	return "", false
}

func executableCandidates(basePath string) []string {
	candidates := []string{basePath}
	if runtime.GOOS != "windows" {
		return candidates
	}

	seen := map[string]bool{basePath: true}
	exts := []string{".exe", ".bat", ".cmd"}
	if pathext := os.Getenv("PATHEXT"); pathext != "" {
		exts = append(strings.Split(strings.ToLower(pathext), ";"), exts...)
	}
	for _, ext := range exts {
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		candidate := basePath + ext
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}
	return candidates
}
