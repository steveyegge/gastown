package daemon

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// logRotationMaxSize is the max size in bytes before auto-rotation triggers.
	// 100MB matches the lumberjack default for daemon.log.
	logRotationMaxSize int64 = 100 * 1024 * 1024

	// logRotationMaxBackups is the maximum number of rotated log files to keep.
	logRotationMaxBackups = 3
)

// RotateLogsResult holds the result of a log rotation run.
type RotateLogsResult struct {
	Rotated []string // Log files that were rotated
	Skipped []string // Log files that were too small
	Errors  []error  // Non-fatal errors
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
