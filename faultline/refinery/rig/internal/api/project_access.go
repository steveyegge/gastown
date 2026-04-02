package api

import (
	"net/http"
	"slices"
	"strconv"
)

// requireProjectAccess returns middleware that verifies the authenticated user
// has access to the project identified by {project_id} in the URL path.
// Owners and admins bypass the team check. Members and viewers must belong to a
// team that is linked to the project via team_projects.
// Project-scoped API tokens (X-Token-Project-ID) are restricted to their
// assigned project regardless of account role.
func (h *Handler) requireProjectAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID := pathInt64(r, "project_id")

		// Enforce token project scope: if the token is scoped to a specific
		// project, deny access to any other project.
		if tokenProjStr := r.Header.Get("X-Token-Project-ID"); tokenProjStr != "" {
			tokenProjID, _ := strconv.ParseInt(tokenProjStr, 10, 64)
			if projectID > 0 && tokenProjID != projectID {
				writeErr(w, http.StatusForbidden, "token is scoped to a different project")
				return
			}
		}

		accountRole := r.Header.Get("X-Account-Role")

		// Owners and admins see all projects.
		if accountRole == "owner" || accountRole == "admin" {
			next(w, r)
			return
		}

		if projectID <= 0 {
			next(w, r) // let the handler itself return the validation error
			return
		}

		accountID, _ := strconv.ParseInt(r.Header.Get("X-Account-ID"), 10, 64)
		if accountID <= 0 {
			writeErr(w, http.StatusForbidden, "project access denied")
			return
		}

		allowed, err := h.DB.ProjectIDsVisibleTo(r.Context(), accountID, accountRole)
		if err != nil {
			h.Log.Error("project access check", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}

		// nil means all visible (owner/admin), but we already handled that above.
		// An empty slice means no projects are visible.
		if !slices.Contains(allowed, projectID) {
			writeErr(w, http.StatusForbidden, "project access denied")
			return
		}

		next(w, r)
	}
}
