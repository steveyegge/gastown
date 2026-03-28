// Package estop provides emergency stop functionality for Gas Town.
//
// The E-stop is a town-wide mechanism to pause all agent work. It uses a
// sentinel file (ESTOP) at the town root. When present, all agents should
// be frozen (SIGTSTP) and the daemon should not restart them.
//
// The Mayor is exempt from E-stop so it can coordinate recovery.
//
// Original implementation by outdoorsea (PR #3237).
package estop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is the sentinel file name at the town root.
const FileName = "ESTOP"

// TriggerManual is the prefix for a human-triggered E-stop.
const TriggerManual = "manual"

// TriggerAuto is the prefix for an auto-triggered E-stop.
const TriggerAuto = "auto"

// Info represents the parsed contents of an ESTOP file.
type Info struct {
	Trigger   string    // "manual" or "auto"
	Reason    string    // human-readable reason
	Timestamp time.Time // when the E-stop was triggered
}

// FilePath returns the full path to the ESTOP sentinel file.
func FilePath(townRoot string) string {
	return filepath.Join(townRoot, FileName)
}

// IsActive checks whether an E-stop is currently active.
func IsActive(townRoot string) bool {
	_, err := os.Stat(FilePath(townRoot))
	return err == nil
}

// Read reads and parses the ESTOP file. Returns nil if not active.
func Read(townRoot string) *Info {
	data, err := os.ReadFile(FilePath(townRoot))
	if err != nil {
		return nil
	}
	return parse(string(data))
}

// Activate creates the ESTOP sentinel file with the given trigger and reason.
func Activate(townRoot, trigger, reason string) error {
	ts := time.Now().Format(time.RFC3339)
	content := fmt.Sprintf("%s\t%s\t%s\n", trigger, ts, reason)
	return os.WriteFile(FilePath(townRoot), []byte(content), 0644)
}

// Deactivate removes the ESTOP sentinel file.
// If onlyAuto is true, only removes auto-triggered E-stops.
func Deactivate(townRoot string, onlyAuto bool) error {
	if onlyAuto {
		info := Read(townRoot)
		if info != nil && info.Trigger == TriggerManual {
			return fmt.Errorf("E-stop was manually triggered — use 'gt thaw' to clear")
		}
	}
	err := os.Remove(FilePath(townRoot))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RigFileName returns the sentinel file name for a per-rig E-stop.
func RigFileName(rigName string) string {
	return fmt.Sprintf("ESTOP.%s", rigName)
}

// RigFilePath returns the full path to a per-rig ESTOP sentinel file.
func RigFilePath(townRoot, rigName string) string {
	return filepath.Join(townRoot, RigFileName(rigName))
}

// IsRigActive checks whether a per-rig E-stop is active.
func IsRigActive(townRoot, rigName string) bool {
	_, err := os.Stat(RigFilePath(townRoot, rigName))
	return err == nil
}

// ReadRig reads and parses a per-rig ESTOP file. Returns nil if not active.
func ReadRig(townRoot, rigName string) *Info {
	data, err := os.ReadFile(RigFilePath(townRoot, rigName))
	if err != nil {
		return nil
	}
	return parse(string(data))
}

// ActivateRig creates a per-rig ESTOP sentinel file.
func ActivateRig(townRoot, rigName, trigger, reason string) error {
	ts := time.Now().Format(time.RFC3339)
	content := fmt.Sprintf("%s\t%s\t%s\n", trigger, ts, reason)
	return os.WriteFile(RigFilePath(townRoot, rigName), []byte(content), 0644)
}

// DeactivateRig removes a per-rig ESTOP sentinel file.
func DeactivateRig(townRoot, rigName string) error {
	err := os.Remove(RigFilePath(townRoot, rigName))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsAnyActive checks if a town-wide or rig-specific E-stop affects this rig.
func IsAnyActive(townRoot, rigName string) bool {
	return IsActive(townRoot) || IsRigActive(townRoot, rigName)
}

func parse(content string) *Info {
	content = strings.TrimSpace(content)
	if content == "" {
		return &Info{Trigger: TriggerManual, Timestamp: time.Now()}
	}

	// Format: trigger\ttimestamp\treason
	parts := strings.SplitN(content, "\t", 3)
	info := &Info{Trigger: TriggerManual}

	if len(parts) >= 1 {
		info.Trigger = parts[0]
	}
	if len(parts) >= 2 {
		if t, err := time.Parse(time.RFC3339, parts[1]); err == nil {
			info.Timestamp = t
		}
	}
	if len(parts) >= 3 {
		info.Reason = parts[2]
	}

	return info
}
