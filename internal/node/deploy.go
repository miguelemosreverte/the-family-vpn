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
	"runtime"
	"strings"
	"syscall"
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
		"version": Version,
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
	log.Printf("[deploy] Starting deployment on %s (server=%v)", d.config.NodeName, d.config.ServerMode)

	// 1. Git pull
	if err := d.gitPull(); err != nil {
		log.Printf("[deploy] Git pull failed: %v", err)
		return
	}

	// 2. Check what needs updating based on VERSION files
	updates := d.checkVersionChanges()

	// 3. Rebuild binaries selectively
	if updates.RebuildNode || updates.RebuildCLI {
		if err := d.rebuildBinariesSelective(updates); err != nil {
			log.Printf("[deploy] Rebuild failed: %v", err)
			return
		}
	} else {
		log.Printf("[deploy] No rebuilds needed")
	}

	// 4. Server-only: Broadcast UPDATE_AVAILABLE to all connected peers
	if d.config.ServerMode {
		d.broadcastUpdate()
	}

	// 5. Restart logic:
	// - SERVER: Restart if frozen/cold layer changed (core/websocket)
	// - CLIENT: NEVER restart automatically. VPN stability is more important.
	//           Client restarts require manual intervention or the server
	//           will notify on reconnect if protocol is incompatible.
	if updates.RestartNode {
		if d.config.ServerMode {
			log.Printf("[deploy] Node restart required (core/websocket changed), scheduling...")
			// Give peers time to receive the update notification
			time.Sleep(2 * time.Second)
			d.scheduleRestart()
		} else {
			// Client mode: DO NOT restart. Log that a restart would be needed.
			log.Printf("[deploy] Core/websocket updated but client will NOT restart automatically")
			log.Printf("[deploy] VPN connection stability prioritized over immediate update")
			log.Printf("[deploy] Client will get updates on next manual restart or reconnect")
		}
	} else if updates.RebuildCLI {
		log.Printf("[deploy] HOT update complete - CLI/UI rebuilt, VPN connection uninterrupted")
	}

	log.Printf("[deploy] Deployment complete on %s", d.config.NodeName)
}

// VersionUpdates indicates what needs to be updated.
type VersionUpdates struct {
	RebuildNode bool // Rebuild vpn-node binary
	RebuildCLI  bool // Rebuild vpn CLI binary
	RestartNode bool // Restart vpn-node service (interrupts VPN)
}

// checkVersionChanges checks VERSION files to determine what changed.
// Service layers:
//   - core, websocket: FROZEN/COLD - requires node restart
//   - cli, ui: HOT - no node restart, just rebuild CLI binary
func (d *Daemon) checkVersionChanges() VersionUpdates {
	// Find project root (where go.mod is)
	projectRoot := d.findProjectRoot()
	if projectRoot == "" {
		log.Printf("[deploy] Could not find project root, assuming full rebuild")
		return VersionUpdates{RebuildNode: true, RebuildCLI: true, RestartNode: true}
	}

	updates := VersionUpdates{}

	// === FROZEN/COLD layer: core and websocket ===
	// These require node restart (interrupts VPN connection)

	// Check services/core/VERSION for core node changes
	coreVersion := d.readVersionFile(filepath.Join(projectRoot, "services", "core", "VERSION"))
	storedCoreVersion := d.readStoredVersion("core")
	if coreVersion != "" {
		if storedCoreVersion == "" {
			// First time seeing this version file - just initialize, don't rebuild
			log.Printf("[deploy] Initializing core version: %s", coreVersion)
			d.storeVersion("core", coreVersion)
		} else if coreVersion != storedCoreVersion {
			// Version actually changed
			log.Printf("[deploy] Core version changed: %s -> %s", storedCoreVersion, coreVersion)
			updates.RebuildNode = true
			updates.RebuildCLI = true // CLI depends on some node packages
			updates.RestartNode = true
			d.storeVersion("core", coreVersion)
		}
	}

	// Check services/websocket/VERSION for websocket changes
	wsVersion := d.readVersionFile(filepath.Join(projectRoot, "services", "websocket", "VERSION"))
	storedWSVersion := d.readStoredVersion("websocket")
	if wsVersion != "" {
		if storedWSVersion == "" {
			// First time - initialize
			log.Printf("[deploy] Initializing websocket version: %s", wsVersion)
			d.storeVersion("websocket", wsVersion)
		} else if wsVersion != storedWSVersion {
			log.Printf("[deploy] WebSocket version changed: %s -> %s", storedWSVersion, wsVersion)
			updates.RebuildNode = true
			updates.RestartNode = true
			d.storeVersion("websocket", wsVersion)
		}
	}

	// === HOT layer: cli and ui ===
	// These do NOT require node restart (VPN stays connected)

	// Check services/cli/VERSION for CLI changes
	cliVersion := d.readVersionFile(filepath.Join(projectRoot, "services", "cli", "VERSION"))
	storedCLIVersion := d.readStoredVersion("cli")
	if cliVersion != "" {
		if storedCLIVersion == "" {
			// First time - initialize
			log.Printf("[deploy] Initializing CLI version: %s", cliVersion)
			d.storeVersion("cli", cliVersion)
		} else if cliVersion != storedCLIVersion {
			log.Printf("[deploy] CLI version changed: %s -> %s (HOT update, no restart)", storedCLIVersion, cliVersion)
			updates.RebuildCLI = true
			// NO RestartNode - this is a hot update!
			d.storeVersion("cli", cliVersion)
		}
	}

	// Check services/ui/VERSION for UI changes
	uiVersion := d.readVersionFile(filepath.Join(projectRoot, "services", "ui", "VERSION"))
	storedUIVersion := d.readStoredVersion("ui")
	if uiVersion != "" {
		if storedUIVersion == "" {
			// First time - initialize
			log.Printf("[deploy] Initializing UI version: %s", uiVersion)
			d.storeVersion("ui", uiVersion)
		} else if uiVersion != storedUIVersion {
			log.Printf("[deploy] UI version changed: %s -> %s (HOT update, no restart)", storedUIVersion, uiVersion)
			updates.RebuildCLI = true // UI is built into CLI binary
			// NO RestartNode - this is a hot update!
			d.storeVersion("ui", uiVersion)
		}
	}

	// Log summary
	if !updates.RebuildNode && !updates.RebuildCLI {
		log.Printf("[deploy] No VERSION file changes detected")
	} else {
		log.Printf("[deploy] Update summary: RebuildNode=%v, RebuildCLI=%v, RestartNode=%v",
			updates.RebuildNode, updates.RebuildCLI, updates.RestartNode)
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

// rebuildBinariesSelective rebuilds only the binaries that changed.
func (d *Daemon) rebuildBinariesSelective(updates VersionUpdates) error {
	projectRoot := d.findProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("could not find project root")
	}

	// Find Go binary
	goBin := d.findGoBinary()
	if goBin == "" {
		return fmt.Errorf("could not find go binary")
	}
	log.Printf("[deploy] Using Go binary: %s", goBin)

	var binariesToSign []string

	// Read version from VERSION file for ldflags
	version := d.readVersionFile(filepath.Join(projectRoot, "services", "core", "VERSION"))
	if version == "" {
		version = "dev"
	}
	ldflags := fmt.Sprintf("-X github.com/miguelemosreverte/vpn/internal/node.Version=%s", version)
	log.Printf("[deploy] Building with version: %s", version)

	// Build vpn-node ONLY if node needs rebuild (core/websocket changed)
	if updates.RebuildNode {
		log.Printf("[deploy] Rebuilding vpn-node (COLD update)...")
		cmd := exec.Command(goBin, "build", "-ldflags", ldflags, "-o", "bin/vpn-node", "./cmd/vpn-node")
		cmd.Dir = projectRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build vpn-node: %w: %s", err, output)
		}
		binariesToSign = append(binariesToSign, "bin/vpn-node")

		// On Linux servers, copy to /usr/local/bin for systemd service
		if !d.isMacOS() {
			srcPath := filepath.Join(projectRoot, "bin", "vpn-node")
			dstPath := "/usr/local/bin/vpn-node"
			log.Printf("[deploy] Copying vpn-node to %s", dstPath)
			cpCmd := exec.Command("cp", srcPath, dstPath)
			if output, err := cpCmd.CombinedOutput(); err != nil {
				log.Printf("[deploy] Warning: failed to copy to /usr/local/bin: %v: %s", err, output)
			}
		}
	}

	// Build vpn CLI if CLI needs rebuild (cli/ui changed, or core changed)
	if updates.RebuildCLI {
		log.Printf("[deploy] Rebuilding vpn CLI (HOT update)...")
		cmd := exec.Command(goBin, "build", "-ldflags", ldflags, "-o", "bin/vpn", "./cmd/vpn")
		cmd.Dir = projectRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build vpn: %w: %s", err, output)
		}
		binariesToSign = append(binariesToSign, "bin/vpn")
	}

	// Sign rebuilt binaries on macOS
	if d.isMacOS() && len(binariesToSign) > 0 {
		log.Printf("[deploy] Signing binaries (macOS)...")
		for _, bin := range binariesToSign {
			cmd := exec.Command("codesign", "--sign", "-", "--force", bin)
			cmd.Dir = projectRoot
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("[deploy] Warning: failed to sign %s: %v: %s", bin, err, output)
			}
		}
	}

	log.Printf("[deploy] Selective rebuild complete")
	return nil
}

// findGoBinary finds the Go binary in common locations.
func (d *Daemon) findGoBinary() string {
	// Common Go locations
	locations := []string{
		"/usr/local/go/bin/go",      // macOS default
		"/usr/local/bin/go",         // Homebrew
		"/opt/homebrew/bin/go",      // Apple Silicon Homebrew
		"/usr/bin/go",               // Linux system
		"/root/go/bin/go",           // Go installed in root home
	}

	// Try PATH first
	if path, err := exec.LookPath("go"); err == nil {
		return path
	}

	// Check common locations
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

// isMacOS returns true if running on macOS.
func (d *Daemon) isMacOS() bool {
	return runtime.GOOS == "darwin"
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

// scheduleRestart performs a graceful restart of the node by exec'ing the new binary.
// This replaces the current process with the newly built binary while preserving
// command-line arguments and environment.
func (d *Daemon) scheduleRestart() {
	log.Printf("[deploy] Preparing to restart node with new binary...")

	// Get the path to the currently running executable
	executable, err := os.Executable()
	if err != nil {
		log.Printf("[deploy] ERROR: Cannot determine executable path: %v", err)
		return
	}

	// Resolve any symlinks to get the real path
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		log.Printf("[deploy] ERROR: Cannot resolve executable path: %v", err)
		return
	}

	log.Printf("[deploy] Restarting: %s %v", executable, os.Args[1:])

	// Perform graceful shutdown first
	d.shutdown()

	// Small delay to ensure cleanup completes
	time.Sleep(500 * time.Millisecond)

	// Exec the new binary - this replaces the current process
	// The new process inherits our PID, so any process manager watching us
	// won't see a change
	err = syscall.Exec(executable, os.Args, os.Environ())
	if err != nil {
		// If exec fails, we're in a bad state - the old binary is still running
		// but we've already called Shutdown
		log.Printf("[deploy] CRITICAL: Failed to exec new binary: %v", err)
		log.Printf("[deploy] Process will exit - service manager should restart us")
		os.Exit(1)
	}
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
