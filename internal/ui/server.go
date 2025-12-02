// Package ui provides the web dashboard for VPN monitoring.
package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/miguelemosreverte/vpn/internal/cli"
	"github.com/miguelemosreverte/vpn/internal/protocol"
)

//go:embed static/*
var staticFiles embed.FS

// Server serves the web dashboard.
type Server struct {
	nodeAddr   string
	listenAddr string
	client     *cli.Client
}

// NewServer creates a new UI server.
func NewServer(nodeAddr, listenAddr string) *Server {
	return &Server{
		nodeAddr:   nodeAddr,
		listenAddr: listenAddr,
	}
}

// Start starts the web server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/peers", s.handlePeers)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/verify", s.handleVerify)
	mux.HandleFunc("/api/connection", s.handleConnection)
	mux.HandleFunc("/api/topology", s.handleTopology)

	// Static files and SPA
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to get static files: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", s.handleIndex)

	fmt.Printf("\n")
	fmt.Printf("  VPN Dashboard starting...\n")
	fmt.Printf("  ────────────────────────────────────────\n")
	fmt.Printf("  URL:  http://%s\n", s.listenAddr)
	fmt.Printf("  Node: %s\n", s.nodeAddr)
	fmt.Printf("  ────────────────────────────────────────\n")
	fmt.Printf("  Press Ctrl+C to stop\n\n")

	return http.ListenAndServe(s.listenAddr, mux)
}

func (s *Server) getClient() (*cli.Client, error) {
	return cli.NewClient(s.nodeAddr)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("index").Parse(indexHTML))
	tmpl.Execute(w, nil)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	client, err := s.getClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	status, err := client.Status()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	client, err := s.getClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	peers, err := client.Peers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	client, err := s.getClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	params := protocol.StatsParams{
		Earliest:    r.URL.Query().Get("earliest"),
		Latest:      r.URL.Query().Get("latest"),
		Granularity: r.URL.Query().Get("granularity"),
	}
	if params.Earliest == "" {
		params.Earliest = "-5m"
	}
	if params.Latest == "" {
		params.Latest = "now"
	}
	if params.Granularity == "" {
		params.Granularity = "auto"
	}

	if metrics := r.URL.Query().Get("metrics"); metrics != "" {
		params.Metrics = []string{metrics}
	}

	stats, err := client.Stats(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	client, err := s.getClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	params := protocol.LogsParams{
		Earliest: r.URL.Query().Get("earliest"),
		Latest:   r.URL.Query().Get("latest"),
		Search:   r.URL.Query().Get("search"),
		Limit:    100,
	}
	if params.Earliest == "" {
		params.Earliest = "-15m"
	}
	if params.Latest == "" {
		params.Latest = "now"
	}

	if level := r.URL.Query().Get("level"); level != "" {
		params.Levels = []string{level}
	}
	if component := r.URL.Query().Get("component"); component != "" {
		params.Components = []string{component}
	}

	logs, err := client.Logs(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	// Fetch public IP from an external service
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"public_ip": "unknown",
			"error":     err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"public_ip": "unknown",
			"error":     err.Error(),
		})
		return
	}

	publicIP := strings.TrimSpace(string(body))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"public_ip": publicIP,
	})
}

func (s *Server) handleConnection(w http.ResponseWriter, r *http.Request) {
	client, err := s.getClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	if r.Method == http.MethodPost {
		// Connect or disconnect based on action
		action := r.URL.Query().Get("action")
		var result *protocol.ConnectionResult
		var connErr error

		if action == "disconnect" {
			result, connErr = client.Disconnect()
		} else {
			result, connErr = client.Connect()
		}

		if connErr != nil {
			http.Error(w, connErr.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	// GET - return current connection status
	status, err := client.ConnectionStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	client, err := s.getClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	topology, err := client.Topology()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topology)
}

// Placeholder for compile - will be replaced with actual HTML
var indexHTML = `<!DOCTYPE html>
<html>
<head><title>VPN Dashboard</title></head>
<body>Loading...</body>
</html>`

func init() {
	// Initialize time location
	time.Local = time.UTC
}
