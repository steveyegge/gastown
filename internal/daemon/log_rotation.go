package daemon

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// logRotationMaxSize is the max size in bytes before auto-rotation triggers.
	// 100MB matches the lumberjack default for daemon.log.
	logRotationMaxSize int64 = 100 * 1024 * 1024

	// logRotationMaxBackups is the maximum number of rotated log files to keep.
	logRotationMaxBackups = 3

	// staleArchiveMaxAge is the maximum age for timestamped archive files.
	// Archives older than this are deleted by cleanStaleArchives.
	staleArchiveMaxAge = 7 * 24 * time.Hour

	// daemonDiskBudget is the maximum total size of the daemon/ directory in bytes.
	// If exceeded, oldest .gz files are deleted until under budget.
	daemonDiskBudget int64 = 500 * 1024 * 1024 // 500MB
)

// staleArchivePattern matches timestamped archive files like dolt-2026-02-28T23-19-42.log.gz
var staleArchivePattern = regexp.MustCompile(`^.+-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.log\.gz$`)

// RotateLogsResult holds the result of a log rotation run.
type RotateLogsResult struct {
	Rotated []string // Log files that were rotated
	Skipped []string // Log files that were too small
	Errors  []error  // Non-fatal errors
}

// CleanupResult holds the result of archive cleanup operations.
type CleanupResult struct {
	StaleRemoved  []string // Stale timestamped archives deleted
	BudgetRemoved []string // Files deleted to meet disk budget
	Errors        []error  // Non-fatal errors
}

// RotateLogs rotates all daemon-managed log files using copytruncate.
// This is safe for Dolt server logs where the child process holds an open fd.
// daemon.log is handled by lumberjack and is skipped here.
func RotateLogs(townRoot string) *RotateLogsResult {
	result := &RotateLogsResult{}
	daemonDir := filepath.Join(townRoot, "daemon")

	// Collect all log files to rotate (excludes daemon.log which uses lumberjack)
	logFiles := collectDoltLogFiles(daemonDir, townRoot)

	for _, logPath := range logFiles {
		info, err := os.Stat(logPath)
		if err != nil {
			if !os.IsNotExist(err) {
				result.Errors = append(result.Errors, fmt.Errorf("stat %s: %w", logPath, err))
			}
			continue
		}

		if info.Size() < logRotationMaxSize {
			result.Skipped = append(result.Skipped, logPath)
			continue
		}

		if err := copyTruncateRotate(logPath); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("rotating %s: %w", logPath, err))
		} else {
			result.Rotated = append(result.Rotated, logPath)
		}
	}

	// Clean stale archives and enforce disk budget after rotation
	CleanDaemonDir(townRoot)

	return result
}

// ForceRotateLogs rotates all daemon-managed log files regardless of size.
func ForceRotateLogs(townRoot string) *RotateLogsResult {
	result := &RotateLogsResult{}
	daemonDir := filepath.Join(townRoot, "daemon")

	logFiles := collectDoltLogFiles(daemonDir, townRoot)

	for _, logPath := range logFiles {
		info, err := os.Stat(logPath)
		if err != nil {
			if !os.IsNotExist(err) {
				result.Errors = append(result.Errors, fmt.Errorf("stat %s: %w", logPath, err))
			}
			continue
		}

		if info.Size() == 0 {
			result.Skipped = append(result.Skipped, logPath)
			continue
		}

		if err := copyTruncateRotate(logPath); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("rotating %s: %w", logPath, err))
		} else {
			result.Rotated = append(result.Rotated, logPath)
		}
	}

	return result
}

// collectDoltLogFiles returns all Dolt-related log files that need copytruncate rotation.
// Excludes daemon.log (handled by lumberjack).
func collectDoltLogFiles(daemonDir, townRoot string) []string {
	var logFiles []string

	// daemon-level Dolt logs
	for _, name := range []string{"dolt.log", "dolt-server.log", "dolt-test-server.log"} {
		path := filepath.Join(daemonDir, name)
		if _, err := os.Stat(path); err == nil {
			logFiles = append(logFiles, path)
		}
	}

	// rig-level .beads/dolt-server.log files
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return logFiles
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "daemon" {
			continue
		}
		rigLog := filepath.Join(townRoot, entry.Name(), ".beads", "dolt-server.log")
		if _, err := os.Stat(rigLog); err == nil {
			logFiles = append(logFiles, rigLog)
		}
		// Also check mayor/rig/.beads path
		mayorRigLog := filepath.Join(townRoot, entry.Name(), "rig", ".beads", "dolt-server.log")
		if _, err := os.Stat(mayorRigLog); err == nil {
			logFiles = append(logFiles, mayorRigLog)
		}
	}

	return logFiles
}

// copyTruncateRotate performs a safe copytruncate rotation:
// 1. Copy current log to .1.gz (compressed)
// 2. Truncate the original file to 0 bytes
// 3. Clean up old rotations beyond maxBackups
//
// This is safe for files held open by child processes (like Dolt server)
// because the fd remains valid — only the file content is truncated.
func copyTruncateRotate(logPath string) error {
	// Shift existing rotations: .2.gz → .3.gz, .1.gz → .2.gz
	for i := logRotationMaxBackups; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d.gz", logPath, i)
		if i == logRotationMaxBackups {
			// Remove the oldest
			os.Remove(old)
		} else {
			next := fmt.Sprintf("%s.%d.gz", logPath, i+1)
			_ = os.Rename(old, next)
		}
	}

	// Copy current log to .1.gz
	dst := logPath + ".1.gz"
	if err := compressFile(logPath, dst); err != nil {
		return fmt.Errorf("compressing to %s: %w", dst, err)
	}

	// Truncate original (keeps fd valid for child processes)
	if err := os.Truncate(logPath, 0); err != nil {
		return fmt.Errorf("truncating %s: %w", logPath, err)
	}

	// Clean up any extra old rotations
	cleanOldRotations(logPath)

	return nil
}

// compressFile copies src to dst with gzip compression.
func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)

	_, err = io.Copy(gz, in)
	if closeErr := gz.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

// CleanDaemonDir runs stale archive cleanup and disk budget enforcement.
// Called from RotateLogs after normal rotation, and can be called independently.
func CleanDaemonDir(townRoot string) *CleanupResult {
	daemonDir := filepath.Join(townRoot, "daemon")
	result := &CleanupResult{}

	// Phase 1: Remove stale timestamped archives (older than 7 days)
	stale, errs := cleanStaleArchives(daemonDir)
	result.StaleRemoved = stale
	result.Errors = append(result.Errors, errs...)

	// Phase 2: Enforce disk budget (delete oldest .gz files until under 500MB)
	budgetRemoved, errs := enforceDiskBudget(daemonDir)
	result.BudgetRemoved = budgetRemoved
	result.Errors = append(result.Errors, errs...)

	return result
}

// cleanStaleArchives removes timestamped archive files older than staleArchiveMaxAge.
// These are files like dolt-2026-02-28T23-19-42.log.gz created by manual/one-time archiving.
func cleanStaleArchives(daemonDir string) (removed []string, errs []error) {
	entries, err := os.ReadDir(daemonDir)
	if err != nil {
		return nil, []error{fmt.Errorf("reading daemon dir: %w", err)}
	}

	cutoff := time.Now().Add(-staleArchiveMaxAge)
	for _, entry := range entries {
		if entry.IsDir() || !staleArchivePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, fmt.Errorf("stat %s: %w", entry.Name(), err))
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(daemonDir, entry.Name())
			if err := os.Remove(path); err != nil {
				errs = append(errs, fmt.Errorf("removing stale archive %s: %w", entry.Name(), err))
			} else {
				removed = append(removed, path)
			}
		}
	}
	return removed, errs
}

// enforceDiskBudget deletes oldest .gz files in daemon/ until total size is under daemonDiskBudget.
func enforceDiskBudget(daemonDir string) (removed []string, errs []error) {
	totalSize, gzFiles, err := collectGzFiles(daemonDir)
	if err != nil {
		return nil, []error{fmt.Errorf("collecting gz files: %w", err)}
	}

	if totalSize <= daemonDiskBudget {
		return nil, nil
	}

	// Sort by modification time, oldest first
	sort.Slice(gzFiles, func(i, j int) bool {
		return gzFiles[i].modTime.Before(gzFiles[j].modTime)
	})

	for _, gf := range gzFiles {
		if totalSize <= daemonDiskBudget {
			break
		}
		if err := os.Remove(gf.path); err != nil {
			errs = append(errs, fmt.Errorf("removing %s for budget: %w", filepath.Base(gf.path), err))
			continue
		}
		totalSize -= gf.size
		removed = append(removed, gf.path)
	}
	return removed, errs
}

type gzFileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// collectGzFiles returns the total size of daemon/ and a list of .gz files with metadata.
func collectGzFiles(daemonDir string) (totalSize int64, gzFiles []gzFileInfo, err error) {
	entries, err := os.ReadDir(daemonDir)
	if err != nil {
		return 0, nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
		if strings.HasSuffix(entry.Name(), ".gz") {
			gzFiles = append(gzFiles, gzFileInfo{
				path:    filepath.Join(daemonDir, entry.Name()),
				size:    info.Size(),
				modTime: info.ModTime(),
			})
		}
	}
	return totalSize, gzFiles, nil
}

// cleanOldRotations removes rotations beyond maxBackups.
func cleanOldRotations(logPath string) {
	dir := filepath.Dir(logPath)
	base := filepath.Base(logPath)
	pattern := base + ".*.gz"

	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) <= logRotationMaxBackups {
		return
	}

	// Sort by modification time (oldest first)
	sort.Slice(matches, func(i, j int) bool {
		fi, _ := os.Stat(matches[i])
		fj, _ := os.Stat(matches[j])
		if fi == nil || fj == nil {
			return false
		}
		return fi.ModTime().Before(fj.ModTime())
	})

	// Remove extras beyond maxBackups
	for i := 0; i < len(matches)-logRotationMaxBackups; i++ {
		os.Remove(matches[i])
	}
}
