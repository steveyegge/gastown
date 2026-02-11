package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CoopBackend wraps the Coop HTTP API to implement Backend.
// This replaces tmux/screen with Coop's PTY-based agent management.
//
// Each CoopBackend connects to a single Coop instance (one agent pod).
// The session parameter in Backend methods is used to select which Coop
// instance to talk to when multiple are registered via AddSession.
type CoopBackend struct {
	client *http.Client

	// mu protects sessions map.
	mu sync.RWMutex
	// sessions maps session name → Coop base URL (e.g., "http://localhost:8080").
	sessions map[string]string

	// token is the optional auth token for Coop API.
	token string
}

// CoopConfig configures a CoopBackend.
type CoopConfig struct {
	// Timeout for HTTP requests to Coop.
	Timeout time.Duration

	// Token is the optional bearer token for authenticated endpoints.
	Token string
}

// NewCoopBackend creates a Backend backed by Coop HTTP API.
func NewCoopBackend(cfg CoopConfig) *CoopBackend {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &CoopBackend{
		client: &http.Client{
			Timeout: timeout,
		},
		sessions: make(map[string]string),
		token:    cfg.Token,
	}
}

// AddSession registers a Coop instance for the given session name.
// The baseURL should be the Coop HTTP endpoint (e.g., "http://localhost:8080").
func (b *CoopBackend) AddSession(session, baseURL string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sessions[session] = strings.TrimRight(baseURL, "/")
}

// RemoveSession unregisters a Coop instance.
func (b *CoopBackend) RemoveSession(session string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.sessions, session)
}

// baseURL returns the Coop base URL for a session, or error if not registered.
func (b *CoopBackend) baseURL(session string) (string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	url, ok := b.sessions[session]
	if !ok {
		return "", fmt.Errorf("coop: no session registered for %q", session)
	}
	return url, nil
}

// doRequest builds and executes an HTTP request against a Coop endpoint.
func (b *CoopBackend) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
	return b.client.Do(req)
}

// coopHealthResponse mirrors Coop's HealthResponse.
type coopHealthResponse struct {
	Status string `json:"status"`
	PID    *int32 `json:"pid"`
	Ready  bool   `json:"ready"`
}

func (b *CoopBackend) HasSession(session string) (bool, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return false, nil // Not registered → not running
	}

	resp, err := b.doRequest("GET", base+"/api/v1/health", nil)
	if err != nil {
		return false, nil // Unreachable → not running
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var health coopHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return false, fmt.Errorf("coop: parsing health response: %w", err)
	}

	return health.Status == "running" && health.PID != nil, nil
}

func (b *CoopBackend) CapturePane(session string, lines int) (string, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return "", err
	}

	resp, err := b.doRequest("GET", base+"/api/v1/screen/text", nil)
	if err != nil {
		return "", fmt.Errorf("coop: screen/text request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("coop: screen/text returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("coop: reading screen/text: %w", err)
	}

	text := string(body)

	// Trim to last N lines if requested (Coop returns full screen).
	if lines > 0 {
		allLines := strings.Split(text, "\n")
		if len(allLines) > lines {
			allLines = allLines[len(allLines)-lines:]
		}
		text = strings.Join(allLines, "\n")
	}

	return text, nil
}

func (b *CoopBackend) CapturePaneAll(session string) (string, error) {
	// Coop returns the full screen content; lines=0 means no truncation.
	return b.CapturePane(session, 0)
}

func (b *CoopBackend) CapturePaneLines(session string, lines int) ([]string, error) {
	out, err := b.CapturePane(session, lines)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// coopNudgeRequest mirrors Coop's NudgeRequest.
type coopNudgeRequest struct {
	Message string `json:"message"`
}

// coopNudgeResponse mirrors Coop's NudgeResponse.
type coopNudgeResponse struct {
	Delivered   bool    `json:"delivered"`
	StateBefore *string `json:"state_before"`
	Reason      *string `json:"reason"`
}

func (b *CoopBackend) NudgeSession(session string, message string) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(coopNudgeRequest{Message: message})
	if err != nil {
		return err
	}

	resp, err := b.doRequest("POST", base+"/api/v1/agent/nudge", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: nudge request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: nudge returned %d: %s", resp.StatusCode, string(body))
	}

	var nudgeResp coopNudgeResponse
	if err := json.NewDecoder(resp.Body).Decode(&nudgeResp); err != nil {
		return fmt.Errorf("coop: parsing nudge response: %w", err)
	}

	if !nudgeResp.Delivered {
		reason := "unknown"
		if nudgeResp.Reason != nil {
			reason = *nudgeResp.Reason
		}
		return fmt.Errorf("coop: nudge not delivered: %s", reason)
	}

	return nil
}

// coopKeysRequest mirrors Coop's KeysRequest.
type coopKeysRequest struct {
	Keys []string `json:"keys"`
}

func (b *CoopBackend) SendKeys(session string, keys string) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	// Split keys string into individual key names.
	// Coop expects an array of key names like ["Enter", "Escape"].
	keyList := strings.Fields(keys)
	if len(keyList) == 0 {
		return nil
	}

	payload, err := json.Marshal(coopKeysRequest{Keys: keyList})
	if err != nil {
		return err
	}

	resp, err := b.doRequest("POST", base+"/api/v1/input/keys", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: input/keys request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: input/keys returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (b *CoopBackend) IsPaneDead(session string) (bool, error) {
	state, err := b.AgentState(session)
	if err != nil {
		return false, err
	}
	// In coop, "exited" or "crashed" agent states are equivalent to a dead pane.
	return state.State == "exited" || state.State == "crashed", nil
}

func (b *CoopBackend) SetPaneDiedHook(session, agentID string) error {
	// No-op for coop: coop manages agent lifecycle internally.
	// Crash detection is handled by polling AgentState via IsPaneDead.
	return nil
}

// AgentState returns the current agent state from Coop.
// This is an extension beyond the Backend interface for richer state tracking.
func (b *CoopBackend) AgentState(session string) (*CoopAgentState, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return nil, err
	}

	resp, err := b.doRequest("GET", base+"/api/v1/agent/state", nil)
	if err != nil {
		return nil, fmt.Errorf("coop: agent/state request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("coop: agent/state returned %d: %s", resp.StatusCode, string(body))
	}

	var state CoopAgentState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("coop: parsing agent/state response: %w", err)
	}

	return &state, nil
}

// RespondToPrompt sends a response to an active agent prompt via Coop.
// This is an extension beyond the Backend interface for prompt handling.
func (b *CoopBackend) RespondToPrompt(session string, req CoopRespondRequest) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := b.doRequest("POST", base+"/api/v1/agent/respond", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: agent/respond request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: agent/respond returned %d: %s", resp.StatusCode, string(body))
	}

	var respondResp CoopRespondResponse
	if err := json.NewDecoder(resp.Body).Decode(&respondResp); err != nil {
		return fmt.Errorf("coop: parsing respond response: %w", err)
	}

	if !respondResp.Delivered {
		reason := "unknown"
		if respondResp.Reason != nil {
			reason = *respondResp.Reason
		}
		return fmt.Errorf("coop: respond not delivered: %s", reason)
	}

	return nil
}

// CoopAgentState mirrors Coop's AgentStateResponse.
type CoopAgentState struct {
	Agent         string        `json:"agent"`
	State         string        `json:"state"`
	SinceSeq      uint64        `json:"since_seq"`
	ScreenSeq     uint64        `json:"screen_seq"`
	DetectionTier string        `json:"detection_tier"`
	Prompt        *PromptContext `json:"prompt,omitempty"`
	ErrorDetail   *string       `json:"error_detail,omitempty"`
	ErrorCategory *string       `json:"error_category,omitempty"`
}

// PromptContext describes the current prompt the agent is showing.
type PromptContext struct {
	Type    string   `json:"type,omitempty"`
	Message string   `json:"message,omitempty"`
	Options []string `json:"options,omitempty"`
}

// CoopRespondRequest mirrors Coop's RespondRequest.
type CoopRespondRequest struct {
	Accept *bool  `json:"accept,omitempty"`
	Option *int   `json:"option,omitempty"`
	Text   *string `json:"text,omitempty"`
}

// CoopRespondResponse mirrors Coop's RespondResponse.
type CoopRespondResponse struct {
	Delivered  bool    `json:"delivered"`
	PromptType *string `json:"prompt_type,omitempty"`
	Reason     *string `json:"reason,omitempty"`
}

// coopStatusResponse mirrors Coop's StatusResponse from /api/v1/status.
type coopStatusResponse struct {
	State    string `json:"state"`
	PID      *int32 `json:"pid,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

// coopSignalRequest is the body for POST /api/v1/signal.
type coopSignalRequest struct {
	Signal string `json:"signal"`
}

// coopInputRequest is the body for POST /api/v1/input.
type coopInputRequest struct {
	Text  string `json:"text"`
	Enter bool   `json:"enter"`
}

// coopSwitchRequest is the body for PUT /api/v1/session/switch.
type coopSwitchRequest struct {
	ExtraEnv map[string]string `json:"extra_env,omitempty"`
}

// coopEnvResponse is returned by GET /api/v1/env/:key.
type coopEnvResponse struct {
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

// coopCwdResponse is returned by GET /api/v1/session/cwd.
type coopCwdResponse struct {
	Cwd string `json:"cwd"`
}

// coopEnvSetRequest is the body for PUT /api/v1/env/:key.
type coopEnvSetRequest struct {
	Value string `json:"value"`
}

// --- Coop-first Backend method implementations ---

func (b *CoopBackend) KillSession(session string) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(coopSignalRequest{Signal: "SIGTERM"})
	resp, err := b.doRequest("POST", base+"/api/v1/signal", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: signal request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: signal returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (b *CoopBackend) IsAgentRunning(session string) (bool, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return false, err
	}

	resp, err := b.doRequest("GET", base+"/api/v1/status", nil)
	if err != nil {
		return false, fmt.Errorf("coop: status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("coop: status returned %d", resp.StatusCode)
	}

	var status coopStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, fmt.Errorf("coop: parsing status: %w", err)
	}

	return status.State == "running", nil
}

func (b *CoopBackend) GetAgentState(session string) (string, error) {
	state, err := b.AgentState(session)
	if err != nil {
		return "", err
	}
	return state.State, nil
}

func (b *CoopBackend) SetEnvironment(session, key, value string) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(coopEnvSetRequest{Value: value})
	resp, err := b.doRequest("PUT", base+"/api/v1/env/"+key, strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: set env request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: set env returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (b *CoopBackend) GetEnvironment(session, key string) (string, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return "", err
	}

	resp, err := b.doRequest("GET", base+"/api/v1/env/"+key, nil)
	if err != nil {
		return "", fmt.Errorf("coop: get env request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("coop: env var %q not found", key)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("coop: get env returned %d: %s", resp.StatusCode, string(body))
	}

	var envResp coopEnvResponse
	if err := json.NewDecoder(resp.Body).Decode(&envResp); err != nil {
		return "", fmt.Errorf("coop: parsing env response: %w", err)
	}
	if envResp.Value == nil {
		return "", fmt.Errorf("coop: env var %q not found", key)
	}
	return *envResp.Value, nil
}

func (b *CoopBackend) GetPaneWorkDir(session string) (string, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return "", err
	}

	resp, err := b.doRequest("GET", base+"/api/v1/session/cwd", nil)
	if err != nil {
		return "", fmt.Errorf("coop: cwd request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("coop: cwd returned %d: %s", resp.StatusCode, string(body))
	}

	var cwdResp coopCwdResponse
	if err := json.NewDecoder(resp.Body).Decode(&cwdResp); err != nil {
		return "", fmt.Errorf("coop: parsing cwd response: %w", err)
	}
	return cwdResp.Cwd, nil
}

func (b *CoopBackend) SendInput(session string, text string, enter bool) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(coopInputRequest{Text: text, Enter: enter})
	resp, err := b.doRequest("POST", base+"/api/v1/input", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: input request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: input returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (b *CoopBackend) RespawnPane(session string) error {
	// Switch-to-self with no extra env restarts the agent process.
	return b.SwitchSession(session, SwitchConfig{})
}

func (b *CoopBackend) SwitchSession(session string, cfg SwitchConfig) error {
	base, err := b.baseURL(session)
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(coopSwitchRequest{ExtraEnv: cfg.ExtraEnv})
	resp, err := b.doRequest("PUT", base+"/api/v1/session/switch", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("coop: switch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("coop: switch returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
