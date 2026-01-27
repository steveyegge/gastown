// Package main implements the Gas Town mobile RPC server using Connect-RPC.
//
// This server exposes StatusService, MailService, and DecisionService
// for mobile client access to Gas Town functionality.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"connectrpc.com/connect"

	"github.com/steveyegge/gastown/internal/eventbus"
	"github.com/steveyegge/gastown/mobile/gen/gastown/v1/gastownv1connect"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	port     = flag.Int("port", 8443, "Server port")
	townRoot = flag.String("town", "", "Town root directory (auto-detected if not set)")
	apiKey   = flag.String("api-key", "", "API key for authentication (optional)")
	certFile = flag.String("cert", "", "TLS certificate file (optional)")
	keyFile  = flag.String("key", "", "TLS key file (optional)")
)

func main() {
	flag.Parse()

	// Find town root
	root := *townRoot
	if root == "" {
		var err error
		root, err = workspace.FindFromCwdOrError()
		if err != nil {
			log.Fatalf("Not in a Gas Town workspace: %v", err)
		}
	}

	// Create event bus for real-time decision notifications
	decisionBus := eventbus.New()
	defer decisionBus.Close()

	// Create service handlers
	statusServer := NewStatusServer(root)
	mailServer := NewMailServer(root)
	decisionServer := NewDecisionServer(root, decisionBus)

	// Set up interceptors
	var opts []connect.HandlerOption
	if *apiKey != "" {
		opts = append(opts, connect.WithInterceptors(APIKeyInterceptor(*apiKey)))
		log.Printf("API key authentication enabled")
	}

	// Create HTTP mux with Connect handlers
	mux := http.NewServeMux()

	// Register services
	statusPath, statusHandler := gastownv1connect.NewStatusServiceHandler(statusServer, opts...)
	mux.Handle(statusPath, statusHandler)

	mailPath, mailHandler := gastownv1connect.NewMailServiceHandler(mailServer, opts...)
	mux.Handle(mailPath, mailHandler)

	decisionPath, decisionHandler := gastownv1connect.NewDecisionServiceHandler(decisionServer, opts...)
	mux.Handle(decisionPath, decisionHandler)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	// SSE endpoint for decision events (browser-friendly streaming)
	mux.HandleFunc("/events/decisions", NewSSEHandler(decisionBus, root))

	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics := decisionBus.Metrics()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"events_published":%d,"events_delivered":%d,"events_dropped":%d,"subscribers_active":%d,"subscribers_total":%d}`,
			metrics.EventsPublished, metrics.EventsDelivered, metrics.EventsDropped,
			metrics.SubscribersActive, metrics.SubscribersTotal)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Gas Town Mobile Server starting on %s", addr)
	log.Printf("Town root: %s", root)
	log.Printf("Services:")
	log.Printf("  %s", statusPath)
	log.Printf("  %s", mailPath)
	log.Printf("  %s", decisionPath)
	log.Printf("  /health")

	// Start server (TLS or plain HTTP)
	if *certFile != "" && *keyFile != "" {
		tlsConfig, err := LoadTLSConfig(*certFile, *keyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS config: %v", err)
		}
		server := &http.Server{
			Addr:      addr,
			Handler:   mux,
			TLSConfig: tlsConfig,
		}
		log.Printf("TLS enabled")
		if err := server.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	} else {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}
}
