package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	copilotapi "github.com/github/copilot-sdk/go"
)

// Config configures the Copilot SDK runner.
type Config struct {
	CLIPath      string
	CLIURL       string
	LogLevel     string
	Model        string
	WorkDir      string
	SessionFile  string
	LogWriter    io.Writer
	Timeout      time.Duration
	AllowAll     bool
	PollInterval time.Duration
}

// Runner manages Copilot SDK sessions for a single worker.
type Runner struct {
	config  Config
	client  *copilotapi.Client
	logger  *log.Logger
	logFile io.Closer

	sessionMu sync.Mutex
	session   *copilotapi.Session
}

// NewRunner creates a new Runner and starts the Copilot CLI server.
func NewRunner(config Config) (*Runner, error) {
	cfg := config
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work dir is required")
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Minute
	}
	if cfg.SessionFile == "" {
		cfg.SessionFile = filepath.Join(cfg.WorkDir, ".runtime", "copilot-session.json")
	}

	var logWriter io.Writer
	if cfg.LogWriter != nil {
		logWriter = cfg.LogWriter
	} else {
		logDir := filepath.Join(cfg.WorkDir, ".logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("creating log dir: %w", err)
		}
		logPath := filepath.Join(logDir, "copilot-agent.log")
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("opening log file: %w", err)
		}
		logWriter = file
		cfg.LogWriter = file
	}

	logger := log.New(logWriter, "[copilot] ", log.LstdFlags)

	client := copilotapi.NewClient(&copilotapi.ClientOptions{
		CLIPath:  cfg.CLIPath,
		CLIUrl:   cfg.CLIURL,
		Cwd:      cfg.WorkDir,
		LogLevel: cfg.LogLevel,
	})
	if err := client.Start(); err != nil {
		return nil, fmt.Errorf("starting Copilot CLI: %w", err)
	}

	var logCloser io.Closer
	if closer, ok := logWriter.(io.Closer); ok {
		logCloser = closer
	}

	return &Runner{
		config:  cfg,
		client:  client,
		logger:  logger,
		logFile: logCloser,
	}, nil
}

// Close stops the Copilot session and CLI server.
func (r *Runner) Close() {
	if r == nil {
		return
	}
	if r.session != nil {
		_ = r.session.Destroy()
	}
	if r.client != nil {
		_ = r.client.Stop()
	}
	if r.logFile != nil {
		_ = r.logFile.Close()
	}
}

// RunOnce runs a single Copilot agent task with the provided prompts.
func (r *Runner) RunOnce(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	_ = ctx
	session, err := r.getSession(systemPrompt)
	if err != nil {
		return "", err
	}

	resp, err := session.SendAndWait(copilotapi.MessageOptions{Prompt: userPrompt}, r.config.Timeout)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Data.Content == nil {
		return "", nil
	}
	return strings.TrimSpace(*resp.Data.Content), nil
}

func (r *Runner) getSession(systemPrompt string) (*copilotapi.Session, error) {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()

	if r.session != nil {
		return r.session, nil
	}

	state, _ := loadSessionState(r.config.SessionFile)
	if state.SessionID != "" {
		session, err := r.client.ResumeSessionWithOptions(state.SessionID, &copilotapi.ResumeSessionConfig{
			WorkingDirectory: r.config.WorkDir,
			Hooks:            buildHooks(r.config.WorkDir, r.logger, r.config.AllowAll),
		})
		if err == nil {
			r.session = session
			return session, nil
		}
		r.logger.Printf("resume failed, starting new session: %v", err)
	}

	session, err := r.client.CreateSession(&copilotapi.SessionConfig{
		Model:            r.config.Model,
		WorkingDirectory: r.config.WorkDir,
		SystemMessage: &copilotapi.SystemMessageConfig{
			Mode:    "append",
			Content: systemPrompt,
		},
		Hooks: buildHooks(r.config.WorkDir, r.logger, r.config.AllowAll),
	})
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	if err := saveSessionState(r.config.SessionFile, sessionState{SessionID: session.SessionID}); err != nil {
		r.logger.Printf("warning: could not persist session: %v", err)
	}

	r.session = session
	return session, nil
}

type sessionState struct {
	SessionID string `json:"session_id"`
}

func loadSessionState(path string) (sessionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionState{}, err
	}
	var state sessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return sessionState{}, err
	}
	return state, nil
}

func saveSessionState(path string, state sessionState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func buildHooks(workDir string, logger *log.Logger, allowAll bool) *copilotapi.SessionHooks {
	return &copilotapi.SessionHooks{
		OnPreToolUse: func(input copilotapi.PreToolUseHookInput, _ copilotapi.HookInvocation) (*copilotapi.PreToolUseHookOutput, error) {
			if !allowAll {
				return &copilotapi.PreToolUseHookOutput{
					PermissionDecision:       "deny",
					PermissionDecisionReason: "tool use disabled by configuration",
				}, nil
			}
			paths := extractPaths(input.ToolArgs)
			for _, p := range paths {
				resolved, err := normalizePath(workDir, p)
				if err != nil {
					return &copilotapi.PreToolUseHookOutput{
						PermissionDecision:       "deny",
						PermissionDecisionReason: err.Error(),
					}, nil
				}
				if !isWithinBase(resolved, workDir) {
					return &copilotapi.PreToolUseHookOutput{
						PermissionDecision:       "deny",
						PermissionDecisionReason: fmt.Sprintf("path %s is outside %s", resolved, workDir),
					}, nil
				}
			}
			return &copilotapi.PreToolUseHookOutput{PermissionDecision: "allow"}, nil
		},
		OnPostToolUse: func(input copilotapi.PostToolUseHookInput, _ copilotapi.HookInvocation) (*copilotapi.PostToolUseHookOutput, error) {
			if logger != nil {
				logger.Printf("tool=%s args=%v", input.ToolName, input.ToolArgs)
			}
			return nil, nil
		},
	}
}

func extractPaths(args interface{}) []string {
	var paths []string
	switch v := args.(type) {
	case map[string]interface{}:
		for _, key := range []string{"path", "paths", "file", "files", "directory", "root"} {
			if value, ok := v[key]; ok {
				paths = append(paths, toStrings(value)...)
			}
		}
	case []interface{}:
		for _, item := range v {
			paths = append(paths, extractPaths(item)...)
		}
	case string:
		paths = append(paths, v)
	}
	return paths
}

func toStrings(value interface{}) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizePath(base, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Clean(filepath.Join(base, value)), nil
}

func isWithinBase(candidate, base string) bool {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, candAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}
