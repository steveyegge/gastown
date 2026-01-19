package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultGitHubAPIURL is the endpoint for fetching latest release info.
const DefaultGitHubAPIURL = "https://api.github.com/repos/steveyegge/gastown/releases/latest"

// getUpgradeURL returns the URL to use for fetching release info.
// GT_UPGRADE_URL env var is only allowed for testing with localhost URLs.
func getUpgradeURL() string {
	if url := os.Getenv("GT_UPGRADE_URL"); url != "" {
		// Only allow localhost URLs for testing to prevent injection attacks
		if strings.HasPrefix(url, "http://localhost") ||
			strings.HasPrefix(url, "http://127.0.0.1") ||
			strings.HasPrefix(url, "https://localhost") ||
			strings.HasPrefix(url, "https://127.0.0.1") {
			return url
		}
		// Silently ignore non-localhost URLs - don't let attackers know their injection failed
	}
	return DefaultGitHubAPIURL
}

const (
	// UserAgent is sent with GitHub API requests.
	UserAgent = "gt-upgrade/1.0"

	// HTTPTimeout is the default timeout for HTTP requests (API calls, checksum fetches).
	HTTPTimeout = 30 * time.Second

	// DownloadTimeout is the timeout for downloading release archives.
	// Longer than HTTPTimeout to accommodate larger files on slower connections.
	DownloadTimeout = 5 * time.Minute

	// MaxBinarySize is the maximum size for extracted binaries (100MB).
	// This prevents decompression bomb attacks.
	MaxBinarySize = 100 * 1024 * 1024

	// MaxArchiveSize is the maximum size for archive downloads (200MB).
	// This prevents excessive disk usage from malicious archives.
	MaxArchiveSize = 200 * 1024 * 1024

	// MaxAPIResponseSize is the maximum size for API responses (1MB).
	// This prevents memory exhaustion from malicious servers.
	MaxAPIResponseSize = 1 * 1024 * 1024

	// MaxErrorBodySize is the maximum size for error response bodies (4KB).
	MaxErrorBodySize = 4 * 1024
)

// allowedDownloadHosts is the list of exact hosts from which downloads are permitted.
// This prevents MITM attacks that could redirect downloads to malicious servers.
// Note: Only exact matches allowed (no subdomains) to prevent spoofing.
var allowedDownloadHosts = []string{
	"github.com",
	"objects.githubusercontent.com",
}

// AllowedDownloadHosts returns a copy of the allowed download hosts list.
// This prevents external modification of the security-critical whitelist.
func AllowedDownloadHosts() []string {
	result := make([]string, len(allowedDownloadHosts))
	copy(result, allowedDownloadHosts)
	return result
}

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

// FetchLatestRelease queries GitHub API for the latest release.
// Use FetchLatestReleaseWithContext for cancellation support.
func FetchLatestRelease() (*ReleaseInfo, error) {
	return FetchLatestReleaseWithContext(context.Background())
}

// FetchLatestReleaseWithContext queries GitHub API for the latest release with context support.
func FetchLatestReleaseWithContext(ctx context.Context) (*ReleaseInfo, error) {
	client := &http.Client{Timeout: HTTPTimeout}

	req, err := http.NewRequestWithContext(ctx, "GET", getUpgradeURL(), nil)
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

// ValidateDownloadURL checks that a download URL points to an allowed host.
// This prevents MITM attacks that could redirect downloads to malicious servers.
// Only exact host matches are allowed (no subdomains) to prevent spoofing attacks.
func ValidateDownloadURL(downloadURL string) error {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("download URL must use HTTPS, got %s", parsed.Scheme)
	}

	// Use Hostname() to strip port number - parsed.Host includes port
	host := strings.ToLower(parsed.Hostname())
	for _, allowed := range allowedDownloadHosts {
		// Only exact matches allowed - no subdomain matching to prevent spoofing
		// (e.g., malicious.github.com would not be allowed)
		if host == allowed {
			return nil
		}
	}

	return fmt.Errorf("download URL host %q not in allowed list: %v", host, allowedDownloadHosts)
}

// newDownloadClient creates an HTTP client that validates redirect URLs.
// This ensures we don't follow redirects to disallowed hosts.
func newDownloadClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Validate each redirect destination
			if err := ValidateDownloadURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect to disallowed host: %w", err)
			}
			// Default redirect limit is 10
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
}

// Download downloads an asset to a temporary file and returns the path.
// Use DownloadWithContext for cancellation support.
func Download(asset *Asset, progress func(downloaded, total int64)) (string, error) {
	return DownloadWithContext(context.Background(), asset, progress)
}

// DownloadWithContext downloads an asset to a temporary file with context support.
// The caller is responsible for removing the returned file when done.
func DownloadWithContext(ctx context.Context, asset *Asset, progress func(downloaded, total int64)) (string, error) {
	// Validate download URL before proceeding
	if err := ValidateDownloadURL(asset.BrowserDownloadURL); err != nil {
		return "", fmt.Errorf("validating download URL: %w", err)
	}

	// Validate archive size before downloading
	if asset.Size > MaxArchiveSize {
		return "", fmt.Errorf("archive size %d exceeds maximum allowed %d bytes", asset.Size, MaxArchiveSize)
	}

	// Use client with redirect validation
	client := newDownloadClient(DownloadTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", asset.BrowserDownloadURL, nil)
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

	// Limit download size to prevent excessive disk usage
	limitedBody := io.LimitReader(resp.Body, MaxArchiveSize+1)

	// Copy with optional progress tracking
	var reader io.Reader = limitedBody
	if progress != nil {
		reader = &progressReader{
			reader:   limitedBody,
			total:    resp.ContentLength,
			callback: progress,
		}
	}

	written, err := io.Copy(tmpFile, reader)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("writing download: %w", err)
	}

	// Check if we hit the size limit
	if written > MaxArchiveSize {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download exceeded maximum archive size of %d bytes", MaxArchiveSize)
	}

	return tmpFile.Name(), nil
}

// FetchChecksum fetches and parses the checksums file for a release.
// Returns a map of filename -> sha256 hash.
// Use FetchChecksumWithContext for cancellation support.
func FetchChecksum(release *ReleaseInfo) (map[string]string, error) {
	return FetchChecksumWithContext(context.Background(), release)
}

// FetchChecksumWithContext fetches and parses the checksums file with context support.
func FetchChecksumWithContext(ctx context.Context, release *ReleaseInfo) (map[string]string, error) {
	// Find the checksums file (GoReleaser convention)
	var checksumAsset *Asset
	for i := range release.Assets {
		name := release.Assets[i].Name
		if strings.HasSuffix(name, "_checksums.txt") || name == "checksums.txt" {
			checksumAsset = &release.Assets[i]
			break
		}
	}

	if checksumAsset == nil {
		return nil, fmt.Errorf("no checksums file found in release")
	}

	// Validate checksum URL
	if err := ValidateDownloadURL(checksumAsset.BrowserDownloadURL); err != nil {
		return nil, fmt.Errorf("validating checksums URL: %w", err)
	}

	client := &http.Client{Timeout: HTTPTimeout}
	req, err := http.NewRequestWithContext(ctx, "GET", checksumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums fetch failed with status %d", resp.StatusCode)
	}

	// Parse checksums file (format: "sha256hash  filename")
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, MaxAPIResponseSize))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := parts[0]
			filename := parts[len(parts)-1] // Last field is filename
			checksums[filename] = hash
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parsing checksums: %w", err)
	}

	return checksums, nil
}

// VerifyChecksum verifies a file's SHA256 hash matches the expected value.
func VerifyChecksum(filePath string, expectedHash string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
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
// 1. Verify new binary works BEFORE installation (security: don't execute untrusted code at final path)
// 2. Rename current binary to .backup
// 3. Move new binary to original location
// 4. Preserve file mode from backup
// 5. Verify again after installation
// 6. Remove backup on success (or restore on failure)
func Replace(currentPath, newBinaryPath string) error {
	// Step 1: Verify new binary BEFORE installation
	// This is critical for security: verify in temp location before trusting it
	if err := Verify(newBinaryPath); err != nil {
		return fmt.Errorf("verifying new binary before install: %w", err)
	}

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

	// Step 2: Rename current to backup
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Step 3: Move new binary to original location
	if err := moveFile(newBinaryPath, currentPath); err != nil {
		// Restore backup on failure
		restoreErr := os.Rename(backupPath, currentPath)
		if restoreErr != nil {
			// Critical: restore failed, user may be left without a working binary
			return fmt.Errorf("installing new binary failed (%w) AND restore from backup also failed (%v) - manual intervention required, backup at %s",
				err, restoreErr, backupPath)
		}
		return fmt.Errorf("installing new binary: %w", err)
	}

	// Step 4: Preserve file mode
	if err := os.Chmod(currentPath, originalMode); err != nil {
		// Non-fatal - return as warning in result
		// Note: We intentionally don't write to stderr from library code
	}

	// Step 5: Verify again after installation (sanity check)
	if err := Verify(currentPath); err != nil {
		// Restore backup on failure
		removeErr := os.Remove(currentPath)
		restoreErr := os.Rename(backupPath, currentPath)
		if restoreErr != nil {
			// Critical: restore failed, user may be left without a working binary
			return fmt.Errorf("post-install verification failed (%w) AND restore from backup also failed (remove: %v, restore: %v) - manual intervention required, backup at %s",
				err, removeErr, restoreErr, backupPath)
		}
		return fmt.Errorf("post-install verification failed: %w", err)
	}

	// Step 6: Remove backup on success
	_ = os.Remove(backupPath)

	return nil
}

// moveFile moves a file, using copy+delete if rename fails (cross-device).
// Applies MaxBinarySize limit during copy to prevent disk exhaustion attacks.
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
	defer func() { _ = srcFile.Close() }()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Check file size before copying to prevent disk exhaustion
	if info.Size() > MaxBinarySize {
		return fmt.Errorf("file size %d exceeds maximum allowed %d bytes", info.Size(), MaxBinarySize)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}

	// Use LimitReader to enforce size limit during copy
	limitedReader := io.LimitReader(srcFile, MaxBinarySize)
	written, err := io.Copy(dstFile, limitedReader)
	if err != nil {
		_ = dstFile.Close()
		_ = os.Remove(dst)
		return err
	}

	// Verify we copied the expected amount
	if written != info.Size() {
		_ = dstFile.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("incomplete copy: expected %d bytes, wrote %d", info.Size(), written)
	}

	// Explicitly close dstFile to flush writes before returning success
	if err := dstFile.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("closing destination file: %w", err)
	}

	// Remove source file - don't fail the operation if dst is already valid
	// Note: We don't log to stderr from library code - caller can handle cleanup
	_ = os.Remove(src)

	return nil
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
