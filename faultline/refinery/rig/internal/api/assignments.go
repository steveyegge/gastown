package api

import (
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
)

func (h *Handler) registerAssignmentRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess

	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/assign/", auth(projAccess(h.assignIssue)))
	mux.HandleFunc("DELETE /api/{project_id}/issues/{issue_id}/assign/", auth(projAccess(h.unassignIssue)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/assign/", auth(projAccess(h.getIssueAssignment)))

	// Without trailing slash.
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/assign", auth(projAccess(h.assignIssue)))
	mux.HandleFunc("DELETE /api/{project_id}/issues/{issue_id}/assign", auth(projAccess(h.unassignIssue)))
	mux.HandleFunc("GET /api/{project_id}/issues/{issue_id}/assign", auth(projAccess(h.getIssueAssignment)))
}

func (h *Handler) assignIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	var body struct {
		AssignedTo string `json:"assigned_to"`
		AssignedBy string `json:"assigned_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.AssignedTo == "" {
		writeErr(w, http.StatusBadRequest, "assigned_to is required")
		return
	}
	if body.AssignedBy == "" {
		body.AssignedBy = "api"
	}

	ctx := r.Context()
	if err := h.DB.AssignIssue(ctx, issueID, projectID, body.AssignedTo, body.AssignedBy); err != nil {
		h.Log.Error("assign issue", "project_id", projectID, "issue_id", issueID, "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to assign issue")
		return
	}

	// Record lifecycle event.
	_ = h.DB.InsertLifecycleEvent(ctx, projectID, issueID, db.LifecycleAssigned, nil, nil, map[string]interface{}{
		"trigger":  "api",
		"assignee": body.AssignedTo,
		"actor":    body.AssignedBy,
	})

	// Send Slack DM notification.
	if h.SlackDMs != nil {
		h.SlackDMs.NotifyAssignment(ctx, projectID, issueID, body.AssignedTo, body.AssignedBy)
	}

	assignment, err := h.DB.GetIssueAssignment(ctx, issueID, projectID)
	if err != nil {
		h.Log.Error("get assignment after assign", "err", err)
		writeErr(w, http.StatusInternalServerError, "assigned but failed to read back")
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}

func (h *Handler) unassignIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	ctx := r.Context()
	if err := h.DB.UnassignIssue(ctx, issueID, projectID); err != nil {
		h.Log.Error("unassign issue", "project_id", projectID, "issue_id", issueID, "err", err)
		writeErr(w, http.StatusInternalServerError, "failed to unassign issue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}

func (h *Handler) getIssueAssignment(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")

	assignment, err := h.DB.GetIssueAssignment(r.Context(), issueID, projectID)
	if err != nil {
		h.Log.Error("get issue assignment", "project_id", projectID, "issue_id", issueID, "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if assignment == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"assigned": false})
		return
	}
	writeJSON(w, http.StatusOK, assignment)
}
