package version

import (
	"context"
	"debug/buildinfo"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const gastownGTMainPackage = "github.com/steveyegge/gastown/cmd/gt"

var (
	currentExecutablePath = os.Executable
	probeGTBinary         = defaultProbeGTBinary
	gtVersionLineRE       = regexp.MustCompile(`^gt version (\S+)(?: \(([^:()]+)(?:: ([^)]+))?\))?$`)
	commitSuffixRE        = regexp.MustCompile(`@([0-9a-f]{7,40})$`)
)

// GTBinaryVersionInfo captures the parsed version identity for a discovered gt binary.
type GTBinaryVersionInfo struct {
	MainPackage string
	VersionLine string
	Version     string
	Build       string
	Detail      string
	Commit      string
	Error       error
}

// Recognized reports whether the probed executable looks like a Gas Town gt binary.
func (i GTBinaryVersionInfo) Recognized() bool {
	return i.MainPackage == gastownGTMainPackage || i.Version != ""
}

// GTBinaryCandidate describes one discovered executable named gt.
type GTBinaryCandidate struct {
	Path         string
	ResolvedPath string
	OnPATH       bool
	PATHIndex    int
	Active       bool
	PathPrimary  bool
	VersionInfo  GTBinaryVersionInfo
	StaleInfo    *StaleBinaryInfo
}

// GTBinaryInventory is a structured view of gt executables discovered from PATH
// plus the currently running binary.
type GTBinaryInventory struct {
	Binaries         []GTBinaryCandidate
	ActiveIndex      int
	PathPrimaryIndex int
	RepoRoot         string
	RepoRootError    error
}

// Active returns the currently running gt binary, if known.
func (i *GTBinaryInventory) Active() *GTBinaryCandidate {
	if i == nil || i.ActiveIndex < 0 || i.ActiveIndex >= len(i.Binaries) {
		return nil
	}
	return &i.Binaries[i.ActiveIndex]
}

// PathPrimary returns the first gt binary found on PATH, if any.
func (i *GTBinaryInventory) PathPrimary() *GTBinaryCandidate {
	if i == nil || i.PathPrimaryIndex < 0 || i.PathPrimaryIndex >= len(i.Binaries) {
		return nil
	}
	return &i.Binaries[i.PathPrimaryIndex]
}

// DiscoverGTBinaries inventories executables named gt across PATH and the
// currently running binary, then probes each candidate for version metadata.
func DiscoverGTBinaries() *GTBinaryInventory {
	inv := &GTBinaryInventory{
		ActiveIndex:      -1,
		PathPrimaryIndex: -1,
	}

	indexByPath := make(map[string]int)
	addCandidate := func(path string, onPATH bool, pathIndex int, active bool) {
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		if idx, ok := indexByPath[cleaned]; ok {
			bin := &inv.Binaries[idx]
			bin.Active = bin.Active || active
			bin.OnPATH = bin.OnPATH || onPATH
			if onPATH && (bin.PATHIndex < 0 || pathIndex < bin.PATHIndex) {
				bin.PATHIndex = pathIndex
			}
			return
		}

		resolved := cleaned
		if realPath, err := filepath.EvalSymlinks(cleaned); err == nil && realPath != "" {
			resolved = realPath
		}

		candidate := GTBinaryCandidate{
			Path:         cleaned,
			ResolvedPath: resolved,
			OnPATH:       onPATH,
			PATHIndex:    -1,
			Active:       active,
		}
		if onPATH {
			candidate.PATHIndex = pathIndex
		}

		indexByPath[cleaned] = len(inv.Binaries)
		inv.Binaries = append(inv.Binaries, candidate)
	}

	if execPath, err := currentExecutablePath(); err == nil && execPath != "" {
		addCandidate(execPath, false, -1, true)
	}

	for idx, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		if candidate := findExecutableInDir(dir, "gt"); candidate != "" {
			addCandidate(candidate, true, idx, false)
		}
	}

	for idx := range inv.Binaries {
		if inv.Binaries[idx].Active {
			inv.ActiveIndex = idx
		}
		if inv.Binaries[idx].OnPATH && (inv.PathPrimaryIndex == -1 || inv.Binaries[idx].PATHIndex < inv.Binaries[inv.PathPrimaryIndex].PATHIndex) {
			inv.PathPrimaryIndex = idx
		}
	}
	if inv.PathPrimaryIndex >= 0 {
		inv.Binaries[inv.PathPrimaryIndex].PathPrimary = true
	}

	repoRoot, repoErr := GetRepoRoot()
	if repoErr == nil {
		inv.RepoRoot = repoRoot
	} else {
		inv.RepoRootError = repoErr
	}

	for idx := range inv.Binaries {
		probe := probeGTBinary(inv.Binaries[idx].Path)
		inv.Binaries[idx].VersionInfo = probe
		if repoRoot != "" && probe.Commit != "" {
			inv.Binaries[idx].StaleInfo = checkBinaryCommitAgainstRepo(repoRoot, probe.Commit)
		}
	}

	return inv
}

func findExecutableInDir(dir, base string) string {
	for _, name := range executableNames(base) {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || !isExecutable(info.Mode()) {
			continue
		}
		return candidate
	}
	return ""
}

func executableNames(base string) []string {
	if runtime.GOOS != "windows" {
		return []string{base}
	}

	pathext := os.Getenv("PATHEXT")
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}

	seen := map[string]bool{
		strings.ToLower(base): true,
	}
	names := []string{base}
	for _, ext := range strings.Split(pathext, ";") {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		name := base + ext
		lower := strings.ToLower(name)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		names = append(names, name)
	}
	return names
}

func isExecutable(mode os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return mode&0o111 != 0
}

func defaultProbeGTBinary(path string) GTBinaryVersionInfo {
	info := GTBinaryVersionInfo{}

	buildInfo, err := buildinfo.ReadFile(path)
	if err != nil {
		info.Error = fmt.Errorf("cannot read build info: %w", err)
		return info
	}
	info.MainPackage = buildInfo.Path
	if buildInfo.Path != gastownGTMainPackage {
		info.Error = fmt.Errorf("unexpected main package %q", buildInfo.Path)
		return info
	}

	line, err := runGTVersionLine(path)
	if err != nil {
		info.Error = err
		return info
	}
	info.VersionLine = line

	matches := gtVersionLineRE.FindStringSubmatch(line)
	if len(matches) == 0 {
		info.Error = fmt.Errorf("unrecognized version output: %q", line)
		return info
	}

	info.Version = matches[1]
	info.Build = matches[2]
	info.Detail = matches[3]

	if suffix := commitSuffixRE.FindStringSubmatch(info.Detail); len(suffix) == 2 {
		info.Commit = suffix[1]
	}

	return info
}

func runGTVersionLine(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "version")
	cmd.Env = minimalVersionProbeEnv()
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("timed out running 'version'")
	}
	if err != nil {
		return "", fmt.Errorf("'version' failed: %w", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gt version ") {
			return line, nil
		}
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "", fmt.Errorf("no version output")
	}
	return "", fmt.Errorf("no parsable version line in output: %q", trimmed)
}

func minimalVersionProbeEnv() []string {
	keys := map[string]bool{
		"HOME":       true,
		"PATH":       true,
		"USER":       true,
		"LOGNAME":    true,
		"SHELL":      true,
		"TMPDIR":     true,
		"TEMP":       true,
		"TMP":        true,
		"SystemRoot": true,
		"COMSPEC":    true,
	}

	env := []string{"NO_COLOR=1", "GT_STALE_WARNED=1"}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || !keys[parts[0]] {
			continue
		}
		env = append(env, entry)
	}
	return env
}
