// Package daytona wraps the daytona CLI for workspace lifecycle management.
// All daytona interactions go through Client to centralize argument handling,
// workspace naming conventions, and multi-tenancy filtering.
package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	// Run executes a command and returns its stdout, stderr, exit code, and error.
	// On success or a normal non-zero exit, err is nil and exitCode holds the
	// process exit status. A non-nil err indicates an OS-level failure (e.g.
	// binary not found, signal); in that case exitCode is -1.
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// execRunner is the default CommandRunner that shells out to a real process.
type execRunner struct{}

func (r *execRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: callers validate args
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return outBuf.String(), errBuf.String(), -1, err
		}
	}
	return outBuf.String(), errBuf.String(), exitCode, nil
}

// Workspace represents a daytona workspace visible to this installation.
type Workspace struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Rig     string `json:"-"` // parsed from name
	Polecat string `json:"-"` // parsed from name
}

// WorkspaceInfo holds the richer per-workspace state from `daytona info -f json`.
// Fields beyond basic Workspace include network config, resource usage, and
// last activity timestamps — useful for zombie detection and smarter restart decisions.
type WorkspaceInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	State        string            `json:"state"`
	Repository   string            `json:"repository"`
	Branch       string            `json:"branch"`
	Target       string            `json:"target"`
	LastActivity string            `json:"last_activity"` // ISO 8601 timestamp or relative
	Resources    WorkspaceResources `json:"resources"`
	Network      WorkspaceNetwork   `json:"network"`
	Labels       map[string]string  `json:"labels"`
}

// WorkspaceResources captures resource allocation/usage from daytona info.
type WorkspaceResources struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
}

// WorkspaceNetwork captures network configuration from daytona info.
type WorkspaceNetwork struct {
	BlockAll  bool   `json:"block_all"`
	AllowList string `json:"allow_list"`
}

// CreateOptions configures workspace creation.
type CreateOptions struct {
	Dockerfile       string            // path to Dockerfile for sandbox snapshot (maps to --dockerfile)
	Snapshot         string            // pre-built snapshot ID (--snapshot flag)
	Target           string            // geographic region for workspace placement (maps to --target)
	Env              map[string]string // extra environment variables
	Labels           map[string]string // workspace labels (--label KEY=VALUE)
	Volumes          []string          // named volumes in "name:/mount/path" format (maps to --volume)
	Class            string            // resource tier: "small", "medium", "large" (maps to --class)
	CPU              int               // CPU cores (maps to --cpu)
	Memory           int               // memory in MB (maps to --memory)
	Disk             int               // disk size in GB (maps to --disk)
	NetworkBlockAll  bool              // block all outbound network (--network-block-all)
	NetworkAllowList string            // comma-separated CIDRs to allow (--network-allow-list)

	// AutoStopInterval is idle minutes before Daytona stops the workspace (0 = Daytona default).
	AutoStopInterval int
	// AutoArchiveInterval is minutes after stop before Daytona archives the workspace (0 = Daytona default).
	AutoArchiveInterval int
	// AutoDeleteInterval is minutes after archive before Daytona deletes the workspace (0 = Daytona default).
	AutoDeleteInterval int
}

// Client wraps the daytona CLI for workspace lifecycle and discovery.
type Client struct {
	installPrefix string // "gt-<installID-short>" — scopes workspaces to this installation
	runner        CommandRunner
	retry         RetryConfig
	listPageSize  int // page size for ListOwned pagination; 0 means use default
}

// NewClient creates a Client that scopes workspaces with the given prefix.
// The installPrefix is typically "gt-<first-12-chars-of-installationID>".
func NewClient(installPrefix string) *Client {
	return &Client{
		installPrefix: installPrefix,
		runner:        &execRunner{},
		retry:         DefaultRetryConfig(),
	}
}

// NewClientWithRunner creates a Client with a custom CommandRunner (for testing).
// Retry is disabled by default; use SetRetry to enable.
func NewClientWithRunner(installPrefix string, runner CommandRunner) *Client {
	return &Client{
		installPrefix: installPrefix,
		runner:        runner,
		retry:         NoRetryConfig(),
	}
}

// SetRetry configures the retry policy for transient CLI failures.
func (c *Client) SetRetry(cfg RetryConfig) {
	c.retry = cfg
}

// WorkspaceName returns the deterministic workspace name for a rig+polecat pair.
// Format: <installPrefix>-<rig>--<polecat>
// The double-hyphen delimiter allows both rig and polecat names to contain
// single hyphens (e.g., rig "my-rig", polecat "bullet-farmer").
func (c *Client) WorkspaceName(rig, polecat string) string {
	return c.installPrefix + "-" + rig + "--" + polecat
}

// ParseWorkspaceName extracts rig and polecat from a workspace name.
// Returns ok=false if the name doesn't match this installation's prefix
// or doesn't contain the "--" rig/polecat delimiter.
func (c *Client) ParseWorkspaceName(name string) (rig, polecat string, ok bool) {
	prefix := c.installPrefix + "-"
	if !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(name, prefix)
	// rest should be "<rig>--<polecat>" — split on LAST double-hyphen delimiter
	// to handle rig names that themselves contain "--".
	idx := strings.LastIndex(rest, "--")
	if idx <= 0 || idx >= len(rest)-2 {
		return "", "", false
	}
	return rest[:idx], rest[idx+2:], true
}

// Create provisions a new daytona sandbox.
// In v0.149+ the CLI no longer accepts a positional repo URL, --branch, --yes,
// or --image. Sandboxes are created from snapshots or Dockerfiles, with repo
// content injected via environment variables or post-create exec.
func (c *Client) Create(ctx context.Context, name, repoURL, branch string, opts CreateOptions) error {
	args := []string{"create", "--name", name}
	if opts.Snapshot != "" {
		args = append(args, "--snapshot", opts.Snapshot)
	}
	if opts.Dockerfile != "" {
		args = append(args, "--dockerfile", opts.Dockerfile)
	}
	if opts.Target != "" {
		args = append(args, "--target", opts.Target)
	}
	for _, vol := range opts.Volumes {
		args = append(args, "--volume", vol)
	}
	if opts.Class != "" {
		args = append(args, "--class", opts.Class)
	}
	if opts.CPU > 0 {
		args = append(args, "--cpu", fmt.Sprintf("%d", opts.CPU))
	}
	if opts.Memory > 0 {
		args = append(args, "--memory", fmt.Sprintf("%d", opts.Memory))
	}
	if opts.Disk > 0 {
		args = append(args, "--disk", fmt.Sprintf("%d", opts.Disk))
	}
	if opts.AutoStopInterval > 0 {
		args = append(args, "--auto-stop", fmt.Sprintf("%d", opts.AutoStopInterval))
	}
	if opts.AutoArchiveInterval > 0 {
		args = append(args, "--auto-archive", fmt.Sprintf("%d", opts.AutoArchiveInterval))
	}
	if opts.AutoDeleteInterval > 0 {
		args = append(args, "--auto-delete", fmt.Sprintf("%d", opts.AutoDeleteInterval))
	}
	// Inject repo URL and branch as env vars for post-create clone
	if repoURL != "" {
		if opts.Env == nil {
			opts.Env = make(map[string]string)
		}
		opts.Env["GT_REPO_URL"] = repoURL
		if branch != "" {
			opts.Env["GT_REPO_BRANCH"] = branch
		}
	}
	for k, v := range opts.Env {
		args = append(args, "--env", k+"="+v)
	}
	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}
	if opts.NetworkBlockAll {
		args = append(args, "--network-block-all")
	}
	if opts.NetworkAllowList != "" {
		args = append(args, "--network-allow-list", opts.NetworkAllowList)
	}
	_, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", args...)
	})
	if err != nil {
		return fmt.Errorf("daytona create: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("daytona create failed (exit %d): %s", exitCode, firstLine(stderr))
	}
	return nil
}

// Start ensures a sandbox is running.
func (c *Client) Start(ctx context.Context, name string) error {
	_, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "start", name)
	})
	if err != nil {
		return fmt.Errorf("daytona start: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("daytona start failed (exit %d): %s", exitCode, firstLine(stderr))
	}
	return nil
}

// Stop pauses a sandbox (preserves state for re-start).
func (c *Client) Stop(ctx context.Context, name string) error {
	_, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "stop", name)
	})
	if err != nil {
		return fmt.Errorf("daytona stop: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("daytona stop failed (exit %d): %s", exitCode, firstLine(stderr))
	}
	return nil
}

// Archive moves a stopped sandbox's filesystem to object storage at reduced cost.
// The sandbox must be stopped first; archiving a running sandbox is an error.
func (c *Client) Archive(ctx context.Context, name string) error {
	_, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "archive", name)
	})
	if err != nil {
		return fmt.Errorf("daytona archive: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("daytona archive failed (exit %d): %s", exitCode, firstLine(stderr))
	}
	return nil
}

// Delete permanently removes a sandbox.
func (c *Client) Delete(ctx context.Context, name string) error {
	_, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "delete", name)
	})
	if err != nil {
		return fmt.Errorf("daytona delete: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("daytona delete failed (exit %d): %s", exitCode, firstLine(stderr))
	}
	return nil
}

// snapshotEntry matches the JSON output of `daytona snapshot list -f json`.
type snapshotEntry struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	ImageName   string  `json:"imageName"`
	ErrorReason *string `json:"errorReason"`
}

// isHealthy returns true if the snapshot is in a usable, active state.
// Snapshots in transitional states ("removing", "creating") or error states
// are not considered healthy.
func (e *snapshotEntry) isHealthy() bool {
	return e.State == "active" && (e.ErrorReason == nil || *e.ErrorReason == "")
}

// findSnapshot returns the snapshot entry with the given name, or nil if not found.
func (c *Client) findSnapshot(ctx context.Context, name string) (*snapshotEntry, error) {
	stdout, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "snapshot", "list", "-f", "json")
	})
	if err != nil {
		return nil, fmt.Errorf("daytona snapshot list: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("daytona snapshot list failed (exit %d): %s", exitCode, firstLine(stderr))
	}

	var entries []snapshotEntry
	if strings.TrimSpace(stdout) == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("daytona snapshot list: parse JSON: %w", err)
	}
	for _, e := range entries {
		if e.Name == name {
			return &e, nil
		}
	}
	return nil, nil
}

// SnapshotExists checks whether a snapshot with the given name exists.
func (c *Client) SnapshotExists(ctx context.Context, name string) (bool, error) {
	entry, err := c.findSnapshot(ctx, name)
	if err != nil {
		return false, err
	}
	return entry != nil, nil
}

// deleteSnapshot removes a snapshot by its ID. Returns nil if the snapshot
// is already gone (not found).
func (c *Client) deleteSnapshot(ctx context.Context, id string) error {
	_, stderr, exitCode, err := c.runWithRetry(ctx, false, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "snapshot", "delete", id)
	})
	if err != nil {
		return fmt.Errorf("daytona snapshot delete: %w", err)
	}
	if exitCode != 0 {
		if strings.Contains(strings.ToLower(stderr), "not found") {
			return nil // already deleted
		}
		return fmt.Errorf("daytona snapshot delete failed (exit %d): %s", exitCode, firstLine(stderr))
	}
	return nil
}

// EnsureSnapshot ensures a healthy (active) snapshot exists for the given name.
//
// The lifecycle is:
//  1. If a snapshot exists and is active → reuse it.
//  2. If it exists but is errored → delete it, then create a new one.
//  3. If it is in a transitional state (pulling, creating, removing) → poll
//     until it settles, then re-evaluate.
//  4. If no snapshot exists → fire `daytona snapshot create` and poll until
//     the snapshot reaches "active" or "error".
//
// The create command may return before the snapshot is ready (async pull), so
// we always poll after creation rather than trusting the exit code alone.
func (c *Client) EnsureSnapshot(ctx context.Context, snapshotName, image string) error {
	const (
		pollInterval  = 5 * time.Second
		maxPollTime   = 10 * time.Minute
		maxCleanups   = 2
	)

	cleanups := 0
	for deadline := time.Now().Add(maxPollTime); ; {
		entry, err := c.findSnapshot(ctx, snapshotName)
		if err != nil {
			return fmt.Errorf("checking snapshot: %w", err)
		}

		if entry == nil {
			// No snapshot — create it.
			break
		}

		if entry.isHealthy() {
			return nil
		}

		if entry.State == "error" || (entry.ErrorReason != nil && *entry.ErrorReason != "") {
			cleanups++
			if cleanups > maxCleanups {
				reason := ""
				if entry.ErrorReason != nil {
					reason = *entry.ErrorReason
				}
				return fmt.Errorf("snapshot %q keeps erroring (last: %s)", snapshotName, reason)
			}
			if err := c.deleteSnapshot(ctx, entry.ID); err != nil {
				return fmt.Errorf("deleting errored snapshot %q: %w", snapshotName, err)
			}
			// Loop back to wait for deletion to take effect.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}

		// Transitional state (pulling, creating, removing, etc.) — wait.
		if time.Now().After(deadline) {
			return fmt.Errorf("snapshot %q stuck in state %q (waited %v)", snapshotName, entry.State, maxPollTime)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
		continue
	}

	// Fire the create. This may return immediately (async) or block until done.
	// Either way, we poll afterwards to confirm the snapshot reached "active".
	_, stderr, exitCode, err := c.runner.Run(ctx, "daytona", "snapshot", "create", snapshotName, "--image", image)
	if err != nil {
		return fmt.Errorf("daytona snapshot create: %w", err)
	}
	if exitCode != 0 {
		stderrLower := strings.ToLower(stderr)
		// "already exists" means a concurrent create or a zombie record. Re-enter
		// the poll loop — findSnapshot will pick it up and handle its state.
		if !strings.Contains(stderrLower, "already exists") {
			return fmt.Errorf("daytona snapshot create failed (exit %d): %s", exitCode, firstLine(stderr))
		}
	}

	// Poll until the snapshot is active or errors out.
	for deadline := time.Now().Add(maxPollTime); ; {
		entry, err := c.findSnapshot(ctx, snapshotName)
		if err != nil {
			return fmt.Errorf("polling snapshot after create: %w", err)
		}
		if entry == nil {
			return fmt.Errorf("snapshot %q disappeared after creation", snapshotName)
		}
		if entry.isHealthy() {
			return nil
		}
		if entry.State == "error" || (entry.ErrorReason != nil && *entry.ErrorReason != "") {
			reason := ""
			if entry.ErrorReason != nil {
				reason = *entry.ErrorReason
			}
			return fmt.Errorf("snapshot %q failed after creation: %s", snapshotName, reason)
		}
		// Still pulling/creating — keep waiting.
		if time.Now().After(deadline) {
			return fmt.Errorf("snapshot %q stuck in state %q after creation (waited %v)", snapshotName, entry.State, maxPollTime)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// Info returns detailed workspace information from `daytona info -f json <name>`.
// This provides richer state than ListOwned (network config, last activity, resources)
// and is useful for finer-grained restart and zombie-detection decisions.
func (c *Client) Info(ctx context.Context, name string) (*WorkspaceInfo, error) {
	stdout, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", "info", "-f", "json", name)
	})
	if err != nil {
		return nil, fmt.Errorf("daytona info: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("daytona info failed (exit %d): %s", exitCode, firstLine(stderr))
	}

	var info WorkspaceInfo
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return nil, fmt.Errorf("daytona info: parse JSON: %w", err)
	}
	return &info, nil
}

// ExecOptions configures optional behaviour for Exec calls.
type ExecOptions struct {
	// Env sets environment variables inside the workspace via an inline
	// `env K=V` prefix (daytona exec does not support --env).
	Env map[string]string

	// Cwd sets the working directory for the command via --cwd.
	Cwd string

	// TTY allocates a pseudo-terminal for the exec session via --tty.
	// Use for interactive commands or when proper argument parsing is needed.
	TTY bool
}

// Exec runs a command inside a workspace and returns stdout, stderr, and exit code.
// Retries on OS-level errors (e.g., daytona binary I/O failure) but not on non-zero
// exit codes, which belong to the command running inside the workspace.
//
// Environment variables are injected via an inline `env K=V` prefix rather than
// --env flags, because daytona exec does not support --env.
func (c *Client) Exec(ctx context.Context, name string, env map[string]string, cmd ...string) (string, string, int, error) {
	return c.ExecWithOptions(ctx, name, ExecOptions{Env: env}, cmd...)
}

// ExecWithOptions is like Exec but accepts an ExecOptions struct for extended
// configuration such as --cwd.
func (c *Client) ExecWithOptions(ctx context.Context, name string, opts ExecOptions, cmd ...string) (string, string, int, error) {
	args := []string{"exec", name}
	if opts.TTY {
		args = append(args, "--tty")
	}
	if opts.Cwd != "" {
		args = append(args, "--cwd", opts.Cwd)
	}
	args = append(args, "--")
	if len(opts.Env) > 0 {
		// daytona exec does not support --env; use the env command to set
		// variables inline inside the container.
		args = append(args, "env")
		// Sort keys for deterministic command output.
		keys := make([]string, 0, len(opts.Env))
		for k := range opts.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, k+"="+opts.Env[k])
		}
	}
	args = append(args, cmd...)
	stdout, stderr, exitCode, err := c.runWithRetry(ctx, false, func() (string, string, int, error) {
		return c.runner.Run(ctx, "daytona", args...)
	})
	if err != nil {
		return "", "", -1, fmt.Errorf("daytona exec: %w", err)
	}
	return stdout, stderr, exitCode, nil
}

// daytonaListEntry matches a single workspace in the JSON output of `daytona list -f json`.
type daytonaListEntry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// daytonaListResponse wraps the paginated JSON output of `daytona list -f json`.
// Daytona changed from returning a bare array to {"items":[...], "total":N, ...}.
type daytonaListResponse struct {
	Items      []daytonaListEntry `json:"items"`
	Total      int                `json:"total"`
	TotalPages int                `json:"totalPages"`
}

// defaultListPageSize is the number of workspaces fetched per page in ListOwned.
const defaultListPageSize = 100

// ListOwned returns all workspaces belonging to this installation (filtered by installPrefix).
// It paginates through all pages of `daytona list` to ensure no workspaces are missed.
func (c *Client) ListOwned(ctx context.Context) ([]Workspace, error) {
	prefix := c.installPrefix + "-"
	var owned []Workspace

	pageSize := c.listPageSize
	if pageSize <= 0 {
		pageSize = defaultListPageSize
	}

	for page := 1; ; page++ {
		pageStr := fmt.Sprintf("%d", page)
		limitStr := fmt.Sprintf("%d", pageSize)
		stdout, stderr, exitCode, err := c.runWithRetry(ctx, true, func() (string, string, int, error) {
			return c.runner.Run(ctx, "daytona", "list", "-f", "json", "-p", pageStr, "-l", limitStr)
		})
		if err != nil {
			return nil, fmt.Errorf("daytona list (page %d): %w", page, err)
		}
		if exitCode != 0 {
			return nil, fmt.Errorf("daytona list failed (page %d, exit %d): %s", page, exitCode, firstLine(stderr))
		}

		if strings.TrimSpace(stdout) == "" {
			break
		}

		// Daytona returns {"items":[...], ...} (wrapped) or [...] (legacy bare array).
		var entries []daytonaListEntry
		trimmed := strings.TrimSpace(stdout)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			var resp daytonaListResponse
			if err := json.Unmarshal([]byte(trimmed), &resp); err != nil {
				return nil, fmt.Errorf("daytona list: parse JSON (page %d): %w", page, err)
			}
			entries = resp.Items
		} else {
			if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
				return nil, fmt.Errorf("daytona list: parse JSON (page %d): %w", page, err)
			}
		}
		if len(entries) == 0 {
			break
		}

		for _, e := range entries {
			if !strings.HasPrefix(e.Name, prefix) {
				continue
			}
			ws := Workspace{
				ID:    e.ID,
				Name:  e.Name,
				State: e.State,
			}
			if rig, polecat, ok := c.ParseWorkspaceName(e.Name); ok {
				ws.Rig = rig
				ws.Polecat = polecat
			}
			owned = append(owned, ws)
		}

		if len(entries) < pageSize {
			break
		}
	}
	return owned, nil
}

// InstallPrefix returns the prefix used for workspace name scoping.
func (c *Client) InstallPrefix() string {
	return c.installPrefix
}

// CertVolumeName returns the deterministic volume name for shared cert storage.
// Format: gt-certs-<installPrefix>
// All workspaces in the same installation share this volume, so certs persist
// across workspace restarts and new workspace creation.
func (c *Client) CertVolumeName() string {
	return "gt-certs-" + c.installPrefix
}

// firstLine returns the first non-empty line from s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return strings.TrimSpace(s)
}
