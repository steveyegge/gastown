package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/outdoorsea/faultline/internal/crypto"
	"github.com/outdoorsea/faultline/internal/db"
)

func (h *Handler) registerDatabaseRoutes(mux *http.ServeMux) {
	auth := h.requireBearer
	projAccess := h.requireProjectAccess
	member := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("member", h) }
	admin := func(h http.HandlerFunc) http.HandlerFunc { return requireRole("admin", h) }

	// List / Create monitored databases.
	mux.HandleFunc("GET /api/{project_id}/databases/", auth(projAccess(member(h.listDatabases))))
	mux.HandleFunc("POST /api/{project_id}/databases/", auth(projAccess(admin(h.createDatabase))))

	// Get / Update / Delete a single monitored database.
	mux.HandleFunc("GET /api/{project_id}/databases/{database_id}/", auth(projAccess(member(h.getDatabase))))
	mux.HandleFunc("PUT /api/{project_id}/databases/{database_id}/", auth(projAccess(admin(h.updateDatabase))))
	mux.HandleFunc("DELETE /api/{project_id}/databases/{database_id}/", auth(projAccess(admin(h.deleteDatabase))))

	// Test connection.
	mux.HandleFunc("POST /api/{project_id}/databases/{database_id}/test/", auth(projAccess(admin(h.testDatabaseConnection))))

	// Check history.
	mux.HandleFunc("GET /api/{project_id}/databases/{database_id}/checks/", auth(projAccess(member(h.listDatabaseChecks))))

	// Without trailing slash.
	mux.HandleFunc("GET /api/{project_id}/databases", auth(projAccess(member(h.listDatabases))))
	mux.HandleFunc("POST /api/{project_id}/databases", auth(projAccess(admin(h.createDatabase))))
	mux.HandleFunc("GET /api/{project_id}/databases/{database_id}", auth(projAccess(member(h.getDatabase))))
	mux.HandleFunc("PUT /api/{project_id}/databases/{database_id}", auth(projAccess(admin(h.updateDatabase))))
	mux.HandleFunc("DELETE /api/{project_id}/databases/{database_id}", auth(projAccess(admin(h.deleteDatabase))))
	mux.HandleFunc("POST /api/{project_id}/databases/{database_id}/test", auth(projAccess(admin(h.testDatabaseConnection))))
	mux.HandleFunc("GET /api/{project_id}/databases/{database_id}/checks", auth(projAccess(member(h.listDatabaseChecks))))
}

// databaseResponse is the JSON shape returned for a monitored database.
// Connection strings are always masked in responses.
type databaseResponse struct {
	ID               string `json:"id"`
	ProjectID        int64  `json:"project_id"`
	Name             string `json:"name"`
	DBType           string `json:"db_type"`
	ConnectionString string `json:"connection_string"` // masked
	Enabled          bool   `json:"enabled"`
	CheckIntervalSec int    `json:"check_interval_sec"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func maskDatabase(m *db.MonitoredDatabase) databaseResponse {
	connStr := crypto.MaskConnectionString(string(m.ConnectionString))
	return databaseResponse{
		ID:               m.ID,
		ProjectID:        m.ProjectID,
		Name:             m.Name,
		DBType:           m.DBType,
		ConnectionString: connStr,
		Enabled:          m.Enabled,
		CheckIntervalSec: m.CheckIntervalSec,
		CreatedAt:        m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *Handler) listDatabases(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	dbs, err := h.DB.ListMonitoredDatabasesByProject(r.Context(), projectID)
	if err != nil {
		h.Log.Error("list databases", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]databaseResponse, len(dbs))
	for i := range dbs {
		resp[i] = maskDatabase(&dbs[i])
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"databases": resp})
}

func (h *Handler) getDatabase(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	databaseID := r.PathValue("database_id")
	if projectID <= 0 || databaseID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	m, err := h.DB.GetMonitoredDatabase(r.Context(), projectID, databaseID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "database not found")
		return
	}

	writeJSON(w, http.StatusOK, maskDatabase(m))
}

type createDatabaseRequest struct {
	Name             string `json:"name"`
	DBType           string `json:"db_type"`
	ConnectionString string `json:"connection_string"`
	Enabled          *bool  `json:"enabled"`
	CheckIntervalSec int    `json:"check_interval_sec"`
}

func (h *Handler) createDatabase(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	if projectID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}

	var req createDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.DBType == "" {
		writeErr(w, http.StatusBadRequest, "db_type is required")
		return
	}
	if req.ConnectionString == "" {
		writeErr(w, http.StatusBadRequest, "connection_string is required")
		return
	}

	// Encrypt the connection string before storing.
	connBytes, err := h.encryptConnectionString(req.ConnectionString)
	if err != nil {
		h.Log.Error("encrypt connection string", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	m := &db.MonitoredDatabase{
		ProjectID:        projectID,
		Name:             req.Name,
		DBType:           req.DBType,
		ConnectionString: connBytes,
		Enabled:          enabled,
		CheckIntervalSec: req.CheckIntervalSec,
	}

	if err := h.DB.InsertMonitoredDatabase(r.Context(), m); err != nil {
		h.Log.Error("create database", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, maskDatabase(m))
}

func (h *Handler) updateDatabase(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	databaseID := r.PathValue("database_id")
	if projectID <= 0 || databaseID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	existing, err := h.DB.GetMonitoredDatabase(r.Context(), projectID, databaseID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "database not found")
		return
	}

	var req createDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Apply partial updates — keep existing values for unset fields.
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.DBType != "" {
		existing.DBType = req.DBType
	}
	if req.ConnectionString != "" {
		connBytes, err := h.encryptConnectionString(req.ConnectionString)
		if err != nil {
			h.Log.Error("encrypt connection string", "err", err)
			writeErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		existing.ConnectionString = connBytes
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.CheckIntervalSec > 0 {
		existing.CheckIntervalSec = req.CheckIntervalSec
	}

	if err := h.DB.UpdateMonitoredDatabase(r.Context(), existing); err != nil {
		h.Log.Error("update database", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, maskDatabase(existing))
}

func (h *Handler) deleteDatabase(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	databaseID := r.PathValue("database_id")
	if projectID <= 0 || databaseID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	if err := h.DB.DeleteMonitoredDatabase(r.Context(), projectID, databaseID); err != nil {
		h.Log.Error("delete database", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) testDatabaseConnection(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	databaseID := r.PathValue("database_id")
	if projectID <= 0 || databaseID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	m, err := h.DB.GetMonitoredDatabase(r.Context(), projectID, databaseID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "database not found")
		return
	}

	// Decrypt the connection string for testing.
	connStr, err := h.decryptConnectionString(m.ConnectionString)
	if err != nil {
		h.Log.Error("decrypt connection string for test", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Attempt a basic connection test using database/sql.
	testDB, err := openTestConnection(m.DBType, connStr)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer func() { _ = testDB.Close() }()

	if err := testDB.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (h *Handler) listDatabaseChecks(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	databaseID := r.PathValue("database_id")
	if projectID <= 0 || databaseID == "" {
		writeErr(w, http.StatusBadRequest, "invalid parameters")
		return
	}

	// Verify the database belongs to the project.
	if _, err := h.DB.GetMonitoredDatabase(r.Context(), projectID, databaseID); err != nil {
		writeErr(w, http.StatusNotFound, "database not found")
		return
	}

	limit := queryInt(r, "limit", 100)
	checks, err := h.DB.ListDBChecksByDatabase(r.Context(), databaseID, limit)
	if err != nil {
		h.Log.Error("list database checks", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if checks == nil {
		checks = []db.DBCheck{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"checks": checks})
}

// encryptConnectionString encrypts a plaintext connection string.
// If no encryption key is configured, stores as plaintext bytes.
func (h *Handler) encryptConnectionString(plaintext string) ([]byte, error) {
	if len(h.EncryptionKey) == 0 {
		return []byte(plaintext), nil
	}
	encrypted, err := crypto.Encrypt(plaintext, h.EncryptionKey)
	if err != nil {
		return nil, err
	}
	return []byte(encrypted), nil
}

// decryptConnectionString decrypts an encrypted connection string.
// If no encryption key is configured, returns the raw bytes as a string.
func (h *Handler) decryptConnectionString(ciphertext []byte) (string, error) {
	if len(h.EncryptionKey) == 0 {
		return string(ciphertext), nil
	}
	return crypto.Decrypt(string(ciphertext), h.EncryptionKey)
}

// openTestConnection opens a database connection for testing connectivity.
func openTestConnection(engine, connStr string) (*sql.DB, error) {
	switch engine {
	case "mysql", "mariadb", "dolt":
		return sql.Open("mysql", connStr)
	default:
		return nil, fmt.Errorf("unsupported engine for test: %s", engine)
	}
}
