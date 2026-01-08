// Package daytona provides a Go client for the Daytona API.
// This client is generated based on the Daytona OpenAPI 3.1.0 specification.
package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultBaseURL is the default Daytona API base URL (for sandbox management).
	DefaultBaseURL = "https://app.daytona.io/api"

	// FallbackToolboxURL is used if fetching config fails.
	FallbackToolboxURL = "https://proxy.app.daytona.io/toolbox"

	// DefaultTimeout is the default HTTP client timeout.
	DefaultTimeout = 30 * time.Second
)

// Client is a Daytona API client.
type Client struct {
	baseURL    string // For sandbox management (create, list, delete)
	toolboxURL string // For toolbox operations (PTY, execute, files)
	apiKey     string
	httpClient *http.Client
	orgID      string // Optional organization ID
}

// DaytonaConfig represents the server configuration returned by /api/config.
type DaytonaConfig struct {
	ProxyToolboxURL string `json:"proxyToolboxUrl"`
}

// ClientOption configures the Daytona client.
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithOrganizationID sets the organization ID header.
func WithOrganizationID(orgID string) ClientOption {
	return func(c *Client) {
		c.orgID = orgID
	}
}

// NewClient creates a new Daytona API client.
// apiKey can be empty if DAYTONA_API_KEY environment variable is set.
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		apiKey = os.Getenv("DAYTONA_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Daytona API key is required (set DAYTONA_API_KEY or pass apiKey)")
	}

	c := &Client{
		baseURL:    DefaultBaseURL,
		toolboxURL: FallbackToolboxURL, // Will be updated from config
		apiKey:     apiKey,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	// Fetch toolbox URL from config (use fallback on error)
	if config, err := c.fetchConfig(); err == nil && config.ProxyToolboxURL != "" {
		c.toolboxURL = config.ProxyToolboxURL
	}

	return c, nil
}

// fetchConfig fetches the server configuration to get dynamic URLs.
func (c *Client) fetchConfig() (*DaytonaConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.doRequest(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return nil, err
	}

	var config DaytonaConfig
	if err := parseResponse(resp, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// doRequest performs an HTTP request with authentication using the base API URL.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	return c.doRequestWithURL(ctx, c.baseURL, method, path, body)
}

// doToolboxRequest performs an HTTP request for toolbox operations using the toolbox URL.
func (c *Client) doToolboxRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	return c.doRequestWithURL(ctx, c.toolboxURL, method, path, body)
}

// doRequestWithURL performs an HTTP request with authentication using a specific base URL.
func (c *Client) doRequestWithURL(ctx context.Context, baseURL, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.orgID != "" {
		req.Header.Set("X-Daytona-Organization-ID", c.orgID)
	}

	return c.httpClient.Do(req)
}

// parseResponse parses an HTTP response into the given target.
func parseResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}
		apiErr.StatusCode = resp.StatusCode
		return &apiErr
	}

	if target != nil && len(body) > 0 {
		if err := json.Unmarshal(body, target); err != nil {
			return fmt.Errorf("parsing response: %w", err)
		}
	}

	return nil
}

// APIError represents a Daytona API error.
type APIError struct {
	StatusCode int    `json:"-"`
	Message    string `json:"message"`
	Error_     string `json:"error"`
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Error_
	}
	return fmt.Sprintf("Daytona API error (status %d): %s", e.StatusCode, msg)
}

// --- Sandbox Types ---

// SandboxState represents the state of a sandbox.
type SandboxState string

const (
	SandboxStateCreating         SandboxState = "creating"
	SandboxStateRestoring        SandboxState = "restoring"
	SandboxStateDestroyed        SandboxState = "destroyed"
	SandboxStateDestroying       SandboxState = "destroying"
	SandboxStateStarted          SandboxState = "started"
	SandboxStateStopped          SandboxState = "stopped"
	SandboxStateStarting         SandboxState = "starting"
	SandboxStateStopping         SandboxState = "stopping"
	SandboxStateError            SandboxState = "error"
	SandboxStateBuildFailed      SandboxState = "build_failed"
	SandboxStatePendingBuild     SandboxState = "pending_build"
	SandboxStateBuildingSnapshot SandboxState = "building_snapshot"
	SandboxStateUnknown          SandboxState = "unknown"
	SandboxStatePullingSnapshot  SandboxState = "pulling_snapshot"
	SandboxStateArchived         SandboxState = "archived"
	SandboxStateArchiving        SandboxState = "archiving"
)

// IsRunning returns true if the sandbox is in a running state.
func (s SandboxState) IsRunning() bool {
	return s == SandboxStateStarted
}

// IsTerminal returns true if the sandbox is in a terminal state.
func (s SandboxState) IsTerminal() bool {
	return s == SandboxStateDestroyed || s == SandboxStateError || s == SandboxStateBuildFailed
}

// Sandbox represents a Daytona sandbox.
type Sandbox struct {
	ID                  string            `json:"id"`
	OrganizationID      string            `json:"organizationId"`
	Name                string            `json:"name"`
	Snapshot            string            `json:"snapshot,omitempty"`
	User                string            `json:"user"`
	Env                 map[string]string `json:"env"`
	Labels              map[string]string `json:"labels"`
	Public              bool              `json:"public"`
	NetworkBlockAll     bool              `json:"networkBlockAll"`
	NetworkAllowList    string            `json:"networkAllowList,omitempty"`
	Target              string            `json:"target"`
	CPU                 float64           `json:"cpu"`
	GPU                 float64           `json:"gpu"`
	Memory              float64           `json:"memory"`
	Disk                float64           `json:"disk"`
	State               SandboxState      `json:"state,omitempty"`
	DesiredState        string            `json:"desiredState,omitempty"`
	ErrorReason         string            `json:"errorReason,omitempty"`
	Recoverable         bool              `json:"recoverable,omitempty"`
	AutoStopInterval    int               `json:"autoStopInterval,omitempty"`
	AutoArchiveInterval int               `json:"autoArchiveInterval,omitempty"`
	AutoDeleteInterval  int               `json:"autoDeleteInterval,omitempty"`
	CreatedAt           string            `json:"createdAt,omitempty"`
	UpdatedAt           string            `json:"updatedAt,omitempty"`
}

// CreateSandboxRequest represents a request to create a sandbox.
type CreateSandboxRequest struct {
	Name                string            `json:"name,omitempty"`
	Snapshot            string            `json:"snapshot,omitempty"`
	User                string            `json:"user,omitempty"`
	Env                 map[string]string `json:"env,omitempty"`
	Labels              map[string]string `json:"labels,omitempty"`
	Public              bool              `json:"public,omitempty"`
	NetworkBlockAll     bool              `json:"networkBlockAll,omitempty"`
	NetworkAllowList    string            `json:"networkAllowList,omitempty"`
	Class               string            `json:"class,omitempty"` // small, medium, large
	Target              string            `json:"target,omitempty"`
	CPU                 int               `json:"cpu,omitempty"`
	GPU                 int               `json:"gpu,omitempty"`
	Memory              int               `json:"memory,omitempty"` // GB
	Disk                int               `json:"disk,omitempty"`   // GB
	AutoStopInterval    int               `json:"autoStopInterval,omitempty"`
	AutoArchiveInterval int               `json:"autoArchiveInterval,omitempty"`
	AutoDeleteInterval  int               `json:"autoDeleteInterval,omitempty"`
}

// --- Sandbox API Methods ---

// CreateSandbox creates a new sandbox.
func (c *Client) CreateSandbox(ctx context.Context, req *CreateSandboxRequest) (*Sandbox, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/sandbox", req)
	if err != nil {
		return nil, err
	}

	var sandbox Sandbox
	if err := parseResponse(resp, &sandbox); err != nil {
		return nil, err
	}

	return &sandbox, nil
}

// GetSandbox retrieves a sandbox by ID or name.
func (c *Client) GetSandbox(ctx context.Context, idOrName string) (*Sandbox, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/sandbox/"+idOrName, nil)
	if err != nil {
		return nil, err
	}

	var sandbox Sandbox
	if err := parseResponse(resp, &sandbox); err != nil {
		return nil, err
	}

	return &sandbox, nil
}

// ListSandboxes lists all sandboxes.
func (c *Client) ListSandboxes(ctx context.Context) ([]Sandbox, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/sandbox", nil)
	if err != nil {
		return nil, err
	}

	var sandboxes []Sandbox
	if err := parseResponse(resp, &sandboxes); err != nil {
		return nil, err
	}

	return sandboxes, nil
}

// DeleteSandbox deletes a sandbox.
func (c *Client) DeleteSandbox(ctx context.Context, idOrName string) (*Sandbox, error) {
	resp, err := c.doRequest(ctx, http.MethodDelete, "/sandbox/"+idOrName, nil)
	if err != nil {
		return nil, err
	}

	var sandbox Sandbox
	if err := parseResponse(resp, &sandbox); err != nil {
		return nil, err
	}

	return &sandbox, nil
}

// StartSandbox starts a sandbox.
func (c *Client) StartSandbox(ctx context.Context, idOrName string) (*Sandbox, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/sandbox/"+idOrName+"/start", nil)
	if err != nil {
		return nil, err
	}

	var sandbox Sandbox
	if err := parseResponse(resp, &sandbox); err != nil {
		return nil, err
	}

	return &sandbox, nil
}

// StopSandbox stops a sandbox.
func (c *Client) StopSandbox(ctx context.Context, idOrName string) (*Sandbox, error) {
	resp, err := c.doRequest(ctx, http.MethodPost, "/sandbox/"+idOrName+"/stop", nil)
	if err != nil {
		return nil, err
	}

	var sandbox Sandbox
	if err := parseResponse(resp, &sandbox); err != nil {
		return nil, err
	}

	return &sandbox, nil
}

// --- Process Execution Types ---

// ExecuteRequest represents a request to execute a command.
type ExecuteRequest struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds, defaults to 10
}

// ExecuteResponse represents the response from executing a command.
type ExecuteResponse struct {
	ExitCode int    `json:"exitCode"`
	Result   string `json:"result"`
}

// PtyCreateRequest represents a request to create a PTY session.
type PtyCreateRequest struct {
	ID        string            `json:"id"`
	Cwd       string            `json:"cwd,omitempty"`
	Envs      map[string]string `json:"envs,omitempty"`
	Cols      int               `json:"cols,omitempty"`
	Rows      int               `json:"rows,omitempty"`
	LazyStart bool              `json:"lazyStart,omitempty"`
}

// PtyCreateResponse represents the response from creating a PTY session.
type PtyCreateResponse struct {
	ID  string `json:"id"`
	Url string `json:"url,omitempty"`
}

// PtySessionInfo represents PTY session information.
type PtySessionInfo struct {
	ID     string `json:"id"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Status string `json:"status"`
}

// --- Toolbox API Methods ---

// ExecuteCommand executes a command synchronously in a sandbox.
func (c *Client) ExecuteCommand(ctx context.Context, sandboxID string, req *ExecuteRequest) (*ExecuteResponse, error) {
	path := fmt.Sprintf("/%s/process/execute", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}

	var result ExecuteResponse
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// CreatePtySession creates a PTY session in a sandbox.
func (c *Client) CreatePtySession(ctx context.Context, sandboxID string, req *PtyCreateRequest) (*PtyCreateResponse, error) {
	path := fmt.Sprintf("/%s/process/pty", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return nil, err
	}

	var result PtyCreateResponse
	if err := parseResponse(resp, &result); err != nil {
		// API may return empty body on success - use request ID
		result.ID = req.ID
	}

	// If response ID is still empty, use request ID
	if result.ID == "" {
		result.ID = req.ID
	}

	return &result, nil
}

// GetPtySession gets PTY session information.
func (c *Client) GetPtySession(ctx context.Context, sandboxID, sessionID string) (*PtySessionInfo, error) {
	path := fmt.Sprintf("/%s/process/pty/%s", sandboxID, sessionID)
	resp, err := c.doToolboxRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var result PtySessionInfo
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeletePtySession deletes a PTY session.
func (c *Client) DeletePtySession(ctx context.Context, sandboxID, sessionID string) error {
	path := fmt.Sprintf("/%s/process/pty/%s", sandboxID, sessionID)
	resp, err := c.doToolboxRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	return parseResponse(resp, nil)
}

// --- Git Types ---

// GitCloneRequest represents a request to clone a git repository.
type GitCloneRequest struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Branch   string `json:"branch,omitempty"`
	CommitID string `json:"commit_id,omitempty"`
}

// GitCheckoutRequest represents a request to checkout a branch or commit.
type GitCheckoutRequest struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
}

// GitStatus represents the status of a git repository.
type GitStatus struct {
	CurrentBranch   string          `json:"currentBranch"`
	Ahead           int             `json:"ahead"`
	Behind          int             `json:"behind"`
	BranchPublished bool            `json:"branchPublished"`
	FileStatus      []GitFileStatus `json:"fileStatus"`
}

// FileStatus represents the git status of a file.
type FileStatus string

const (
	FileStatusUnmodified         FileStatus = "Unmodified"
	FileStatusUntracked          FileStatus = "Untracked"
	FileStatusModified           FileStatus = "Modified"
	FileStatusAdded              FileStatus = "Added"
	FileStatusDeleted            FileStatus = "Deleted"
	FileStatusRenamed            FileStatus = "Renamed"
	FileStatusCopied             FileStatus = "Copied"
	FileStatusUpdatedButUnmerged FileStatus = "Updated but unmerged"
)

// GitFileStatus represents the status of a file in a git repository.
type GitFileStatus struct {
	Name     string     `json:"name"`
	Staging  FileStatus `json:"staging"`  // Status in staging area
	Worktree FileStatus `json:"worktree"` // Status in working tree
	Extra    string     `json:"extra,omitempty"`
}

// GitPullPushRequest represents a request to pull or push changes.
type GitPullPushRequest struct {
	Path     string `json:"path"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// GitAddRequest represents a request to stage files.
type GitAddRequest struct {
	Path  string   `json:"path"`
	Files []string `json:"files"`
}

// GitCommitRequest represents a request to commit changes.
type GitCommitRequest struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Email   string `json:"email"`
}

// GitBranchRequest represents a request to create or delete a branch.
type GitBranchRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// GitBranch represents a git branch.
type GitBranch struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"isCurrent,omitempty"`
}

// GitBranchesResponse represents the response from listing branches.
type GitBranchesResponse struct {
	Branches []string `json:"branches"`
}

// --- Git API Methods ---

// GitClone clones a git repository into a sandbox.
func (c *Client) GitClone(ctx context.Context, sandboxID string, req *GitCloneRequest) error {
	path := fmt.Sprintf("/%s/git/clone", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitCheckout checks out a branch or commit.
func (c *Client) GitCheckout(ctx context.Context, sandboxID string, req *GitCheckoutRequest) error {
	path := fmt.Sprintf("/%s/git/checkout", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitStatus gets the status of a git repository.
func (c *Client) GitStatus(ctx context.Context, sandboxID, repoPath string) (*GitStatus, error) {
	path := fmt.Sprintf("/%s/git/status?path=%s", sandboxID, repoPath)
	resp, err := c.doToolboxRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var status GitStatus
	if err := parseResponse(resp, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// GitPull pulls changes from remote.
func (c *Client) GitPull(ctx context.Context, sandboxID string, req *GitPullPushRequest) error {
	path := fmt.Sprintf("/%s/git/pull", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitPush pushes changes to remote.
func (c *Client) GitPush(ctx context.Context, sandboxID string, req *GitPullPushRequest) error {
	path := fmt.Sprintf("/%s/git/push", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitAdd stages files for the next commit.
func (c *Client) GitAdd(ctx context.Context, sandboxID string, req *GitAddRequest) error {
	path := fmt.Sprintf("/%s/git/add", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitCommit commits staged changes.
func (c *Client) GitCommit(ctx context.Context, sandboxID string, req *GitCommitRequest) error {
	path := fmt.Sprintf("/%s/git/commit", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitBranches lists all branches in a repository.
func (c *Client) GitBranches(ctx context.Context, sandboxID, repoPath string) ([]GitBranch, error) {
	path := fmt.Sprintf("/%s/git/branches?path=%s", sandboxID, url.QueryEscape(repoPath))
	resp, err := c.doToolboxRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var response GitBranchesResponse
	if err := parseResponse(resp, &response); err != nil {
		return nil, err
	}

	// Convert string slice to GitBranch slice
	branches := make([]GitBranch, len(response.Branches))
	for i, name := range response.Branches {
		branches[i] = GitBranch{Name: name}
	}
	return branches, nil
}

// GitCreateBranch creates a new branch.
func (c *Client) GitCreateBranch(ctx context.Context, sandboxID string, req *GitBranchRequest) error {
	path := fmt.Sprintf("/%s/git/branches", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// GitDeleteBranch deletes a branch.
func (c *Client) GitDeleteBranch(ctx context.Context, sandboxID string, req *GitBranchRequest) error {
	path := fmt.Sprintf("/%s/git/branches", sandboxID)
	resp, err := c.doToolboxRequest(ctx, http.MethodDelete, path, req)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// --- File System Types ---

// FileInfo represents information about a file in a sandbox.
type FileInfo struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	Mode    string `json:"mode"`
}

// --- File System API Methods ---

// UploadFile uploads a single file to a sandbox.
// localPath is the path to the local file, remotePath is the destination path in the sandbox.
func (c *Client) UploadFile(ctx context.Context, sandboxID, localPath, remotePath string) error {
	// Open the local file
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file field
	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copying file content: %w", err)
	}
	writer.Close()

	// Build URL with path query parameter (using toolbox URL)
	path := fmt.Sprintf("/%s/files/upload?path=%s", sandboxID, url.QueryEscape(remotePath))

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.toolboxURL+path, &buf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.orgID != "" {
		req.Header.Set("X-Daytona-Organization-ID", c.orgID)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	return nil
}

// DownloadFile downloads a file from a sandbox.
// remotePath is the path in the sandbox, localPath is where to save locally.
func (c *Client) DownloadFile(ctx context.Context, sandboxID, remotePath, localPath string) error {
	// Build URL with path query parameter
	path := fmt.Sprintf("/%s/files/download?path=%s", sandboxID, url.QueryEscape(remotePath))

	resp, err := c.doToolboxRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create local file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating local file: %w", err)
	}
	defer file.Close()

	// Copy content
	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// ListFiles lists files in a directory in the sandbox.
func (c *Client) ListFiles(ctx context.Context, sandboxID, dirPath string) ([]FileInfo, error) {
	path := fmt.Sprintf("/%s/files?path=%s", sandboxID, url.QueryEscape(dirPath))
	resp, err := c.doToolboxRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	if err := parseResponse(resp, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// DeleteFile deletes a file or directory in the sandbox.
func (c *Client) DeleteFile(ctx context.Context, sandboxID, filePath string, recursive bool) error {
	path := fmt.Sprintf("/%s/files?path=%s&recursive=%t", sandboxID, url.QueryEscape(filePath), recursive)
	resp, err := c.doToolboxRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// CreateFolder creates a directory in the sandbox.
func (c *Client) CreateFolder(ctx context.Context, sandboxID, folderPath string, mode string) error {
	path := fmt.Sprintf("/%s/files/folder?path=%s", sandboxID, url.QueryEscape(folderPath))
	if mode != "" {
		path += "&mode=" + url.QueryEscape(mode)
	}
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	return parseResponse(resp, nil)
}

// --- Helper Methods ---

// WaitForSandboxState waits for a sandbox to reach a specific state.
func (c *Client) WaitForSandboxState(ctx context.Context, idOrName string, targetState SandboxState, timeout time.Duration) (*Sandbox, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		sandbox, err := c.GetSandbox(ctx, idOrName)
		if err != nil {
			return nil, err
		}

		if sandbox.State == targetState {
			return sandbox, nil
		}

		if sandbox.State.IsTerminal() && sandbox.State != targetState {
			return sandbox, fmt.Errorf("sandbox reached terminal state %s (expected %s): %s",
				sandbox.State, targetState, sandbox.ErrorReason)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return nil, fmt.Errorf("timeout waiting for sandbox state %s", targetState)
}

// WaitForSandboxStarted waits for a sandbox to be started.
func (c *Client) WaitForSandboxStarted(ctx context.Context, idOrName string, timeout time.Duration) (*Sandbox, error) {
	return c.WaitForSandboxState(ctx, idOrName, SandboxStateStarted, timeout)
}
