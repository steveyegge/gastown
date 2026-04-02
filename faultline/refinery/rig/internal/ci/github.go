// Package ci handles CI/CD monitoring and conversion to faultline events.
package ci

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// CIEventHandler is called when a CI workflow run completes.
type CIEventHandler func(ctx context.Context, projectID int64, event CIEvent) error

// CIEvent represents a completed CI workflow run (success or failure).
type CIEvent struct {
	Repo       string    `json:"repo"`
	Branch     string    `json:"branch"`
	Commit     string    `json:"commit"`
	CommitMsg  string    `json:"commit_message"`
	Workflow   string    `json:"workflow"`
	RunID      int64     `json:"run_id"`
	RunURL     string    `json:"run_url"`
	Actor      string    `json:"actor"`
	FailedJobs []string  `json:"failed_jobs"`
	Conclusion string    `json:"conclusion"`
	Timestamp  time.Time `json:"timestamp"`
}

// ProjectLookup maps a GitHub repo (owner/name) to a faultline project ID.
type ProjectLookup func(repo string) int64

// Handler handles GitHub webhook events for CI monitoring.
type Handler struct {
	Secret        string         // webhook secret for signature verification
	OnFailure     CIEventHandler // called when CI fails
	OnSuccess     CIEventHandler // called when CI succeeds (stored in ci_runs for verification)
	LookupProject ProjectLookup  // maps repo to project ID
	Log           *slog.Logger
}

// RegisterRoutes adds CI webhook routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/hooks/ci/github", h.HandleGitHub)
	mux.HandleFunc("POST /api/hooks/ci/github/", h.HandleGitHub)
}

// HandleGitHub processes a GitHub webhook request.
func (h *Handler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify webhook signature if secret is configured.
	if h.Secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, sig, h.Secret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	event := r.Header.Get("X-GitHub-Event")
	if event != "workflow_run" {
		// Accept but ignore non-workflow events.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored"}`))
		return
	}

	var payload struct {
		Action      string `json:"action"`
		WorkflowRun struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
			HeadBranch string `json:"head_branch"`
			HeadSHA    string `json:"head_sha"`
			Actor      struct {
				Login string `json:"login"`
			} `json:"actor"`
			HeadCommit struct {
				Message string `json:"message"`
			} `json:"head_commit"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
			UpdatedAt string `json:"updated_at"`
		} `json:"workflow_run"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	run := payload.WorkflowRun

	// Only process completed runs (skip in-progress, cancelled).
	if payload.Action != "completed" || run.Conclusion == "cancelled" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","action":"skipped"}`))
		return
	}

	// Look up the faultline project for this repo.
	projectID := h.LookupProject(run.Repository.FullName)
	if projectID == 0 {
		h.Log.Warn("ci webhook: unknown repo", "repo", run.Repository.FullName)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","action":"unknown_repo"}`))
		return
	}

	ts, _ := time.Parse(time.RFC3339, run.UpdatedAt)
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	evt := CIEvent{
		Repo:       run.Repository.FullName,
		Branch:     run.HeadBranch,
		Commit:     run.HeadSHA,
		CommitMsg:  firstLine(run.HeadCommit.Message),
		Workflow:   run.Name,
		RunID:      run.ID,
		RunURL:     run.HTMLURL,
		Actor:      run.Actor.Login,
		Conclusion: run.Conclusion,
		Timestamp:  ts,
	}

	if run.Conclusion == "success" {
		// Track CI success for fix verification.
		if h.OnSuccess != nil {
			if err := h.OnSuccess(r.Context(), projectID, evt); err != nil {
				h.Log.Error("ci success handler", "err", err)
			}
		}
		h.Log.Info("ci success recorded",
			"repo", evt.Repo,
			"workflow", evt.Workflow,
			"branch", evt.Branch,
			"commit", evt.Commit[:minLen(len(evt.Commit), 8)],
		)
	} else {
		// CI failure — process as error event.
		h.Log.Info("ci failure detected",
			"repo", evt.Repo,
			"workflow", evt.Workflow,
			"branch", evt.Branch,
			"conclusion", evt.Conclusion,
		)
		if err := h.OnFailure(r.Context(), projectID, evt); err != nil {
			h.Log.Error("ci failure handler", "err", err)
			http.Error(w, "processing error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok","action":"processed"}`))
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func verifySignature(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// ConvertToSentryEvent converts a CI failure to a Sentry-compatible event payload
// that can be fed through the normal faultline ingest pipeline.
func ConvertToSentryEvent(evt CIEvent) json.RawMessage {
	// Extract short repo name (e.g. "faultline" from "outdoorsea/faultline").
	repoShort := evt.Repo
	if idx := strings.LastIndex(repoShort, "/"); idx >= 0 {
		repoShort = repoShort[idx+1:]
	}

	title := fmt.Sprintf("CI: %s/%s failed on %s (%s)", repoShort, evt.Workflow, evt.Branch, evt.Conclusion)

	// Use repo+branch+workflow as the fingerprint so different repos and
	// workflows get separate issue groups, but repeated failures on the
	// same repo/branch/workflow group together.
	fingerprint := fmt.Sprintf("ci:%s:%s:%s", evt.Repo, evt.Branch, evt.Workflow)

	event := map[string]interface{}{
		"event_id":    fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d-%s", evt.RunID, evt.Repo))))[:32],
		"timestamp":   float64(evt.Timestamp.Unix()),
		"platform":    "other",
		"level":       "error",
		"logger":      "ci.github",
		"message":     title,
		"fingerprint": []string{fingerprint},
		"culprit":     fmt.Sprintf("%s@%s", evt.Repo, evt.Branch),
		"tags": map[string]string{
			"source":   "ci",
			"ci":       "github-actions",
			"repo":     evt.Repo,
			"branch":   evt.Branch,
			"workflow": evt.Workflow,
			"actor":    evt.Actor,
		},
		"extra": map[string]interface{}{
			"run_id":     evt.RunID,
			"run_url":    evt.RunURL,
			"commit":     evt.Commit,
			"commit_msg": evt.CommitMsg,
			"conclusion": evt.Conclusion,
		},
		"exception": map[string]interface{}{
			"values": []map[string]interface{}{
				{
					"type":  "CIFailure",
					"value": title,
				},
			},
		},
	}

	data, _ := json.Marshal(event)
	return data
}
