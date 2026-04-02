package ingest

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ProjectAuth maps public keys to project IDs and optional rig names.
type ProjectAuth struct {
	mu sync.RWMutex
	// keyToProject maps sentry_key (public DSN key) -> project_id.
	keyToProject map[string]int64
	// projectToRig maps project_id -> target rig name for Gas Town integration.
	projectToRig map[int64]string
}

// NewProjectAuth creates an authenticator from "project_id:public_key[:rig]" pairs.
// The rig component is optional — projects without a rig won't trigger bead creation.
func NewProjectAuth(pairs []string) (*ProjectAuth, error) {
	keys := make(map[string]int64, len(pairs))
	rigs := make(map[int64]string, len(pairs))
	for _, p := range pairs {
		parts := strings.SplitN(p, ":", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid project auth pair: %q (want project_id:public_key[:rig])", p)
		}
		var id int64
		if _, err := fmt.Sscanf(parts[0], "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid project id in %q: %w", p, err)
		}
		keys[parts[1]] = id
		if len(parts) == 3 && parts[2] != "" {
			rigs[id] = parts[2]
		}
	}
	return &ProjectAuth{keyToProject: keys, projectToRig: rigs}, nil
}

// Register adds a new project to the auth map at runtime (no restart needed).
func (a *ProjectAuth) Register(publicKey string, projectID int64, rig string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keyToProject[publicKey] = projectID
	if rig != "" {
		a.projectToRig[projectID] = rig
	}
}

// Unregister removes a project from the auth map at runtime.
func (a *ProjectAuth) Unregister(publicKey string, projectID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.keyToProject, publicKey)
	delete(a.projectToRig, projectID)
}

// UnregisterAll removes all projects from the auth map.
func (a *ProjectAuth) UnregisterAll() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.keyToProject = make(map[string]int64)
	a.projectToRig = make(map[int64]string)
}

// RigForProject returns the target Gas Town rig for a project, or "" if none configured.
func (a *ProjectAuth) RigForProject(projectID int64) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.projectToRig[projectID]
}

// Authenticate extracts the sentry_key from the request and returns the project ID.
// It checks X-Sentry-Auth header, Authorization header, and sentry_key query param.
func (a *ProjectAuth) Authenticate(r *http.Request) (int64, error) {
	key := extractKey(r)
	if key == "" {
		return 0, fmt.Errorf("missing sentry authentication")
	}
	a.mu.RLock()
	pid, ok := a.keyToProject[key]
	a.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("invalid sentry key")
	}
	return pid, nil
}

// ProjectRecord is the minimal project info needed to build auth from a database.
type ProjectRecord struct {
	ID        int64
	PublicKey string
	Slug      string // used as rig name
}

// NewProjectAuthFromRecords creates an authenticator from database project records.
func NewProjectAuthFromRecords(records []ProjectRecord) *ProjectAuth {
	keys := make(map[string]int64, len(records))
	rigs := make(map[int64]string, len(records))
	for _, r := range records {
		if r.PublicKey != "" {
			keys[r.PublicKey] = r.ID
		}
		if r.Slug != "" {
			rigs[r.ID] = r.Slug
		}
	}
	return &ProjectAuth{keyToProject: keys, projectToRig: rigs}
}

// extractKey pulls the sentry_key from the request in priority order:
// 1. X-Sentry-Auth header
// 2. Authorization header
// 3. sentry_key query parameter
func extractKey(r *http.Request) string {
	for _, hdr := range []string{"X-Sentry-Auth", "Authorization"} {
		if v := r.Header.Get(hdr); v != "" {
			if key := parseAuthHeader(v); key != "" {
				return key
			}
		}
	}
	return r.URL.Query().Get("sentry_key")
}

// parseAuthHeader parses "Sentry sentry_key=VALUE, ..." format.
func parseAuthHeader(header string) string {
	header = strings.TrimPrefix(header, "Sentry ")
	header = strings.TrimPrefix(header, "sentry ")
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "sentry_key=") {
			return strings.TrimPrefix(part, "sentry_key=")
		}
	}
	return ""
}
