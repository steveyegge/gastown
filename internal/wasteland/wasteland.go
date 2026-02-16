// Package wasteland implements the Wasteland federation protocol for Gas Town.
//
// The Wasteland is a federation of Gas Towns via DoltHub. Each town has a
// sovereign fork of a shared commons database. Towns register by writing
// to the commons' towns table, and contribute wanted work items and
// completions through DoltHub's fork/PR/merge primitives.
//
// See ~/hop/docs/wasteland/design.md for the full design.
package wasteland

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Config holds the wasteland configuration for a town.
type Config struct {
	// Upstream is the DoltHub path of the upstream commons (e.g., "steveyegge/wl-commons").
	Upstream string `json:"upstream"`

	// ForkOrg is the DoltHub org where the fork lives (e.g., "alice-dev").
	ForkOrg string `json:"fork_org"`

	// ForkDB is the database name of the fork (e.g., "wl-commons").
	ForkDB string `json:"fork_db"`

	// LocalDir is the absolute path to the local clone of the fork.
	LocalDir string `json:"local_dir"`

	// TownHandle is the town's handle in the registry.
	TownHandle string `json:"town_handle"`

	// JoinedAt is when the town joined the wasteland.
	JoinedAt time.Time `json:"joined_at"`
}

// ConfigPath returns the path to the wasteland config file for a town.
func ConfigPath(townRoot string) string {
	return filepath.Join(townRoot, "mayor", "wasteland.json")
}

// LoadConfig loads the wasteland configuration from disk.
func LoadConfig(townRoot string) (*Config, error) {
	data, err := os.ReadFile(ConfigPath(townRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("town has not joined a wasteland (run 'gt wl join <upstream>')")
		}
		return nil, fmt.Errorf("reading wasteland config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing wasteland config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes the wasteland configuration to disk.
func SaveConfig(townRoot string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling wasteland config: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(ConfigPath(townRoot), data, 0644)
}

// dolthubAPIBase is the DoltHub REST API base URL.
// Var so tests can override it.
var dolthubAPIBase = "https://www.dolthub.com/api/v1alpha1"

// dolthubRemoteBase is the Dolt remote API base URL.
const dolthubRemoteBase = "https://doltremoteapi.dolthub.com"

// ParseUpstream parses an upstream path like "steveyegge/wl-commons" into org and db.
func ParseUpstream(upstream string) (org, db string, err error) {
	parts := strings.SplitN(upstream, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid upstream path %q: expected format 'org/database'", upstream)
	}
	return parts[0], parts[1], nil
}

// ForkDoltHubRepo forks a DoltHub database to the target org.
// Uses the DoltHub fork API endpoint.
func ForkDoltHubRepo(fromOrg, fromDB, toOrg, token string) error {
	body := map[string]string{
		"owner_name":     toOrg,
		"new_repo_name":  fromDB,
		"from_owner":     fromOrg,
		"from_repo_name": fromDB,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling fork request: %w", err)
	}

	url := dolthubAPIBase + "/database/fork"
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating fork request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authorization", "token "+token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DoltHub fork API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Parse error response
	var errResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&errResp); decErr == nil {
		// "already exists" is not an error — fork was already created
		if strings.Contains(strings.ToLower(errResp.Message), "already exists") {
			return nil
		}
		return fmt.Errorf("DoltHub fork API error (HTTP %d): %s", resp.StatusCode, errResp.Message)
	}
	return fmt.Errorf("DoltHub fork API error (HTTP %d)", resp.StatusCode)
}

// CloneLocally clones a DoltHub database to a local directory.
// Returns the absolute path to the clone.
func CloneLocally(org, db, targetDir string) error {
	remoteURL := fmt.Sprintf("%s/%s/%s", dolthubRemoteBase, org, db)

	if err := os.MkdirAll(filepath.Dir(targetDir), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// If directory already exists with a .dolt folder, skip clone
	if _, err := os.Stat(filepath.Join(targetDir, ".dolt")); err == nil {
		return nil
	}

	cmd := exec.Command("dolt", "clone", remoteURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dolt clone %s: %w (%s)", remoteURL, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// RegisterTown inserts a row into the towns table on the local clone.
// For Phase 1 (wild-west mode), writes directly to main.
func RegisterTown(localDir string, handle, dolthubOrg, displayName, ownerEmail, gtVersion string) error {
	sql := fmt.Sprintf(
		`INSERT INTO towns (handle, display_name, dolthub_org, owner_email, gt_version, trust_level, registered_at, last_seen) `+
			`VALUES ('%s', '%s', '%s', '%s', '%s', 1, NOW(), NOW()) `+
			`ON DUPLICATE KEY UPDATE last_seen = NOW(), gt_version = '%s'`,
		escapeSQLString(handle),
		escapeSQLString(displayName),
		escapeSQLString(dolthubOrg),
		escapeSQLString(ownerEmail),
		escapeSQLString(gtVersion),
		escapeSQLString(gtVersion),
	)

	cmd := exec.Command("dolt", "sql", "-q", sql)
	cmd.Dir = localDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inserting town registration: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	// Stage and commit
	addCmd := exec.Command("dolt", "add", ".")
	addCmd.Dir = localDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dolt add: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	commitCmd := exec.Command("dolt", "commit", "-m", fmt.Sprintf("Register town: %s", handle))
	commitCmd.Dir = localDir
	output, err = commitCmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "nothing to commit") || strings.Contains(lower, "no changes added") {
			return nil // already registered
		}
		return fmt.Errorf("dolt commit: %w (%s)", err, msg)
	}

	return nil
}

// PushToOrigin pushes the local clone to origin main.
func PushToOrigin(localDir string) error {
	cmd := exec.Command("dolt", "push", "origin", "main")
	cmd.Dir = localDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dolt push: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// AddUpstreamRemote adds the upstream commons as a remote named "upstream".
func AddUpstreamRemote(localDir, upstreamOrg, upstreamDB string) error {
	url := fmt.Sprintf("%s/%s/%s", dolthubRemoteBase, upstreamOrg, upstreamDB)

	// Check if upstream remote already exists
	checkCmd := exec.Command("dolt", "remote", "-v")
	checkCmd.Dir = localDir
	output, err := checkCmd.CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "upstream") {
				return nil // already exists
			}
		}
	}

	cmd := exec.Command("dolt", "remote", "add", "upstream", url)
	cmd.Dir = localDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(msg), "already exists") {
			return nil
		}
		return fmt.Errorf("dolt remote add upstream: %w (%s)", err, msg)
	}
	return nil
}

// WastelandDir returns the directory where wasteland data is stored for a town.
func WastelandDir(townRoot string) string {
	return filepath.Join(townRoot, ".wasteland")
}

// LocalCloneDir returns the local clone directory for a specific wasteland commons.
func LocalCloneDir(townRoot, upstreamOrg, upstreamDB string) string {
	return filepath.Join(WastelandDir(townRoot), upstreamOrg, upstreamDB)
}

// escapeSQLString escapes single quotes in SQL strings.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
