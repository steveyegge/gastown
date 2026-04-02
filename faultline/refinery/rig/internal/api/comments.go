package api

import (
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
)

// listComments handles GET /api/{project_id}/issues/{issue_id}/comments/.
func (h *Handler) listComments(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n := parseInt(v); n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n := parseInt(v); n >= 0 {
			offset = n
		}
	}

	comments, err := h.DB.ListComments(r.Context(), projectID, issueID, limit, offset)
	if err != nil {
		h.Log.Error("list comments", "project_id", projectID, "issue_id", issueID, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if comments == nil {
		comments = []db.Comment{}
	}
	writeJSON(w, http.StatusOK, comments)
}

// createComment handles POST /api/{project_id}/issues/{issue_id}/comments/.
// It creates a comment, generates in-app notifications for @mentions, and sends
// Slack DMs to mentioned users who have linked Slack accounts.
func (h *Handler) createComment(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	var body struct {
		Body       string `json:"body"`
		AuthorID   int64  `json:"author_id"`
		AuthorName string `json:"author_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body is required")
		return
	}
	if body.AuthorID == 0 {
		writeErr(w, http.StatusBadRequest, "author_id is required")
		return
	}
	if body.AuthorName == "" {
		body.AuthorName = "api"
	}

	ctx := r.Context()
	comment, err := h.DB.CreateComment(ctx, projectID, issueID, body.AuthorID, body.Body)
	if err != nil {
		h.Log.Error("create comment", "project_id", projectID, "issue_id", issueID, "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to create comment")
		return
	}

	// Send Slack DMs and in-app notifications for @mentions.
	if h.SlackDMs != nil {
		h.SlackDMs.NotifyMentions(ctx, projectID, issueID, comment, body.AuthorID, body.AuthorName)
	}

	writeJSON(w, http.StatusCreated, comment)
}

func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}
