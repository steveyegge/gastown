package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// BackupDirName is the name of the backup directory within .migration-backup.
	BackupDirName = ".migration-backup"

	// ManifestFileName is the name of the backup manifest file.
	ManifestFileName = "manifest.json"
)

// BackupManager handles creating and restoring migration backups.
type BackupManager struct {
	townRoot string
}

// NewBackupManager creates a new backup manager for the given workspace.
func NewBackupManager(townRoot string) *BackupManager {
	return &BackupManager{townRoot: townRoot}
}

// CreateBackup creates a timestamped backup of critical workspace files.
// Returns the backup directory path and a manifest describing the backup.
func (b *BackupManager) CreateBackup(migrationID string, fromVersion, toVersion string) (string, *BackupManifest, error) {
	// Create backup directory with timestamp
	timestamp := time.Now()
	backupName := fmt.Sprintf("%s-%s", timestamp.Format("20060102-150405"), migrationID)
	backupDir := filepath.Join(b.townRoot, BackupDirName, backupName)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", nil, fmt.Errorf("creating backup directory: %w", err)
	}

	manifest := &BackupManifest{
		Timestamp:   timestamp,
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		TownRoot:    b.townRoot,
		MigrationID: migrationID,
		Files:       []BackupFile{},
	}

	// Files/directories to back up - use constant list for maintainability
	itemsToBackup := BackupFiles()

	// Add rig-level beads directories
	rigs := detectRigs(b.townRoot)
	for _, rigPath := range rigs {
		rigName := filepath.Base(rigPath)
		// Check for rig-level .beads directory
		rigBeadsDir := filepath.Join(rigName, ".beads")
		if _, err := os.Stat(filepath.Join(b.townRoot, rigBeadsDir)); err == nil {
			itemsToBackup = append(itemsToBackup, rigBeadsDir)
		}
		// Also check for mayor/rig/.beads (0.2.x layout)
		rigMayorBeadsDir := filepath.Join(rigName, "mayor", "rig", ".beads")
		if _, err := os.Stat(filepath.Join(b.townRoot, rigMayorBeadsDir)); err == nil {
			itemsToBackup = append(itemsToBackup, rigMayorBeadsDir)
		}
	}

	for _, item := range itemsToBackup {
		srcPath := filepath.Join(b.townRoot, item)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue // Skip non-existent files
		}

		backupPath := filepath.Join(backupDir, item)
		if err := b.copyItem(srcPath, backupPath); err != nil {
			// Clean up on failure
			_ = os.RemoveAll(backupDir)
			return "", nil, fmt.Errorf("backing up %s: %w", item, err)
		}

		info, _ := os.Stat(srcPath)
		manifest.Files = append(manifest.Files, BackupFile{
			OriginalPath: item,
			BackupPath:   item,
			IsDirectory:  info.IsDir(),
			Size:         info.Size(),
		})
	}

	// Write manifest
	manifestPath := filepath.Join(backupDir, ManifestFileName)
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = os.RemoveAll(backupDir)
		return "", nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	// Add trailing newline for POSIX compliance
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		_ = os.RemoveAll(backupDir)
		return "", nil, fmt.Errorf("writing manifest: %w", err)
	}

	return backupDir, manifest, nil
}

// RestoreBackup restores a workspace from a backup directory.
// Uses a two-phase approach for atomicity:
// 1. Copy all files to a staging location (.restore-staging)
// 2. Move files from staging to final locations
// This ensures we don't delete existing files until we're sure the copy succeeded.
func (b *BackupManager) RestoreBackup(backupDir string) error {
	// Load manifest
	manifestPath := filepath.Join(backupDir, ManifestFileName)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// Phase 1: Copy all files to staging directory
	stagingDir := filepath.Join(b.townRoot, ".restore-staging")
	if err := os.RemoveAll(stagingDir); err != nil {
		return fmt.Errorf("cleaning staging directory: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }() // Clean up staging on exit

	for _, file := range manifest.Files {
		srcPath := filepath.Join(backupDir, file.BackupPath)
		stagingPath := filepath.Join(stagingDir, file.OriginalPath)

		// Ensure parent directory exists in staging
		if err := os.MkdirAll(filepath.Dir(stagingPath), 0755); err != nil {
			return fmt.Errorf("creating staging directory for %s: %w", file.OriginalPath, err)
		}

		// Copy from backup to staging
		if err := b.copyItem(srcPath, stagingPath); err != nil {
			return fmt.Errorf("copying %s to staging: %w", file.OriginalPath, err)
		}
	}

	// Phase 2: All copies succeeded, now replace existing files atomically.
	// We use rename-to-backup-then-rename pattern to ensure we don't lose
	// existing files if the process is interrupted.
	var replacedFiles []string // Track files we've replaced for cleanup
	for _, file := range manifest.Files {
		stagingPath := filepath.Join(stagingDir, file.OriginalPath)
		dstPath := filepath.Join(b.townRoot, file.OriginalPath)
		tmpBackupPath := dstPath + ".restore-backup"

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", file.OriginalPath, err)
		}

		// Step 1: If destination exists, rename to temporary backup
		dstExists := false
		if _, err := os.Stat(dstPath); err == nil {
			dstExists = true
			if err := os.Rename(dstPath, tmpBackupPath); err != nil {
				// If rename fails, try to remove (might be cross-device or permission issue)
				if removeErr := os.RemoveAll(dstPath); removeErr != nil {
					return fmt.Errorf("could not move or remove existing %s: rename=%v, remove=%v", file.OriginalPath, err, removeErr)
				}
				dstExists = false // Successfully removed, no backup needed
			}
		}

		// Step 2: Move from staging to final location
		if err := os.Rename(stagingPath, dstPath); err != nil {
			// Fall back to copy if rename fails (cross-device)
			if copyErr := b.copyItem(stagingPath, dstPath); copyErr != nil {
				// Restore from temp backup if we have one
				if dstExists {
					_ = os.Rename(tmpBackupPath, dstPath)
				}
				return fmt.Errorf("restoring %s: %w", file.OriginalPath, copyErr)
			}
		}

		// Step 3: Remove temp backup on success
		if dstExists {
			_ = os.RemoveAll(tmpBackupPath)
		}
		replacedFiles = append(replacedFiles, dstPath)
	}

	return nil
}

// backupWithTime pairs a backup path with its timestamp for sorting.
type backupWithTime struct {
	path      string
	timestamp time.Time
}

// ListBackups returns all available backup directories, sorted by timestamp (oldest first).
func (b *BackupManager) ListBackups() ([]string, error) {
	backupBase := filepath.Join(b.townRoot, BackupDirName)
	if _, err := os.Stat(backupBase); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(backupBase)
	if err != nil {
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	var backupsWithTime []backupWithTime
	for _, entry := range entries {
		if entry.IsDir() {
			backupDir := filepath.Join(backupBase, entry.Name())
			manifestPath := filepath.Join(backupDir, ManifestFileName)

			manifestData, err := os.ReadFile(manifestPath)
			if err != nil {
				continue // Skip directories without valid manifest
			}

			var manifest BackupManifest
			if err := json.Unmarshal(manifestData, &manifest); err != nil {
				continue // Skip directories with invalid manifest
			}

			backupsWithTime = append(backupsWithTime, backupWithTime{
				path:      backupDir,
				timestamp: manifest.Timestamp,
			})
		}
	}

	// Sort by timestamp (oldest first)
	sort.Slice(backupsWithTime, func(i, j int) bool {
		return backupsWithTime[i].timestamp.Before(backupsWithTime[j].timestamp)
	})

	// Extract just the paths
	backups := make([]string, len(backupsWithTime))
	for i, b := range backupsWithTime {
		backups[i] = b.path
	}

	return backups, nil
}

// GetLatestBackup returns the most recent backup directory.
func (b *BackupManager) GetLatestBackup() (string, *BackupManifest, error) {
	backups, err := b.ListBackups()
	if err != nil {
		return "", nil, err
	}
	if len(backups) == 0 {
		return "", nil, fmt.Errorf("no backups found")
	}

	// Backups are sorted by timestamp in their name, so the last one is newest
	latestDir := backups[len(backups)-1]

	// Load manifest
	manifestPath := filepath.Join(latestDir, ManifestFileName)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return latestDir, &manifest, nil
}

// CleanupBackup removes a backup directory.
func (b *BackupManager) CleanupBackup(backupDir string) error {
	return os.RemoveAll(backupDir)
}

// CleanupOldBackups removes backups older than the specified duration.
func (b *BackupManager) CleanupOldBackups(maxAge time.Duration) error {
	backups, err := b.ListBackups()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-maxAge)

	for _, backupDir := range backups {
		manifestPath := filepath.Join(backupDir, ManifestFileName)
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest BackupManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			continue
		}

		if manifest.Timestamp.Before(cutoff) {
			_ = os.RemoveAll(backupDir)
		}
	}

	return nil
}

// copyItem copies a file or directory from src to dst.
func (b *BackupManager) copyItem(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return b.copyDir(src, dst)
	}
	return b.copyFile(src, dst)
}

// copyFile copies a single file.
func (b *BackupManager) copyFile(src, dst string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyDir copies a directory recursively.
func (b *BackupManager) copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := b.copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := b.copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
