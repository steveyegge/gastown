package api

import (
	"encoding/json"
	"net/http"

	"github.com/outdoorsea/faultline/internal/db"
)

func (h *Handler) registerTeamRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	// Team CRUD.
	mux.HandleFunc("GET /api/v1/teams/", auth(h.listTeams))
	mux.HandleFunc("POST /api/v1/teams/", auth(admin(h.createTeam)))
	mux.HandleFunc("GET /api/v1/teams/{team_id}/", auth(h.getTeam))
	mux.HandleFunc("PUT /api/v1/teams/{team_id}/", auth(admin(h.updateTeam)))
	mux.HandleFunc("DELETE /api/v1/teams/{team_id}/", auth(admin(h.deleteTeam)))

	// Team members.
	mux.HandleFunc("GET /api/v1/teams/{team_id}/members/", auth(h.listTeamMembers))
	mux.HandleFunc("POST /api/v1/teams/{team_id}/members/", auth(admin(h.addTeamMember)))
	mux.HandleFunc("DELETE /api/v1/teams/{team_id}/members/{account_id}/", auth(admin(h.removeTeamMember)))

	// Team projects.
	mux.HandleFunc("GET /api/v1/teams/{team_id}/projects/", auth(h.listTeamProjects))
	mux.HandleFunc("POST /api/v1/teams/{team_id}/projects/", auth(admin(h.linkTeamProject)))
	mux.HandleFunc("DELETE /api/v1/teams/{team_id}/projects/{project_id}/", auth(admin(h.unlinkTeamProject)))

	// Without trailing slash.
	mux.HandleFunc("GET /api/v1/teams", auth(h.listTeams))
	mux.HandleFunc("POST /api/v1/teams", auth(admin(h.createTeam)))
	mux.HandleFunc("GET /api/v1/teams/{team_id}", auth(h.getTeam))
	mux.HandleFunc("PUT /api/v1/teams/{team_id}", auth(admin(h.updateTeam)))
	mux.HandleFunc("DELETE /api/v1/teams/{team_id}", auth(admin(h.deleteTeam)))
	mux.HandleFunc("GET /api/v1/teams/{team_id}/members", auth(h.listTeamMembers))
	mux.HandleFunc("POST /api/v1/teams/{team_id}/members", auth(admin(h.addTeamMember)))
	mux.HandleFunc("DELETE /api/v1/teams/{team_id}/members/{account_id}", auth(admin(h.removeTeamMember)))
	mux.HandleFunc("GET /api/v1/teams/{team_id}/projects", auth(h.listTeamProjects))
	mux.HandleFunc("POST /api/v1/teams/{team_id}/projects", auth(admin(h.linkTeamProject)))
	mux.HandleFunc("DELETE /api/v1/teams/{team_id}/projects/{project_id}", auth(admin(h.unlinkTeamProject)))
}

func (h *Handler) listTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.DB.ListTeams(r.Context())
	if err != nil {
		h.Log.Error("list teams", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if teams == nil {
		teams = []db.Team{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"teams": teams})
}

func (h *Handler) createTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Slug == "" {
		writeErr(w, http.StatusBadRequest, "slug is required")
		return
	}

	team, err := h.DB.CreateTeam(r.Context(), req.Name, req.Slug)
	if err != nil {
		h.Log.Error("create team", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, team)
}

func (h *Handler) getTeam(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	team, err := h.DB.GetTeam(r.Context(), teamID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "team not found")
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (h *Handler) updateTeam(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	// Verify team exists.
	if _, err := h.DB.GetTeam(r.Context(), teamID); err != nil {
		writeErr(w, http.StatusNotFound, "team not found")
		return
	}

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := h.DB.UpdateTeam(r.Context(), teamID, req.Name, req.Slug); err != nil {
		h.Log.Error("update team", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	team, err := h.DB.GetTeam(r.Context(), teamID)
	if err != nil {
		h.Log.Error("get team after update", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, team)
}

func (h *Handler) deleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	if err := h.DB.DeleteTeam(r.Context(), teamID); err != nil {
		h.Log.Error("delete team", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	members, err := h.DB.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		h.Log.Error("list team members", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if members == nil {
		members = []db.TeamMember{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"members": members})
}

func (h *Handler) addTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	var req struct {
		AccountID int64  `json:"account_id"`
		Role      string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.AccountID <= 0 {
		writeErr(w, http.StatusBadRequest, "account_id is required")
		return
	}

	if err := h.DB.AddTeamMember(r.Context(), teamID, req.AccountID, req.Role); err != nil {
		h.Log.Error("add team member", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

func (h *Handler) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	accountID := pathInt64(r, "account_id")
	if teamID <= 0 || accountID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.RemoveTeamMember(r.Context(), teamID, accountID); err != nil {
		h.Log.Error("remove team member", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) listTeamProjects(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	projects, err := h.DB.ListTeamProjects(r.Context(), teamID)
	if err != nil {
		h.Log.Error("list team projects", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if projects == nil {
		projects = []db.TeamProject{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})
}

func (h *Handler) linkTeamProject(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	if teamID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid team_id")
		return
	}

	var req struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.ProjectID <= 0 {
		writeErr(w, http.StatusBadRequest, "project_id is required")
		return
	}

	if err := h.DB.LinkTeamProject(r.Context(), teamID, req.ProjectID); err != nil {
		h.Log.Error("link team project", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "linked"})
}

func (h *Handler) unlinkTeamProject(w http.ResponseWriter, r *http.Request) {
	teamID := pathInt64(r, "team_id")
	projectID := pathInt64(r, "project_id")
	if teamID <= 0 || projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.UnlinkTeamProject(r.Context(), teamID, projectID); err != nil {
		h.Log.Error("unlink team project", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}
