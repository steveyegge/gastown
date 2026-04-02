package api

import (
	"encoding/json"
	"net/http"
)

// registerMergeRoutes adds issue merge/unmerge API routes.
func (h *Handler) registerMergeRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	member := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("member", h) }

	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/merge", auth(projAccess(member(h.mergeIssue))))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/merge/", auth(projAccess(member(h.mergeIssue))))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/unmerge", auth(projAccess(member(h.unmergeIssue))))
	mux.HandleFunc("POST /api/{project_id}/issues/{issue_id}/unmerge/", auth(projAccess(member(h.unmergeIssue))))
}

func (h *Handler) mergeIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	sourceID := r.PathValue("issue_id")
	if projectID <= 0 || sourceID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	var body struct {
		TargetID string `json:"target_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.TargetID == "" {
		writeErr(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if sourceID == body.TargetID {
		writeErr(w, http.StatusBadRequest, "cannot merge an issue into itself")
		return
	}

	if err := h.DB.MergeIssue(r.Context(), projectID, sourceID, body.TargetID); err != nil {
		h.Log.Error("merge issue", "err", err, "source", sourceID, "target", body.TargetID)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	h.Log.Info("issue merged", "source", sourceID, "target", body.TargetID, "project_id", projectID)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "merged",
		"source_id": sourceID,
		"target_id": body.TargetID,
	})
}

func (h *Handler) unmergeIssue(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	sourceID := r.PathValue("issue_id")
	if projectID <= 0 || sourceID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.UnmergeIssue(r.Context(), projectID, sourceID); err != nil {
		h.Log.Error("unmerge issue", "err", err, "source", sourceID)
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	h.Log.Info("issue unmerged", "source", sourceID, "project_id", projectID)

	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "unmerged",
		"source_id": sourceID,
	})
}
