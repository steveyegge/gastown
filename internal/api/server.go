// ABOUTME: HTTP API server for Gas Town.
// ABOUTME: Exposes Gas Town functionality via REST API for web integrations.

// @title Gas Town API
// @version 1.0
// @description REST API for Gas Town - Multi-agent orchestration system for Claude Code
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url https://github.com/steveyegge/gastown

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /api
// @schemes http https

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/web"
)

// ============================================================================
// Common Response Types
// ============================================================================

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error" example:"resource not found"`
}

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Status string `json:"status" example:"ok"`
	Output string `json:"output,omitempty" example:"Operation completed successfully"`
}

// ============================================================================
// Health Types
// ============================================================================

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status" example:"ok"`
	TownRoot  string `json:"town_root" example:"/home/user/gt"`
	Timestamp string `json:"timestamp" example:"2024-01-15T10:30:00Z"`
}

// ============================================================================
// Rig Types
// ============================================================================

// RigInfo represents information about a rig (project)
type RigInfo struct {
	Name      string `json:"name" example:"myproject"`
	Path      string `json:"path" example:"/home/user/gt/myproject"`
	HasBeads  bool   `json:"has_beads" example:"true"`
	HasConfig bool   `json:"has_config" example:"true"`
}

// RigListResponse represents the response for listing rigs
type RigListResponse struct {
	Rigs  []RigInfo `json:"rigs"`
	Count int       `json:"count" example:"3"`
}

// RigDetailResponse represents detailed rig information
type RigDetailResponse struct {
	Name         string                 `json:"name" example:"myproject"`
	Path         string                 `json:"path" example:"/home/user/gt/myproject"`
	Config       map[string]interface{} `json:"config"`
	PolecatCount int                    `json:"polecat_count" example:"5"`
	CrewCount    int                    `json:"crew_count" example:"2"`
}

// CreateRigRequest represents a request to create a new rig
type CreateRigRequest struct {
	// Name of the rig (required)
	Name string `json:"name" example:"my-project" binding:"required"`
	// Git URL to clone (required)
	GitURL string `json:"git_url" example:"https://github.com/user/repo.git" binding:"required"`
	// Optional branch name (defaults to auto-detected from remote)
	Branch string `json:"branch,omitempty" example:"main"`
	// Optional beads issue prefix (defaults to derived from name)
	Prefix string `json:"prefix,omitempty" example:"mp"`
	// Optional description for the rig
	Description string `json:"description,omitempty" example:"My awesome project"`
}

// CreateRigResponse represents the response from creating a rig
type CreateRigResponse struct {
	Status string  `json:"status" example:"created"`
	Rig    RigInfo `json:"rig"`
	Output string  `json:"output,omitempty"`
}

// ============================================================================
// Job (Bead) Types
// ============================================================================

// CreateJobRequest represents a request to create a new job/bead
type CreateJobRequest struct {
	// Title of the job (required)
	Title string `json:"title" example:"Implement user authentication" binding:"required"`
	// Detailed description of what needs to be done
	Description string `json:"description" example:"Add JWT-based auth to the API endpoints"`
	// Type of job: task, bug, feature, etc.
	Type string `json:"type" example:"task"`
	// Priority level (1=highest, 5=lowest)
	Priority int `json:"priority" example:"2"`
}

// UpdateJobRequest represents a request to update an existing job
type UpdateJobRequest struct {
	// New status: open, in_progress, closed
	Status string `json:"status,omitempty" example:"in_progress"`
	// Additional notes to append
	Notes string `json:"notes,omitempty" example:"Started working on this"`
	// Updated description
	Description string `json:"description,omitempty"`
	// Assign to an agent
	Assignee string `json:"assignee,omitempty" example:"myproject/polecats/jasper"`
}

// CloseJobRequest represents a request to close a job
type CloseJobRequest struct {
	// Reason for closing
	Reason string `json:"reason,omitempty" example:"Completed successfully"`
}

// JobResponse represents a job in responses
type JobResponse struct {
	ID          string `json:"id" example:"mp-12345"`
	Title       string `json:"title" example:"Fix login bug"`
	Description string `json:"description"`
	Type        string `json:"type" example:"bug"`
	Status      string `json:"status" example:"open"`
	Priority    int    `json:"priority" example:"1"`
	Assignee    string `json:"assignee,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// JobListResponse represents the response for listing jobs
type JobListResponse struct {
	Jobs  []interface{} `json:"jobs"`
	Count int           `json:"count" example:"10"`
}

// ============================================================================
// Sling Types
// ============================================================================

// SlingRequest represents a request to dispatch work to polecats
type SlingRequest struct {
	// Bead ID to work on (either this or formula required)
	BeadID string `json:"bead_id,omitempty" example:"mp-12345"`
	// Formula to execute (either this or bead_id required)
	Formula string `json:"formula,omitempty" example:"mol-polecat-work"`
	// Additional arguments for the formula
	Args string `json:"args,omitempty" example:"--verbose"`
	// Skip convoy creation
	NoConvoy bool `json:"no_convoy,omitempty" example:"false"`
}

// SlingResponse represents the response from a sling operation
type SlingResponse struct {
	Status string `json:"status" example:"slung"`
	BeadID string `json:"bead_id" example:"mp-12345"`
	Rig    string `json:"rig" example:"myproject"`
	Output string `json:"output"`
}

// ============================================================================
// Merge Queue Types
// ============================================================================

// MQSubmitRequest represents a request to submit to the merge queue
type MQSubmitRequest struct {
	// Branch to submit (optional, uses current branch if empty)
	Branch string `json:"branch,omitempty" example:"feature/new-api"`
	// Associated issue ID
	Issue string `json:"issue,omitempty" example:"mp-12345"`
	// Priority in queue (higher = processed first)
	Priority int `json:"priority,omitempty" example:"1"`
}

// MQSubmitResponse represents the response from MQ submit
type MQSubmitResponse struct {
	Status string `json:"status" example:"submitted"`
	Output string `json:"output"`
}

// ============================================================================
// Polecat Types
// ============================================================================

// PolecatInfo represents information about a polecat worker
type PolecatInfo struct {
	Name           string `json:"name" example:"jasper"`
	Rig            string `json:"rig" example:"myproject"`
	State          string `json:"state" example:"working"`
	SessionRunning bool   `json:"session_running" example:"true"`
}

// PolecatListResponse represents the response for listing polecats
type PolecatListResponse struct {
	Polecats interface{} `json:"polecats"`
}

// PolecatStatusResponse represents a polecat's status
type PolecatStatusResponse struct {
	Name   string `json:"name" example:"jasper"`
	Rig    string `json:"rig" example:"myproject"`
	Status string `json:"status"`
}

// PolecatNukeResponse represents the response from nuking a polecat
type PolecatNukeResponse struct {
	Status string `json:"status" example:"nuked"`
	Name   string `json:"name" example:"jasper"`
	Rig    string `json:"rig" example:"myproject"`
	Output string `json:"output"`
}

// ============================================================================
// Refinery Types
// ============================================================================

// RefineryStatusResponse represents refinery status
type RefineryStatusResponse struct {
	Rig    string `json:"rig" example:"myproject"`
	Status string `json:"status"`
}

// RefineryActionResponse represents response from refinery start/stop
type RefineryActionResponse struct {
	Status string `json:"status" example:"started"`
	Rig    string `json:"rig" example:"myproject"`
	Output string `json:"output"`
}

// ============================================================================
// Mayor Types
// ============================================================================

// MayorStatusResponse represents mayor status
type MayorStatusResponse struct {
	Status string `json:"status"`
}

// MayorActionResponse represents response from mayor start/stop
type MayorActionResponse struct {
	Status string `json:"status" example:"started"`
	Output string `json:"output"`
}

// ============================================================================
// Witness Types
// ============================================================================

// WitnessStatusResponse represents witness status
type WitnessStatusResponse struct {
	Rig    string `json:"rig" example:"myproject"`
	Status string `json:"status"`
}

// WitnessActionResponse represents response from witness start/stop
type WitnessActionResponse struct {
	Status string `json:"status" example:"started"`
	Rig    string `json:"rig" example:"myproject"`
	Output string `json:"output"`
}

// ============================================================================
// Convoy Types
// ============================================================================

// ConvoyInfo represents information about a convoy
type ConvoyInfo struct {
	ID        string `json:"id" example:"hq-cv-12345"`
	Title     string `json:"title" example:"Feature batch"`
	Status    string `json:"status" example:"open"`
	CreatedAt string `json:"created_at"`
}

// ConvoyListResponse represents the response for listing convoys
type ConvoyListResponse struct {
	Convoys interface{} `json:"convoys"`
}

// CreateConvoyRequest represents a request to create a convoy
type CreateConvoyRequest struct {
	// Title of the convoy
	Title string `json:"title" example:"Feature batch" binding:"required"`
	// Bead IDs to track in this convoy
	Tracks []string `json:"tracks" example:"mp-123,mp-456" binding:"required"`
}

// ConvoyStatusResponse represents convoy status
type ConvoyStatusResponse struct {
	ConvoyID string `json:"convoy_id" example:"hq-cv-12345"`
	Status   string `json:"status"`
}

// ============================================================================
// Mail Types
// ============================================================================

// SendMailRequest represents a request to send mail to an agent
type SendMailRequest struct {
	// Recipient agent address (e.g., "myproject/witness" or "mayor")
	To string `json:"to" example:"myproject/witness" binding:"required"`
	// Subject line
	Subject string `json:"subject" example:"Help needed" binding:"required"`
	// Message body
	Body string `json:"body,omitempty" example:"I'm stuck on the authentication issue"`
}

// SendMailResponse represents the response from sending mail
type SendMailResponse struct {
	Status string `json:"status" example:"sent"`
	To     string `json:"to" example:"myproject/witness"`
	Output string `json:"output"`
}

// InboxResponse represents an agent's inbox
type InboxResponse struct {
	Agent string `json:"agent" example:"myproject/witness"`
	Inbox string `json:"inbox"`
}

// ============================================================================
// Server
// ============================================================================

// Server is the HTTP API server for Gas Town.
type Server struct {
	townRoot         string
	port             int
	server           *http.Server
	gtBinary         string // Path to gt binary
	includeDashboard bool   // Whether to serve the HTML dashboard at /
}

// ServerOption is a functional option for configuring the server.
type ServerOption func(*Server)

// WithDashboard enables the HTML dashboard at /.
func WithDashboard() ServerOption {
	return func(s *Server) {
		s.includeDashboard = true
	}
}

// NewServer creates a new API server.
func NewServer(townRoot string, port int, opts ...ServerOption) *Server {
	// Find gt binary - use absolute path for reliability
	gtBinary := "gt" // Default to PATH

	// First check current directory
	if cwd, err := os.Getwd(); err == nil {
		localBin := filepath.Join(cwd, "gt")
		if _, err := os.Stat(localBin); err == nil {
			gtBinary = localBin
		}
	}

	// Check if we have the binary from os.Executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if exeDir != "" && strings.HasSuffix(exe, "gt") {
			gtBinary = exe
		}
	}

	s := &Server{
		townRoot: townRoot,
		port:     port,
		gtBinary: gtBinary,
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// runGT executes a gt command and returns stdout, stderr, and error.
func (s *Server) runGT(args ...string) (string, string, error) {
	cmd := exec.Command(s.gtBinary, args...)
	cmd.Dir = s.townRoot
	cmd.Env = append(os.Environ(), "GT_ROOT="+s.townRoot)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// runBD executes a bd command and returns stdout, stderr, and error.
func (s *Server) runBD(rigName string, args ...string) (string, string, error) {
	cmd := exec.Command("bd", args...)
	cmd.Dir = filepath.Join(s.townRoot, rigName)
	cmd.Env = append(os.Environ(), "GT_ROOT="+s.townRoot)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", s.handleHealth)

	// Swagger docs
	mux.HandleFunc("GET /swagger/", s.handleSwagger)
	mux.HandleFunc("GET /api/docs", s.handleAPIDocs)

	// Rigs
	mux.HandleFunc("GET /api/rigs", s.handleListRigs)
	mux.HandleFunc("POST /api/rigs", s.handleCreateRig)
	mux.HandleFunc("GET /api/rigs/{rig}", s.handleGetRig)

	// Jobs (Beads)
	mux.HandleFunc("POST /api/rigs/{rig}/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /api/rigs/{rig}/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/rigs/{rig}/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("PUT /api/rigs/{rig}/jobs/{id}", s.handleUpdateJob)
	mux.HandleFunc("POST /api/rigs/{rig}/jobs/{id}/close", s.handleCloseJob)

	// Sling (dispatch work)
	mux.HandleFunc("POST /api/rigs/{rig}/sling", s.handleSling)

	// Merge Queue
	mux.HandleFunc("GET /api/rigs/{rig}/mq", s.handleListMergeQueue)
	mux.HandleFunc("POST /api/rigs/{rig}/mq/submit", s.handleMQSubmit)

	// Polecats
	mux.HandleFunc("GET /api/rigs/{rig}/polecats", s.handleListPolecats)
	mux.HandleFunc("GET /api/rigs/{rig}/polecats/{name}", s.handleGetPolecat)
	mux.HandleFunc("POST /api/rigs/{rig}/polecats/{name}/nuke", s.handleNukePolecat)

	// Refinery
	mux.HandleFunc("GET /api/rigs/{rig}/refinery", s.handleRefineryStatus)
	mux.HandleFunc("POST /api/rigs/{rig}/refinery/start", s.handleRefineryStart)
	mux.HandleFunc("POST /api/rigs/{rig}/refinery/stop", s.handleRefineryStop)

	// Mayor
	mux.HandleFunc("GET /api/mayor", s.handleMayorStatus)
	mux.HandleFunc("POST /api/mayor/start", s.handleMayorStart)
	mux.HandleFunc("POST /api/mayor/stop", s.handleMayorStop)

	// Witness
	mux.HandleFunc("GET /api/rigs/{rig}/witness", s.handleWitnessStatus)
	mux.HandleFunc("POST /api/rigs/{rig}/witness/start", s.handleWitnessStart)
	mux.HandleFunc("POST /api/rigs/{rig}/witness/stop", s.handleWitnessStop)

	// Convoy
	mux.HandleFunc("GET /api/convoys", s.handleListConvoys)
	mux.HandleFunc("POST /api/convoys", s.handleCreateConvoy)
	mux.HandleFunc("GET /api/convoys/{id}", s.handleGetConvoy)

	// Mail
	mux.HandleFunc("POST /api/mail/send", s.handleSendMail)
	mux.HandleFunc("GET /api/rigs/{rig}/agents/{agent}/inbox", s.handleGetInbox)

	// Dashboard (HTML UI) - mount if enabled
	if s.includeDashboard {
		fetcher, err := web.NewLiveConvoyFetcherWithRoot(s.townRoot)
		if err != nil {
			return fmt.Errorf("creating convoy fetcher: %w", err)
		}

		dashboardHandler, err := web.NewConvoyHandler(fetcher)
		if err != nil {
			return fmt.Errorf("creating dashboard handler: %w", err)
		}

		// Serve static files and dashboard
		// Note: Register "/" last and without method prefix to avoid conflicts
		staticHandler := web.StaticHandler()
		mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler))
		mux.HandleFunc("GET /{$}", dashboardHandler.ServeHTTP)
	}

	// CORS middleware
	handler := corsMiddleware(mux)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Longer for sling operations
	}

	fmt.Printf("🚀 Gas Town API server starting on port %d\n", s.port)
	fmt.Printf("   Town root: %s\n", s.townRoot)
	fmt.Printf("   GT binary: %s\n", s.gtBinary)
	if s.includeDashboard {
		fmt.Printf("\n   Dashboard:\n")
		fmt.Printf("     GET  /                   - HTML Dashboard\n")
		fmt.Printf("     GET  /static/            - Static assets\n")
	}
	fmt.Printf("\n   Documentation:\n")
	fmt.Printf("     GET  /api/docs           - API documentation (JSON)\n")
	fmt.Printf("     GET  /swagger/           - Swagger UI\n")
	fmt.Printf("\n   REST API Endpoints:\n")
	fmt.Printf("   Health:\n")
	fmt.Printf("     GET  /health\n")
	fmt.Printf("   Rigs:\n")
	fmt.Printf("     GET  /api/rigs\n")
	fmt.Printf("     POST /api/rigs\n")
	fmt.Printf("     GET  /api/rigs/{rig}\n")
	fmt.Printf("   Jobs:\n")
	fmt.Printf("     POST /api/rigs/{rig}/jobs\n")
	fmt.Printf("     GET  /api/rigs/{rig}/jobs\n")
	fmt.Printf("     GET  /api/rigs/{rig}/jobs/{id}\n")
	fmt.Printf("     PUT  /api/rigs/{rig}/jobs/{id}\n")
	fmt.Printf("     POST /api/rigs/{rig}/jobs/{id}/close\n")
	fmt.Printf("   Sling:\n")
	fmt.Printf("     POST /api/rigs/{rig}/sling\n")
	fmt.Printf("   Merge Queue:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/mq\n")
	fmt.Printf("     POST /api/rigs/{rig}/mq/submit\n")
	fmt.Printf("   Polecats:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/polecats\n")
	fmt.Printf("     GET  /api/rigs/{rig}/polecats/{name}\n")
	fmt.Printf("     POST /api/rigs/{rig}/polecats/{name}/nuke\n")
	fmt.Printf("   Refinery:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/refinery\n")
	fmt.Printf("     POST /api/rigs/{rig}/refinery/start\n")
	fmt.Printf("     POST /api/rigs/{rig}/refinery/stop\n")
	fmt.Printf("   Mayor:\n")
	fmt.Printf("     GET  /api/mayor\n")
	fmt.Printf("     POST /api/mayor/start\n")
	fmt.Printf("     POST /api/mayor/stop\n")
	fmt.Printf("   Witness:\n")
	fmt.Printf("     GET  /api/rigs/{rig}/witness\n")
	fmt.Printf("     POST /api/rigs/{rig}/witness/start\n")
	fmt.Printf("     POST /api/rigs/{rig}/witness/stop\n")
	fmt.Printf("   Convoy:\n")
	fmt.Printf("     GET  /api/convoys\n")
	fmt.Printf("     POST /api/convoys\n")
	fmt.Printf("     GET  /api/convoys/{id}\n")
	fmt.Printf("   Mail:\n")
	fmt.Printf("     POST /api/mail/send\n")
	fmt.Printf("     GET  /api/rigs/{rig}/agents/{agent}/inbox\n")

	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// JSON response helpers
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, ErrorResponse{Error: message})
}

// ============================================================================
// Health
// ============================================================================

// handleHealth godoc
// @Summary Health check
// @Description Check if the API server is running and healthy
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		TownRoot:  s.townRoot,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ============================================================================
// Swagger/Documentation
// ============================================================================

// handleAPIDocs serves the OpenAPI specification as JSON
func (s *Server) handleAPIDocs(w http.ResponseWriter, r *http.Request) {
	docs := s.generateOpenAPISpec()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}

// handleSwagger serves a simple Swagger UI
func (s *Server) handleSwagger(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Gas Town API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "/api/docs",
                dom_id: '#swagger-ui',
                presets: [SwaggerUIBundle.presets.apis],
                layout: "BaseLayout"
            });
        }
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// generateOpenAPISpec generates the OpenAPI 3.0 specification
func (s *Server) generateOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Gas Town API",
			"description": "REST API for Gas Town - Multi-agent orchestration system for Claude Code. Gas Town coordinates AI agents (polecats) working on software development tasks, manages merge queues, and provides supervision through witnesses and refineries.",
			"version":     "1.0.0",
			"contact": map[string]string{
				"name": "Gas Town",
				"url":  "https://github.com/steveyegge/gastown",
			},
		},
		"servers": []map[string]string{
			{"url": fmt.Sprintf("http://localhost:%d", s.port), "description": "Local server"},
		},
		"tags": []map[string]string{
			{"name": "health", "description": "Health check endpoints"},
			{"name": "rigs", "description": "Project management - Rigs are isolated project environments"},
			{"name": "jobs", "description": "Job/Bead management - Work items tracked in the system"},
			{"name": "sling", "description": "Work dispatch - Assign work to polecat agents"},
			{"name": "merge-queue", "description": "Merge queue operations - Queue and process completed work"},
			{"name": "polecats", "description": "Polecat management - Ephemeral worker agents"},
			{"name": "refinery", "description": "Refinery management - Merge queue processor service"},
			{"name": "mayor", "description": "Mayor management - Global town coordinator"},
			{"name": "witness", "description": "Witness management - Polecat supervisor service"},
			{"name": "convoys", "description": "Convoy management - Batch work tracking"},
			{"name": "mail", "description": "Inter-agent messaging system"},
		},
		"paths": s.generatePaths(),
		"components": map[string]interface{}{
			"schemas": s.generateSchemas(),
		},
	}
}

func (s *Server) generatePaths() map[string]interface{} {
	return map[string]interface{}{
		"/health": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"health"},
				"summary":     "Health check",
				"description": "Check if the API server is running and healthy",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Server is healthy",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/HealthResponse"},
							},
						},
					},
				},
			},
		},
		"/api/rigs": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"rigs"},
				"summary":     "List all rigs",
				"description": "Get a list of all configured rigs (projects) in this Gas Town instance",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "List of rigs",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/RigListResponse"},
							},
						},
					},
				},
			},
		},
		"/api/rigs/{rig}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"rigs"},
				"summary":     "Get rig details",
				"description": "Get detailed information about a specific rig including config, polecat count, and crew count",
				"parameters": []map[string]interface{}{
					{
						"name":        "rig",
						"in":          "path",
						"required":    true,
						"description": "Rig name",
						"schema":      map[string]string{"type": "string"},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Rig details",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/RigDetailResponse"},
							},
						},
					},
					"404": map[string]interface{}{
						"description": "Rig not found",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/ErrorResponse"},
							},
						},
					},
				},
			},
		},
		"/api/rigs/{rig}/jobs": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"jobs"},
				"summary":     "List jobs",
				"description": "Get all jobs/beads in a rig. Filter by status or type using query parameters.",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "status", "in": "query", "description": "Filter by status (open, in_progress, closed)", "schema": map[string]string{"type": "string"}},
					{"name": "type", "in": "query", "description": "Filter by type (task, bug, feature)", "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "List of jobs",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/JobListResponse"},
							},
						},
					},
				},
			},
			"post": map[string]interface{}{
				"tags":        []string{"jobs"},
				"summary":     "Create a job",
				"description": "Create a new job/bead in the rig. Jobs are work items that can be assigned to polecats.",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/CreateJobRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"201": map[string]interface{}{
						"description": "Job created",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/JobResponse"},
							},
						},
					},
				},
			},
		},
		"/api/rigs/{rig}/jobs/{id}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"jobs"},
				"summary":     "Get job details",
				"description": "Get detailed information about a specific job",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Job details"},
					"404": map[string]interface{}{"description": "Job not found"},
				},
			},
			"put": map[string]interface{}{
				"tags":        []string{"jobs"},
				"summary":     "Update a job",
				"description": "Update job status, notes, or assignee",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"requestBody": map[string]interface{}{
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/UpdateJobRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Job updated"},
				},
			},
		},
		"/api/rigs/{rig}/jobs/{id}/close": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"jobs"},
				"summary":     "Close a job",
				"description": "Close a job with an optional reason",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"requestBody": map[string]interface{}{
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/CloseJobRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Job closed"},
				},
			},
		},
		"/api/rigs/{rig}/sling": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"sling"},
				"summary":     "Dispatch work to polecats",
				"description": "Sling a job to polecat workers. This spawns polecat agents to work on the specified bead using the given formula (workflow template).",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/SlingRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "Work dispatched",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/SlingResponse"},
							},
						},
					},
				},
			},
		},
		"/api/rigs/{rig}/mq": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"merge-queue"},
				"summary":     "List merge queue",
				"description": "Get the current state of the merge queue for a rig",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Merge queue state"},
				},
			},
		},
		"/api/rigs/{rig}/mq/submit": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"merge-queue"},
				"summary":     "Submit to merge queue",
				"description": "Submit a branch to the merge queue for processing by the refinery",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"requestBody": map[string]interface{}{
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/MQSubmitRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Submitted to queue"},
				},
			},
		},
		"/api/rigs/{rig}/polecats": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"polecats"},
				"summary":     "List polecats",
				"description": "Get all polecat workers in a rig with their current status",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{
						"description": "List of polecats",
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]string{"$ref": "#/components/schemas/PolecatListResponse"},
							},
						},
					},
				},
			},
		},
		"/api/rigs/{rig}/polecats/{name}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"polecats"},
				"summary":     "Get polecat status",
				"description": "Get detailed status of a specific polecat worker",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "name", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Polecat status"},
				},
			},
		},
		"/api/rigs/{rig}/polecats/{name}/nuke": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"polecats"},
				"summary":     "Nuke a polecat",
				"description": "Terminate and remove a polecat worker, cleaning up its worktree",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "name", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Polecat nuked"},
				},
			},
		},
		"/api/rigs/{rig}/refinery": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"refinery"},
				"summary":     "Get refinery status",
				"description": "Get the status of the refinery (merge queue processor) for a rig",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Refinery status"},
				},
			},
		},
		"/api/rigs/{rig}/refinery/start": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"refinery"},
				"summary":     "Start the refinery",
				"description": "Start the refinery service to process the merge queue",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Refinery started"},
				},
			},
		},
		"/api/rigs/{rig}/refinery/stop": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"refinery"},
				"summary":     "Stop the refinery",
				"description": "Stop the refinery service",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Refinery stopped"},
				},
			},
		},
		"/api/mayor": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"mayor"},
				"summary":     "Get mayor status",
				"description": "Get the status of the Mayor (global town coordinator). The Mayor handles cross-rig coordination and town-level operations.",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Mayor status"},
				},
			},
		},
		"/api/mayor/start": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"mayor"},
				"summary":     "Start the mayor",
				"description": "Start the Mayor session",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Mayor started"},
				},
			},
		},
		"/api/mayor/stop": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"mayor"},
				"summary":     "Stop the mayor",
				"description": "Stop the Mayor session",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Mayor stopped"},
				},
			},
		},
		"/api/rigs/{rig}/witness": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"witness"},
				"summary":     "Get witness status",
				"description": "Get the status of the Witness (polecat supervisor). The Witness monitors polecats, handles nudges, and manages cleanup.",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Witness status"},
				},
			},
		},
		"/api/rigs/{rig}/witness/start": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"witness"},
				"summary":     "Start the witness",
				"description": "Start the Witness service for a rig",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Witness started"},
				},
			},
		},
		"/api/rigs/{rig}/witness/stop": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"witness"},
				"summary":     "Stop the witness",
				"description": "Stop the Witness service for a rig",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Witness stopped"},
				},
			},
		},
		"/api/convoys": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"convoys"},
				"summary":     "List convoys",
				"description": "Get all convoys. Convoys are batch work tracking units that group related beads together.",
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "List of convoys"},
				},
			},
			"post": map[string]interface{}{
				"tags":        []string{"convoys"},
				"summary":     "Create a convoy",
				"description": "Create a new convoy to track a batch of related work items",
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/CreateConvoyRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"201": map[string]interface{}{"description": "Convoy created"},
				},
			},
		},
		"/api/convoys/{id}": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"convoys"},
				"summary":     "Get convoy status",
				"description": "Get the status of a specific convoy",
				"parameters": []map[string]interface{}{
					{"name": "id", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Convoy status"},
				},
			},
		},
		"/api/mail/send": map[string]interface{}{
			"post": map[string]interface{}{
				"tags":        []string{"mail"},
				"summary":     "Send mail",
				"description": "Send a message to an agent. Agents can be addressed as 'mayor', 'rig/witness', 'rig/polecats/name', etc.",
				"requestBody": map[string]interface{}{
					"required": true,
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{
							"schema": map[string]string{"$ref": "#/components/schemas/SendMailRequest"},
						},
					},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Mail sent"},
				},
			},
		},
		"/api/rigs/{rig}/agents/{agent}/inbox": map[string]interface{}{
			"get": map[string]interface{}{
				"tags":        []string{"mail"},
				"summary":     "Get agent inbox",
				"description": "Get the inbox for a specific agent",
				"parameters": []map[string]interface{}{
					{"name": "rig", "in": "path", "required": true, "schema": map[string]string{"type": "string"}},
					{"name": "agent", "in": "path", "required": true, "description": "Agent type (witness, refinery, polecats/name)", "schema": map[string]string{"type": "string"}},
				},
				"responses": map[string]interface{}{
					"200": map[string]interface{}{"description": "Agent inbox"},
				},
			},
		},
	}
}

func (s *Server) generateSchemas() map[string]interface{} {
	return map[string]interface{}{
		"ErrorResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"error": map[string]interface{}{"type": "string", "example": "resource not found"},
			},
		},
		"HealthResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status":    map[string]interface{}{"type": "string", "example": "ok"},
				"town_root": map[string]interface{}{"type": "string", "example": "/home/user/gt"},
				"timestamp": map[string]interface{}{"type": "string", "format": "date-time"},
			},
		},
		"RigInfo": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":       map[string]interface{}{"type": "string", "example": "myproject"},
				"path":       map[string]interface{}{"type": "string", "example": "/home/user/gt/myproject"},
				"has_beads":  map[string]interface{}{"type": "boolean"},
				"has_config": map[string]interface{}{"type": "boolean"},
			},
		},
		"RigListResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rigs":  map[string]interface{}{"type": "array", "items": map[string]string{"$ref": "#/components/schemas/RigInfo"}},
				"count": map[string]interface{}{"type": "integer"},
			},
		},
		"RigDetailResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":          map[string]interface{}{"type": "string"},
				"path":          map[string]interface{}{"type": "string"},
				"config":        map[string]interface{}{"type": "object"},
				"polecat_count": map[string]interface{}{"type": "integer"},
				"crew_count":    map[string]interface{}{"type": "integer"},
			},
		},
		"CreateJobRequest": map[string]interface{}{
			"type":     "object",
			"required": []string{"title"},
			"properties": map[string]interface{}{
				"title":       map[string]interface{}{"type": "string", "description": "Job title", "example": "Implement user authentication"},
				"description": map[string]interface{}{"type": "string", "description": "Detailed description"},
				"type":        map[string]interface{}{"type": "string", "enum": []string{"task", "bug", "feature"}, "default": "task"},
				"priority":    map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 5, "default": 3},
			},
		},
		"UpdateJobRequest": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status":      map[string]interface{}{"type": "string", "enum": []string{"open", "in_progress", "closed"}},
				"notes":       map[string]interface{}{"type": "string"},
				"description": map[string]interface{}{"type": "string"},
				"assignee":    map[string]interface{}{"type": "string", "example": "myproject/polecats/jasper"},
			},
		},
		"CloseJobRequest": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{"type": "string", "example": "Completed successfully"},
			},
		},
		"JobListResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"jobs":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}},
				"count": map[string]interface{}{"type": "integer"},
			},
		},
		"JobResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":          map[string]interface{}{"type": "string"},
				"title":       map[string]interface{}{"type": "string"},
				"description": map[string]interface{}{"type": "string"},
				"type":        map[string]interface{}{"type": "string"},
				"status":      map[string]interface{}{"type": "string"},
				"priority":    map[string]interface{}{"type": "integer"},
				"assignee":    map[string]interface{}{"type": "string"},
				"created_at":  map[string]interface{}{"type": "string", "format": "date-time"},
			},
		},
		"SlingRequest": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"bead_id":   map[string]interface{}{"type": "string", "description": "Bead ID to work on"},
				"formula":   map[string]interface{}{"type": "string", "description": "Formula (workflow) to execute", "example": "mol-polecat-work"},
				"args":      map[string]interface{}{"type": "string", "description": "Additional arguments"},
				"no_convoy": map[string]interface{}{"type": "boolean", "description": "Skip convoy creation"},
			},
		},
		"SlingResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status":  map[string]interface{}{"type": "string", "example": "slung"},
				"bead_id": map[string]interface{}{"type": "string"},
				"rig":     map[string]interface{}{"type": "string"},
				"output":  map[string]interface{}{"type": "string"},
			},
		},
		"MQSubmitRequest": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"branch":   map[string]interface{}{"type": "string", "description": "Branch to submit"},
				"issue":    map[string]interface{}{"type": "string", "description": "Associated issue ID"},
				"priority": map[string]interface{}{"type": "integer", "description": "Priority in queue"},
			},
		},
		"PolecatListResponse": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"polecats": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name":            map[string]interface{}{"type": "string"},
							"rig":             map[string]interface{}{"type": "string"},
							"state":           map[string]interface{}{"type": "string"},
							"session_running": map[string]interface{}{"type": "boolean"},
						},
					},
				},
			},
		},
		"CreateConvoyRequest": map[string]interface{}{
			"type":     "object",
			"required": []string{"title", "tracks"},
			"properties": map[string]interface{}{
				"title":  map[string]interface{}{"type": "string", "description": "Convoy title"},
				"tracks": map[string]interface{}{"type": "array", "items": map[string]string{"type": "string"}, "description": "Bead IDs to track"},
			},
		},
		"SendMailRequest": map[string]interface{}{
			"type":     "object",
			"required": []string{"to", "subject"},
			"properties": map[string]interface{}{
				"to":      map[string]interface{}{"type": "string", "description": "Recipient address", "example": "myproject/witness"},
				"subject": map[string]interface{}{"type": "string", "description": "Subject line"},
				"body":    map[string]interface{}{"type": "string", "description": "Message body"},
			},
		},
	}
}

// ============================================================================
// Rigs
// ============================================================================

// handleListRigs godoc
// @Summary List all rigs
// @Description Get a list of all configured rigs (projects)
// @Tags rigs
// @Accept json
// @Produce json
// @Success 200 {object} RigListResponse
// @Router /api/rigs [get]
func (s *Server) handleListRigs(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.townRoot)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rigs []RigInfo
	skipDirs := map[string]bool{
		"mayor": true, "deacon": true, ".beads": true,
		".claude": true, ".git": true, "plugins": true,
		"logs": true, "settings": true, "daemon": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() || skipDirs[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigPath := filepath.Join(s.townRoot, entry.Name())
		configPath := filepath.Join(rigPath, "config.json")
		beadsPath := filepath.Join(rigPath, ".beads")

		_, hasConfig := os.Stat(configPath)
		_, hasBeads := os.Stat(beadsPath)

		if hasConfig == nil {
			rigs = append(rigs, RigInfo{
				Name:      entry.Name(),
				Path:      rigPath,
				HasConfig: hasConfig == nil,
				HasBeads:  hasBeads == nil,
			})
		}
	}

	jsonResponse(w, http.StatusOK, RigListResponse{
		Rigs:  rigs,
		Count: len(rigs),
	})
}

// handleCreateRig godoc
// @Summary Create a new rig
// @Description Create a new rig by cloning a git repository
// @Tags rigs
// @Accept json
// @Produce json
// @Param request body CreateRigRequest true "Rig creation request"
// @Success 201 {object} CreateRigResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/rigs [post]
func (s *Server) handleCreateRig(w http.ResponseWriter, r *http.Request) {
	var req CreateRigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.GitURL == "" {
		jsonError(w, http.StatusBadRequest, "git_url is required")
		return
	}

	// Check if rig already exists
	rigPath := filepath.Join(s.townRoot, req.Name)
	if _, err := os.Stat(rigPath); err == nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("rig already exists: %s", req.Name))
		return
	}

	// Build gt rig add command
	args := []string{"rig", "add", req.Name, req.GitURL}
	if req.Branch != "" {
		args = append(args, "--branch", req.Branch)
	}
	if req.Prefix != "" {
		args = append(args, "--prefix", req.Prefix)
	}

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt rig add failed: %s %s", stderr, err))
		return
	}

	// Check if rig was created
	configPath := filepath.Join(rigPath, "config.json")
	beadsPath := filepath.Join(rigPath, ".beads")
	_, hasConfig := os.Stat(configPath)
	_, hasBeads := os.Stat(beadsPath)

	jsonResponse(w, http.StatusCreated, CreateRigResponse{
		Status: "created",
		Rig: RigInfo{
			Name:      req.Name,
			Path:      rigPath,
			HasConfig: hasConfig == nil,
			HasBeads:  hasBeads == nil,
		},
		Output: stdout,
	})
}

// handleGetRig godoc
// @Summary Get rig details
// @Description Get detailed information about a specific rig
// @Tags rigs
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Success 200 {object} RigDetailResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/rigs/{rig} [get]
func (s *Server) handleGetRig(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	rigPath := filepath.Join(s.townRoot, rigName)

	configPath := filepath.Join(rigPath, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig not found: %s", rigName))
		return
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Count polecats
	polecatsPath := filepath.Join(rigPath, "polecats")
	polecatCount := 0
	if entries, err := os.ReadDir(polecatsPath); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				polecatCount++
			}
		}
	}

	// Count crew
	crewPath := filepath.Join(rigPath, "crew")
	crewCount := 0
	if entries, err := os.ReadDir(crewPath); err == nil {
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				crewCount++
			}
		}
	}

	jsonResponse(w, http.StatusOK, RigDetailResponse{
		Name:         rigName,
		Path:         rigPath,
		Config:       config,
		PolecatCount: polecatCount,
		CrewCount:    crewCount,
	})
}

// ============================================================================
// Jobs (Beads)
// ============================================================================

// handleCreateJob godoc
// @Summary Create a job
// @Description Create a new job/bead in the rig
// @Tags jobs
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Param job body CreateJobRequest true "Job details"
// @Success 201 {object} JobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/rigs/{rig}/jobs [post]
func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Title == "" {
		jsonError(w, http.StatusBadRequest, "title is required")
		return
	}

	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	issueType := req.Type
	if issueType == "" {
		issueType = "task"
	}

	opts := beads.CreateOptions{
		Title:       req.Title,
		Description: req.Description,
		Type:        issueType,
		Priority:    req.Priority,
	}

	issue, err := b.Create(opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, issue)
}

// handleListJobs godoc
// @Summary List jobs
// @Description Get all jobs in a rig with optional filters
// @Tags jobs
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Param status query string false "Filter by status"
// @Param type query string false "Filter by type"
// @Success 200 {object} JobListResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/rigs/{rig}/jobs [get]
func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	status := r.URL.Query().Get("status")
	issueType := r.URL.Query().Get("type")

	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	opts := beads.ListOptions{
		Status: status,
		Type:   issueType,
	}

	issues, err := b.List(opts)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to interface slice for JSON
	jobList := make([]interface{}, len(issues))
	for i, issue := range issues {
		jobList[i] = issue
	}

	jsonResponse(w, http.StatusOK, JobListResponse{
		Jobs:  jobList,
		Count: len(issues),
	})
}

// handleGetJob godoc
// @Summary Get job details
// @Description Get detailed information about a specific job
// @Tags jobs
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Param id path string true "Job ID"
// @Success 200 {object} JobResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/rigs/{rig}/jobs/{id} [get]
func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	jobID := r.PathValue("id")

	rigBeadsDir := filepath.Join(s.townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeadsDir); os.IsNotExist(err) {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("rig %s has no beads directory", rigName))
		return
	}

	b := beads.New(rigBeadsDir)

	issue, err := b.Show(jobID)
	if err != nil {
		jsonError(w, http.StatusNotFound, fmt.Sprintf("job not found: %s", jobID))
		return
	}

	jsonResponse(w, http.StatusOK, issue)
}

// handleUpdateJob godoc
// @Summary Update a job
// @Description Update job status, notes, or assignee
// @Tags jobs
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Param id path string true "Job ID"
// @Param update body UpdateJobRequest true "Update details"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/rigs/{rig}/jobs/{id} [put]
func (s *Server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	jobID := r.PathValue("id")

	var req UpdateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Build bd update command
	args := []string{"update", jobID}
	if req.Status != "" {
		args = append(args, "--status", req.Status)
	}
	if req.Notes != "" {
		args = append(args, "--notes", req.Notes)
	}
	if req.Assignee != "" {
		args = append(args, "--assignee", req.Assignee)
	}

	stdout, stderr, err := s.runBD(rigName, args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("bd update failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
		"job_id": jobID,
		"output": stdout,
	})
}

// handleCloseJob godoc
// @Summary Close a job
// @Description Close a job with an optional reason
// @Tags jobs
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Param id path string true "Job ID"
// @Param close body CloseJobRequest false "Close details"
// @Success 200 {object} SuccessResponse
// @Router /api/rigs/{rig}/jobs/{id}/close [post]
func (s *Server) handleCloseJob(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	jobID := r.PathValue("id")

	var req CloseJobRequest
	json.NewDecoder(r.Body).Decode(&req) // Optional body

	args := []string{"close", jobID}
	if req.Reason != "" {
		args = append(args, "--reason", req.Reason)
	}

	stdout, stderr, err := s.runBD(rigName, args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("bd close failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status": "closed",
		"job_id": jobID,
		"output": stdout,
	})
}

// ============================================================================
// Sling
// ============================================================================

// handleSling godoc
// @Summary Dispatch work to polecats
// @Description Sling a job to polecat workers using a formula
// @Tags sling
// @Accept json
// @Produce json
// @Param rig path string true "Rig name"
// @Param sling body SlingRequest true "Sling request"
// @Success 200 {object} SlingResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/rigs/{rig}/sling [post]
func (s *Server) handleSling(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req SlingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.BeadID == "" && req.Formula == "" {
		jsonError(w, http.StatusBadRequest, "bead_id or formula is required")
		return
	}

	// Build gt sling command
	args := []string{"sling"}
	if req.Formula != "" {
		args = append(args, req.Formula)
		if req.BeadID != "" {
			args = append(args, "--on", req.BeadID)
		}
	} else {
		args = append(args, req.BeadID)
	}
	args = append(args, rigName)

	if req.Args != "" {
		args = append(args, "--args", req.Args)
	}
	if req.NoConvoy {
		args = append(args, "--no-convoy")
	}

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt sling failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, SlingResponse{
		Status: "slung",
		BeadID: req.BeadID,
		Rig:    rigName,
		Output: stdout,
	})
}

// ============================================================================
// Merge Queue
// ============================================================================

func (s *Server) handleListMergeQueue(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("mq", "list", rigName, "--json")
	if err != nil {
		// Try without --json if it fails
		stdout, stderr, err = s.runGT("mq", "list", rigName)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mq list failed: %s %s", stderr, err))
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) handleMQSubmit(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	var req MQSubmitRequest
	json.NewDecoder(r.Body).Decode(&req)

	args := []string{"mq", "submit"}
	if req.Branch != "" {
		args = append(args, "--branch", req.Branch)
	}
	if req.Issue != "" {
		args = append(args, "--issue", req.Issue)
	}

	// Run from rig directory
	cmd := exec.Command(s.gtBinary, args...)
	cmd.Dir = filepath.Join(s.townRoot, rigName, "refinery", "rig")
	cmd.Env = append(os.Environ(), "GT_ROOT="+s.townRoot)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mq submit failed: %s %s", stderr.String(), err))
		return
	}

	jsonResponse(w, http.StatusOK, MQSubmitResponse{
		Status: "submitted",
		Output: stdout.String(),
	})
}

// ============================================================================
// Polecats
// ============================================================================

func (s *Server) handleListPolecats(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("polecat", "list", rigName, "--json")
	if err != nil {
		// Fallback to non-JSON
		stdout, _, _ = s.runGT("polecat", "list", rigName)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
			"error":      stderr,
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	jsonResponse(w, http.StatusOK, PolecatListResponse{
		Polecats: result,
	})
}

func (s *Server) handleGetPolecat(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	name := r.PathValue("name")

	fullName := fmt.Sprintf("%s/%s", rigName, name)
	stdout, stderr, err := s.runGT("polecat", "status", fullName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt polecat status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, PolecatStatusResponse{
		Name:   name,
		Rig:    rigName,
		Status: stdout,
	})
}

func (s *Server) handleNukePolecat(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	name := r.PathValue("name")

	fullName := fmt.Sprintf("%s/%s", rigName, name)
	stdout, stderr, err := s.runGT("polecat", "nuke", fullName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt polecat nuke failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, PolecatNukeResponse{
		Status: "nuked",
		Name:   name,
		Rig:    rigName,
		Output: stdout,
	})
}

// ============================================================================
// Refinery
// ============================================================================

func (s *Server) handleRefineryStatus(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("refinery", "status", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt refinery status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, RefineryStatusResponse{
		Rig:    rigName,
		Status: stdout,
	})
}

func (s *Server) handleRefineryStart(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("refinery", "start", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt refinery start failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, RefineryActionResponse{
		Status: "started",
		Rig:    rigName,
		Output: stdout,
	})
}

func (s *Server) handleRefineryStop(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("refinery", "stop", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt refinery stop failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, RefineryActionResponse{
		Status: "stopped",
		Rig:    rigName,
		Output: stdout,
	})
}

// ============================================================================
// Mayor
// ============================================================================

func (s *Server) handleMayorStatus(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("mayor", "status")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mayor status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, MayorStatusResponse{
		Status: stdout,
	})
}

func (s *Server) handleMayorStart(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("mayor", "start")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mayor start failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, MayorActionResponse{
		Status: "started",
		Output: stdout,
	})
}

func (s *Server) handleMayorStop(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("mayor", "stop")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mayor stop failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, MayorActionResponse{
		Status: "stopped",
		Output: stdout,
	})
}

// ============================================================================
// Witness
// ============================================================================

func (s *Server) handleWitnessStatus(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("witness", "status", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt witness status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, WitnessStatusResponse{
		Rig:    rigName,
		Status: stdout,
	})
}

func (s *Server) handleWitnessStart(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("witness", "start", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt witness start failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, WitnessActionResponse{
		Status: "started",
		Rig:    rigName,
		Output: stdout,
	})
}

func (s *Server) handleWitnessStop(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")

	stdout, stderr, err := s.runGT("witness", "stop", rigName)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt witness stop failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, WitnessActionResponse{
		Status: "stopped",
		Rig:    rigName,
		Output: stdout,
	})
}

// ============================================================================
// Convoy
// ============================================================================

func (s *Server) handleListConvoys(w http.ResponseWriter, r *http.Request) {
	stdout, stderr, err := s.runGT("convoy", "list", "--json")
	if err != nil {
		stdout, _, _ = s.runGT("convoy", "list")
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
			"error":      stderr,
		})
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"raw_output": stdout,
		})
		return
	}

	jsonResponse(w, http.StatusOK, ConvoyListResponse{
		Convoys: result,
	})
}

func (s *Server) handleCreateConvoy(w http.ResponseWriter, r *http.Request) {
	var req CreateConvoyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Title == "" || len(req.Tracks) == 0 {
		jsonError(w, http.StatusBadRequest, "title and tracks are required")
		return
	}

	args := []string{"convoy", "create", "--title", req.Title}
	args = append(args, req.Tracks...)

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt convoy create failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"status": "created",
		"output": stdout,
	})
}

func (s *Server) handleGetConvoy(w http.ResponseWriter, r *http.Request) {
	convoyID := r.PathValue("id")

	stdout, stderr, err := s.runGT("convoy", "status", convoyID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt convoy status failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, ConvoyStatusResponse{
		ConvoyID: convoyID,
		Status:   stdout,
	})
}

// ============================================================================
// Mail
// ============================================================================

func (s *Server) handleSendMail(w http.ResponseWriter, r *http.Request) {
	var req SendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.To == "" || req.Subject == "" {
		jsonError(w, http.StatusBadRequest, "to and subject are required")
		return
	}

	args := []string{"mail", "send", req.To, "-s", req.Subject}
	if req.Body != "" {
		args = append(args, "-m", req.Body)
	}

	stdout, stderr, err := s.runGT(args...)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mail send failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, SendMailResponse{
		Status: "sent",
		To:     req.To,
		Output: stdout,
	})
}

func (s *Server) handleGetInbox(w http.ResponseWriter, r *http.Request) {
	rigName := r.PathValue("rig")
	agent := r.PathValue("agent")

	// Construct agent address
	agentAddr := fmt.Sprintf("%s/%s", rigName, agent)

	stdout, stderr, err := s.runGT("mail", "inbox", agentAddr)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("gt mail inbox failed: %s %s", stderr, err))
		return
	}

	jsonResponse(w, http.StatusOK, InboxResponse{
		Agent: agentAddr,
		Inbox: stdout,
	})
}
