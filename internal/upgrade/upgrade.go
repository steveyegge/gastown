package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GitHubAPIURL is the endpoint for fetching latest release info.
// Can be overridden with GT_UPGRADE_URL env var for testing.
var GitHubAPIURL = getUpgradeURL()

func getUpgradeURL() string {
	if url := os.Getenv("GT_UPGRADE_URL"); url != "" {
		return url
	}
	return "https://api.github.com/repos/steveyegge/gastown/releases/latest"
}

const (
	// UserAgent is sent with GitHub API requests.
	UserAgent = "gt-upgrade/1.0"

	// HTTPTimeout is the timeout for HTTP requests.
	HTTPTimeout = 30 * time.Second

	// MaxBinarySize is the maximum size for extracted binaries (100MB).
	// This prevents decompression bomb attacks.
	MaxBinarySize = 100 * 1024 * 1024

	// MaxAPIResponseSize is the maximum size for API responses (1MB).
	// This prevents memory exhaustion from malicious servers.
	MaxAPIResponseSize = 1 * 1024 * 1024

	// MaxErrorBodySize is the maximum size for error response bodies (4KB).
	MaxErrorBodySize = 4 * 1024
)

// ReleaseInfo contains information about a GitHub release.
type ReleaseInfo struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	HTMLURL    string  `json:"html_url"`
	Assets     []Asset `json:"assets"`
}

// Asset represents a downloadable file from a release.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
}

// UpgradeResult contains the result of an upgrade operation.
type UpgradeResult struct {
	CurrentVersion string
	NewVersion     string
	Downloaded     bool
	Installed      bool
	BackupPath     string
	Error          error
}

// FetchLatestRelease queries GitHub API for the latest release.
func FetchLatestRelease() (*ReleaseInfo, error) {
	client := &http.Client{Timeout: HTTPTimeout}

	req, err := http.NewRequest("GET", GitHubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBodySize))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	var release ReleaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxAPIResponseSize)).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing release info: %w", err)
	}

	return &release, nil
}

// SelectAsset finds the appropriate asset for the current OS/architecture.
// Expected naming: gastown_{version}_{os}_{arch}.tar.gz (or .zip for Windows)
func SelectAsset(release *ReleaseInfo) (*Asset, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Build expected archive name pattern
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}

	// GoReleaser uses format: gastown_{version}_{os}_{arch}.tar.gz
	// The version in the filename doesn't have the "v" prefix
	version := strings.TrimPrefix(release.TagName, "v")
	expectedName := fmt.Sprintf("gastown_%s_%s_%s%s", version, goos, goarch, ext)

	for i := range release.Assets {
		if release.Assets[i].Name == expectedName {
			return &release.Assets[i], nil
		}
	}

	// List available assets for debugging
	var available []string
	for _, a := range release.Assets {
		available = append(available, a.Name)
	}

	return nil, fmt.Errorf("no asset found for %s/%s, expected %q, available: %v",
		goos, goarch, expectedName, available)
}

// Download downloads an asset to a temporary file and returns the path.
func Download(asset *Asset, progress func(downloaded, total int64)) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}

	req, err := http.NewRequest("GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create temp file with appropriate extension
	ext := filepath.Ext(asset.Name)
	if strings.HasSuffix(asset.Name, ".tar.gz") {
		ext = ".tar.gz"
	}
	tmpFile, err := os.CreateTemp("", "gt-upgrade-*"+ext)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer func() { _ = tmpFile.Close() }()

	// Copy with optional progress tracking
	var reader io.Reader = resp.Body
	if progress != nil {
		reader = &progressReader{
			reader:   resp.Body,
			total:    resp.ContentLength,
			callback: progress,
		}
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("writing download: %w", err)
	}

	return tmpFile.Name(), nil
}

// progressReader wraps an io.Reader to track download progress.
type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	callback   func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.callback != nil {
		pr.callback(pr.downloaded, pr.total)
	}
	return n, err
}

// Extract extracts the binary from an archive and returns the path to it.
func Extract(archivePath string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath)
	}
	return extractTarGz(archivePath)
}

func extractTarGz(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive: %w", err)
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	// Create temp directory for extraction
	extractDir, err := os.MkdirTemp("", "gt-extract-*")
	if err != nil {
		return "", fmt.Errorf("creating extract dir: %w", err)
	}

	binaryName := "gt"
	var binaryPath string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = os.RemoveAll(extractDir)
			return "", fmt.Errorf("reading tar: %w", err)
		}

		// Only extract the binary
		name := filepath.Base(header.Name)
		if name != binaryName {
			continue
		}

		binaryPath = filepath.Join(extractDir, binaryName)
		// Mask mode to avoid integer overflow (G115) and use safe permissions (no world-writable)
		mode := os.FileMode(header.Mode & 0o755)
		outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY, mode)
		if err != nil {
			_ = os.RemoveAll(extractDir)
			return "", fmt.Errorf("creating file: %w", err)
		}

		// Use LimitReader to prevent decompression bomb attacks (G110)
		limitedReader := io.LimitReader(tr, MaxBinarySize)
		n, err := io.Copy(outFile, limitedReader)
		if err != nil {
			_ = outFile.Close()
			_ = os.RemoveAll(extractDir)
			return "", fmt.Errorf("extracting file: %w", err)
		}
		_ = outFile.Close()

		// Check if we hit the size limit (file was truncated)
		if n == MaxBinarySize {
			// Try reading one more byte to see if there's more data
			buf := make([]byte, 1)
			if extra, _ := tr.Read(buf); extra > 0 {
				_ = os.RemoveAll(extractDir)
				return "", fmt.Errorf("binary exceeds maximum allowed size of %d bytes", MaxBinarySize)
			}
		}
	}

	if binaryPath == "" {
		_ = os.RemoveAll(extractDir)
		return "", fmt.Errorf("binary %q not found in archive", binaryName)
	}

	return binaryPath, nil
}

func extractZip(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening zip: %w", err)
	}
	defer func() { _ = r.Close() }()

	extractDir, err := os.MkdirTemp("", "gt-extract-*")
	if err != nil {
		return "", fmt.Errorf("creating extract dir: %w", err)
	}

	binaryName := "gt.exe"
	var binaryPath string

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name != binaryName {
			continue
		}

		binaryPath = filepath.Join(extractDir, binaryName)

		rc, err := f.Open()
		if err != nil {
			_ = os.RemoveAll(extractDir)
			return "", fmt.Errorf("opening file in zip: %w", err)
		}

		// Mask mode to avoid arbitrary permissions from archive
		mode := f.Mode() & 0o755
		outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY, mode)
		if err != nil {
			_ = rc.Close()
			_ = os.RemoveAll(extractDir)
			return "", fmt.Errorf("creating file: %w", err)
		}

		// Use LimitReader to prevent decompression bomb attacks (G110)
		limitedReader := io.LimitReader(rc, MaxBinarySize)
		n, copyErr := io.Copy(outFile, limitedReader)
		_ = outFile.Close()

		// Check if we hit the size limit (file was truncated)
		if copyErr == nil && n == MaxBinarySize {
			// Try reading one more byte to see if there's more data
			buf := make([]byte, 1)
			if extra, _ := rc.Read(buf); extra > 0 {
				_ = rc.Close()
				_ = os.RemoveAll(extractDir)
				return "", fmt.Errorf("binary exceeds maximum allowed size of %d bytes", MaxBinarySize)
			}
		}
		_ = rc.Close()

		if copyErr != nil {
			_ = os.RemoveAll(extractDir)
			return "", fmt.Errorf("extracting file: %w", copyErr)
		}
	}

	if binaryPath == "" {
		_ = os.RemoveAll(extractDir)
		return "", fmt.Errorf("binary %q not found in archive", binaryName)
	}

	return binaryPath, nil
}

// GetCurrentBinaryPath returns the absolute path to the currently running binary.
// It resolves symlinks to get the real path.
func GetCurrentBinaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("getting executable path: %w", err)
	}

	// Resolve symlinks to get the actual binary location
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("resolving symlinks: %w", err)
	}

	return realPath, nil
}

// IsHomebrew returns true if the binary appears to be installed via Homebrew.
func IsHomebrew(binaryPath string) bool {
	lower := strings.ToLower(binaryPath)
	return strings.Contains(lower, "/cellar/") ||
		strings.Contains(lower, "/homebrew/") ||
		strings.Contains(lower, "/linuxbrew/")
}

// Replace performs atomic replacement of the current binary.
// Steps:
// 1. Rename current binary to .backup
// 2. Move new binary to original location
// 3. Preserve file mode from backup
// 4. Verify new binary runs
// 5. Remove backup on success (or restore on failure)
func Replace(currentPath, newBinaryPath string) error {
	// Get current binary's mode before backup
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}
	originalMode := info.Mode()

	// Create backup path
	backupPath := currentPath + ".backup"

	// Remove any existing backup
	_ = os.Remove(backupPath)

	// Step 1: Rename current to backup
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Step 2: Move new binary to original location
	if err := moveFile(newBinaryPath, currentPath); err != nil {
		// Restore backup on failure
		_ = os.Rename(backupPath, currentPath)
		return fmt.Errorf("installing new binary: %w", err)
	}

	// Step 3: Preserve file mode
	if err := os.Chmod(currentPath, originalMode); err != nil {
		// Non-fatal, but log it
		fmt.Fprintf(os.Stderr, "Warning: could not preserve file mode: %v\n", err)
	}

	// Step 4: Verify new binary runs
	if err := Verify(currentPath); err != nil {
		// Restore backup on failure
		_ = os.Remove(currentPath)
		_ = os.Rename(backupPath, currentPath)
		return fmt.Errorf("verifying new binary: %w", err)
	}

	// Step 5: Remove backup on success
	_ = os.Remove(backupPath)

	return nil
}

// moveFile moves a file, using copy+delete if rename fails (cross-device).
func moveFile(src, dst string) error {
	// Try rename first (atomic if same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fall back to copy + delete for cross-device moves
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return os.Remove(src)
}

// Verify runs the new binary with --version to ensure it works.
func Verify(binaryPath string) error {
	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("binary failed to run: %w (output: %s)", err, string(output))
	}

	// Check that output contains "gt version"
	if !strings.Contains(string(output), "gt version") {
		return fmt.Errorf("unexpected version output: %s", string(output))
	}

	return nil
}

// FormatSize formats a byte size as human-readable string.
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
