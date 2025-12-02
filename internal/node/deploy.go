package node

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/miguelemosreverte/vpn/internal/protocol"
)

// DeployRequest is the payload from GitHub Actions or manual trigger.
type DeployRequest struct {
	Ref    string `json:"ref"`    // Git SHA
	Branch string `json:"branch"` // Branch name
}

// DeployResponse is sent back to the webhook caller.
type DeployResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Node    string `json:"node"`
}

// StartDeployServer starts the HTTP server for deploy webhooks.
// This runs on the WebSocket port alongside any other HTTP handlers.
func (d *Daemon) StartDeployServer(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/deploy", d.handleDeploy)
	mux.HandleFunc("/health", d.handleHealth)

	log.Printf("[deploy] Webhook server starting on %s", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[deploy] Server error: %v", err)
		}
	}()

	return nil
}

// handleHealth returns a simple health check.
func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "healthy",
		"node":    d.config.NodeName,
		"uptime":  d.Uptime().String(),
		"version": version,
	})
}

// handleDeploy handles the deploy webhook.
func (d *Daemon) handleDeploy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Optional: verify deploy token
	// token := r.Header.Get("X-Deploy-Token")
	// if token != expectedToken { ... }

	// Parse request
	var req DeployRequest
	if r.Body != nil {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
	}

	log.Printf("[deploy] Received deploy request: ref=%s branch=%s", req.Ref, req.Branch)

	// Respond immediately (async deployment)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(DeployResponse{
		Success: true,
		Message: "Deploy triggered, propagating to network",
		Node:    d.config.NodeName,
	})

	// Perform deployment asynchronously
	go d.performDeploy(req)
}

// performDeploy does the actual deployment work.
func (d *Daemon) performDeploy(req DeployRequest) {
	log.Printf("[deploy] Starting deployment on %s", d.config.NodeName)

	// 1. Git pull
	if err := d.gitPull(); err != nil {
		log.Printf("[deploy] Git pull failed: %v", err)
		return
	}

	// 2. Check what needs updating based on VERSION files
	updates := d.checkVersionChanges()

	// 3. Rebuild binaries if needed
	if updates.NeedsRebuild {
		if err := d.rebuildBinaries(); err != nil {
			log.Printf("[deploy] Rebuild failed: %v", err)
			return
		}
	}

	// 4. Broadcast UPDATE_AVAILABLE to all connected peers
	d.broadcastUpdate()

	// 5. Restart services if needed (after broadcast so clients get notified)
	if updates.RestartNode {
		log.Printf("[deploy] Node restart required, scheduling...")
		// Give peers time to receive the update notification
		time.Sleep(2 * time.Second)
		d.scheduleRestart()
	} else if updates.RestartCLI {
		log.Printf("[deploy] CLI-only update, no restart needed")
	}

	log.Printf("[deploy] Deployment complete on %s", d.config.NodeName)
}

// VersionUpdates indicates what needs to be updated.
type VersionUpdates struct {
	NeedsRebuild bool
	RestartNode  bool
	RestartCLI   bool
}

// checkVersionChanges checks VERSION files to determine what changed.
func (d *Daemon) checkVersionChanges() VersionUpdates {
	// Find project root (where go.mod is)
	projectRoot := d.findProjectRoot()
	if projectRoot == "" {
		log.Printf("[deploy] Could not find project root, assuming full rebuild")
		return VersionUpdates{NeedsRebuild: true, RestartNode: true}
	}

	updates := VersionUpdates{}

	// Check services/core/VERSION for node changes
	coreVersion := d.readVersionFile(filepath.Join(projectRoot, "services", "core", "VERSION"))
	storedCoreVersion := d.readStoredVersion("core")
	if coreVersion != storedCoreVersion && coreVersion != "" {
		log.Printf("[deploy] Core version changed: %s -> %s", storedCoreVersion, coreVersion)
		updates.NeedsRebuild = true
		updates.RestartNode = true
		d.storeVersion("core", coreVersion)
	}

	// Check services/cli/VERSION for CLI changes
	cliVersion := d.readVersionFile(filepath.Join(projectRoot, "services", "cli", "VERSION"))
	storedCLIVersion := d.readStoredVersion("cli")
	if cliVersion != storedCLIVersion && cliVersion != "" {
		log.Printf("[deploy] CLI version changed: %s -> %s", storedCLIVersion, cliVersion)
		updates.NeedsRebuild = true
		updates.RestartCLI = true
		d.storeVersion("cli", cliVersion)
	}

	// If no VERSION files, always rebuild (simple approach)
	if !updates.NeedsRebuild {
		log.Printf("[deploy] No VERSION file changes detected, rebuilding anyway")
		updates.NeedsRebuild = true
	}

	return updates
}

// gitPull performs git pull in the project directory.
func (d *Daemon) gitPull() error {
	projectRoot := d.findProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("could not find project root")
	}

	log.Printf("[deploy] Running git pull in %s", projectRoot)

	cmd := exec.Command("git", "pull", "origin", "main")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()

	log.Printf("[deploy] git pull output: %s", string(output))

	if err != nil {
		return fmt.Errorf("git pull failed: %w: %s", err, output)
	}

	return nil
}

// rebuildBinaries rebuilds the VPN binaries.
func (d *Daemon) rebuildBinaries() error {
	projectRoot := d.findProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("could not find project root")
	}

	log.Printf("[deploy] Rebuilding binaries...")

	// Build vpn-node
	cmd := exec.Command("go", "build", "-o", "bin/vpn-node", "./cmd/vpn-node")
	cmd.Dir = projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build vpn-node: %w: %s", err, output)
	}

	// Build vpn CLI
	cmd = exec.Command("go", "build", "-o", "bin/vpn", "./cmd/vpn")
	cmd.Dir = projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to build vpn: %w: %s", err, output)
	}

	log.Printf("[deploy] Binaries rebuilt successfully")
	return nil
}

// broadcastUpdate sends UPDATE_AVAILABLE to all connected peers.
func (d *Daemon) broadcastUpdate() {
	msg := protocol.MakeControlMessage(protocol.CmdUpdateAvailable)

	d.peerConnsMu.RLock()
	defer d.peerConnsMu.RUnlock()

	log.Printf("[deploy] Broadcasting UPDATE_AVAILABLE to %d peers", len(d.peerConns))

	for vpnIP, conn := range d.peerConns {
		if err := conn.WritePacket(msg); err != nil {
			log.Printf("[deploy] Failed to notify %s: %v", vpnIP, err)
		} else {
			log.Printf("[deploy] Notified peer %s", vpnIP)
		}
	}
}

// scheduleRestart schedules a graceful restart of the node.
func (d *Daemon) scheduleRestart() {
	// For now, just log - in production you'd use systemd or supervisor
	log.Printf("[deploy] Restart would be performed here")
	log.Printf("[deploy] In production, use: systemctl restart vpn-node")

	// You could also exec the new binary:
	// executable, _ := os.Executable()
	// syscall.Exec(executable, os.Args, os.Environ())
}

// findProjectRoot finds the project root directory (where go.mod is).
func (d *Daemon) findProjectRoot() string {
	// Try common locations
	locations := []string{
		"/root/vpn-source",                       // Server (Hetzner)
		"/root/the-family-vpn",                   // Server (legacy)
		os.Getenv("HOME") + "/the-family-vpn",    // macOS clients
		os.Getenv("HOME") + "/vpn",               // Alternative
		".",
	}

	// Also try to find it from the executable path
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(filepath.Dir(exe)) // Go up from bin/
		locations = append([]string{dir}, locations...)
	}

	for _, loc := range locations {
		if _, err := os.Stat(filepath.Join(loc, "go.mod")); err == nil {
			return loc
		}
	}

	return ""
}

// readVersionFile reads a VERSION file.
func (d *Daemon) readVersionFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// readStoredVersion reads a stored version from the data directory.
func (d *Daemon) readStoredVersion(name string) string {
	dataDir := d.config.DataDir
	if dataDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataDir = filepath.Join(home, ".vpn-node")
		}
	}
	path := filepath.Join(dataDir, "versions", name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// storeVersion stores a version in the data directory.
func (d *Daemon) storeVersion(name, version string) {
	dataDir := d.config.DataDir
	if dataDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataDir = filepath.Join(home, ".vpn-node")
		}
	}
	dir := filepath.Join(dataDir, "versions")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, name)
	os.WriteFile(path, []byte(version), 0644)
}

// HandleUpdateMessage handles an UPDATE_AVAILABLE control message (client mode).
func (d *Daemon) HandleUpdateMessage() {
	log.Printf("[deploy] Received UPDATE_AVAILABLE from server")

	// Perform the same deployment steps
	go d.performDeploy(DeployRequest{})
}
