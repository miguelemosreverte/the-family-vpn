// Package tunnel handles TUN device creation and IP packet processing.
package tunnel

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/songgao/water"
)

const (
	// MTU is the maximum transmission unit for the TUN device.
	// Reduced from 1500 to account for encryption overhead (GCM adds ~28 bytes).
	MTU = 1400

	// DefaultServerIP is the VPN gateway IP address.
	DefaultServerIP = "10.8.0.1"

	// DefaultSubnet is the VPN subnet.
	DefaultSubnet = "10.8.0.0/24"
)

// TUN represents a TUN device for VPN traffic.
type TUN struct {
	iface          *water.Interface
	name           string
	localIP        string
	gatewayIP      string
	originalGW     string // Original default gateway before VPN
	serverPublicIP string // Server's public IP (for route cleanup)
	ipv6WasEnabled bool   // Track if IPv6 was enabled before VPN connected
}

// Config holds TUN device configuration.
type Config struct {
	// LocalIP is the IP address assigned to this node's TUN interface.
	LocalIP string

	// GatewayIP is the VPN gateway (usually the server's VPN IP).
	GatewayIP string

	// DeviceName is the desired TUN device name (Linux only).
	DeviceName string
}

// New creates a new TUN device.
func New(cfg Config) (*TUN, error) {
	waterCfg := water.Config{
		DeviceType: water.TUN,
	}

	// On Linux, we can specify the device name
	if runtime.GOOS == "linux" && cfg.DeviceName != "" {
		waterCfg.Name = cfg.DeviceName
	}

	iface, err := water.New(waterCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %w", err)
	}

	tun := &TUN{
		iface:     iface,
		name:      iface.Name(),
		localIP:   cfg.LocalIP,
		gatewayIP: cfg.GatewayIP,
	}

	log.Printf("[tun] Created TUN device: %s", tun.name)

	if err := tun.configure(); err != nil {
		iface.Close()
		return nil, err
	}

	return tun, nil
}

// configure sets up IP address and MTU on the TUN device.
func (t *TUN) configure() error {
	if runtime.GOOS == "darwin" {
		return t.configureDarwin()
	}
	return t.configureLinux()
}

// configureDarwin configures the TUN device on macOS.
func (t *TUN) configureDarwin() error {
	// macOS uses ifconfig with point-to-point syntax
	cmd := exec.Command("ifconfig", t.name, t.localIP, t.gatewayIP, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to configure %s: %v - %s", t.name, err, out)
	}

	// Set MTU
	cmd = exec.Command("ifconfig", t.name, "mtu", fmt.Sprintf("%d", MTU))
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to set MTU: %v", err)
	}

	// Add route for VPN subnet
	cmd = exec.Command("route", "-n", "add", "-net", DefaultSubnet, "-interface", t.name)
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to add subnet route: %v", err)
	}

	log.Printf("[tun] Configured %s: %s -> %s (MTU=%d)", t.name, t.localIP, t.gatewayIP, MTU)
	return nil
}

// configureLinux configures the TUN device on Linux.
func (t *TUN) configureLinux() error {
	// Flush existing IPs
	exec.Command("ip", "addr", "flush", "dev", t.name).Run()

	// Assign IP address
	cmd := exec.Command("ip", "addr", "add", t.localIP+"/24", "dev", t.name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to assign IP: %v - %s", err, out)
	}

	// Set MTU
	cmd = exec.Command("ip", "link", "set", "dev", t.name, "mtu", fmt.Sprintf("%d", MTU))
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to set MTU: %v", err)
	}

	// Increase TX queue length for high throughput
	cmd = exec.Command("ip", "link", "set", "dev", t.name, "txqueuelen", "10000")
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to set txqueuelen: %v", err)
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", "dev", t.name, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring interface up: %v", err)
	}

	log.Printf("[tun] Configured %s: %s/24 (MTU=%d)", t.name, t.localIP, MTU)
	return nil
}

// Name returns the TUN device name.
func (t *TUN) Name() string {
	return t.name
}

// Read reads a packet from the TUN device.
func (t *TUN) Read(buf []byte) (int, error) {
	return t.iface.Read(buf)
}

// Write writes a packet to the TUN device.
func (t *TUN) Write(buf []byte) (int, error) {
	// Validate IP packet before writing
	if !IsValidIPPacket(buf) {
		return 0, fmt.Errorf("invalid IP packet (len=%d)", len(buf))
	}
	return t.iface.Write(buf)
}

// Close closes the TUN device.
func (t *TUN) Close() error {
	if t.iface != nil {
		log.Printf("[tun] Closing TUN device: %s", t.name)
		return t.iface.Close()
	}
	return nil
}

// Reconfigure updates the TUN device with a new local IP.
// This is used when reconnecting and the server assigns a different IP.
func (t *TUN) Reconfigure(newLocalIP string) error {
	if newLocalIP == t.localIP {
		log.Printf("[tun] IP unchanged (%s), no reconfiguration needed", newLocalIP)
		return nil
	}

	log.Printf("[tun] Reconfiguring %s: %s -> %s", t.name, t.localIP, newLocalIP)
	t.localIP = newLocalIP

	if runtime.GOOS == "darwin" {
		return t.reconfigureDarwin()
	}
	return t.reconfigureLinux()
}

// reconfigureDarwin reconfigures the TUN device on macOS.
func (t *TUN) reconfigureDarwin() error {
	// On macOS, we need to update the point-to-point addresses
	// First delete the old address configuration
	cmd := exec.Command("ifconfig", t.name, "delete", t.localIP)
	cmd.Run() // Ignore error, might fail if old IP already removed

	// Reconfigure with new IP
	cmd = exec.Command("ifconfig", t.name, t.localIP, t.gatewayIP, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reconfigure %s: %v - %s", t.name, err, out)
	}

	// Re-add subnet route (might be lost after reconfig)
	cmd = exec.Command("route", "-n", "add", "-net", DefaultSubnet, "-interface", t.name)
	cmd.Run() // Ignore error if route exists

	log.Printf("[tun] Reconfigured %s: %s -> %s", t.name, t.localIP, t.gatewayIP)
	return nil
}

// reconfigureLinux reconfigures the TUN device on Linux.
func (t *TUN) reconfigureLinux() error {
	// Flush existing IPs
	exec.Command("ip", "addr", "flush", "dev", t.name).Run()

	// Assign new IP address
	cmd := exec.Command("ip", "addr", "add", t.localIP+"/24", "dev", t.name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to assign IP: %v - %s", err, out)
	}

	log.Printf("[tun] Reconfigured %s: %s/24", t.name, t.localIP)
	return nil
}

// LocalIP returns the current local IP of the TUN device.
func (t *TUN) LocalIP() string {
	return t.localIP
}

// GetDefaultGateway returns the current default gateway.
func GetDefaultGateway() (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("sh", "-c", "route -n get default | grep gateway | awk '{print $2}'")
	} else {
		cmd = exec.Command("sh", "-c", "ip route | grep default | awk '{print $3}'")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	result := string(output)
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}
	return result, nil
}

// RouteAllTraffic routes all traffic through the VPN.
func (t *TUN) RouteAllTraffic(serverPublicIP string) error {
	// Save original gateway
	gw, err := GetDefaultGateway()
	if err != nil {
		return fmt.Errorf("failed to get default gateway: %w", err)
	}
	t.originalGW = gw
	log.Printf("[tun] Original gateway: %s", t.originalGW)

	if runtime.GOOS == "darwin" {
		return t.routeAllTrafficDarwin(serverPublicIP)
	}
	return t.routeAllTrafficLinux(serverPublicIP)
}

func (t *TUN) routeAllTrafficDarwin(serverPublicIP string) error {
	// Save server IP for cleanup later
	t.serverPublicIP = serverPublicIP

	// Route VPN server through original gateway (prevent routing loop)
	cmd := exec.Command("route", "-n", "add", "-host", serverPublicIP, t.originalGW)
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to add server route: %v", err)
	}

	// Delete default route
	cmd = exec.Command("route", "-n", "delete", "default")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete default route: %v", err)
	}

	// Add default route through VPN gateway
	cmd = exec.Command("route", "-n", "add", "-net", "default", t.gatewayIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add VPN route: %v", err)
	}

	// Configure DNS to use fast public resolvers through VPN
	// This prevents DNS leaks and improves privacy
	cmd = exec.Command("networksetup", "-setdnsservers", "Wi-Fi", "1.1.1.1", "8.8.8.8")
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to set DNS servers: %v (DNS may leak)", err)
	} else {
		log.Printf("[tun] DNS configured: 1.1.1.1 (Cloudflare), 8.8.8.8 (Google) through VPN")
	}

	// Prevent IPv6 leaks by disabling IPv6 on Wi-Fi
	// First, check if IPv6 is currently enabled
	cmd = exec.Command("networksetup", "-getinfo", "Wi-Fi")
	output, err := cmd.Output()
	if err == nil {
		outputStr := string(output)
		// Check if IPv6 is set to "Automatic" or "Manual" (enabled states)
		t.ipv6WasEnabled = strings.Contains(outputStr, "IPv6: Automatic") ||
			strings.Contains(outputStr, "IPv6: Manual")
	}

	// Disable IPv6 to prevent leaks
	cmd = exec.Command("networksetup", "-setv6off", "Wi-Fi")
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to disable IPv6: %v (IPv6 may leak)", err)
	} else {
		log.Printf("[tun] IPv6 disabled to prevent location leaks")
	}

	log.Printf("[tun] All traffic now routed through VPN")
	return nil
}

func (t *TUN) routeAllTrafficLinux(serverPublicIP string) error {
	// Save server IP for cleanup later
	t.serverPublicIP = serverPublicIP

	// Route VPN server through original gateway
	cmd := exec.Command("ip", "route", "add", serverPublicIP, "via", t.originalGW)
	if err := cmd.Run(); err != nil {
		log.Printf("[tun] Warning: failed to add server route: %v", err)
	}

	// Delete default route
	cmd = exec.Command("ip", "route", "del", "default")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete default route: %v", err)
	}

	// Add default route through VPN
	cmd = exec.Command("ip", "route", "add", "default", "via", t.gatewayIP, "dev", t.name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add VPN route: %v", err)
	}

	log.Printf("[tun] All traffic now routed through VPN")
	return nil
}

// RestoreRouting restores the original routing table.
func (t *TUN) RestoreRouting() error {
	if t.originalGW == "" {
		return nil
	}

	if runtime.GOOS == "darwin" {
		// Delete the server-specific route that was added to prevent routing loops
		if t.serverPublicIP != "" {
			exec.Command("route", "-n", "delete", "-host", t.serverPublicIP).Run()
			log.Printf("[tun] Deleted server route: %s", t.serverPublicIP)
		}

		exec.Command("route", "-n", "delete", "default").Run()
		cmd := exec.Command("route", "-n", "add", "-net", "default", t.originalGW)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restore default route: %v", err)
		}

		// Restore DNS to DHCP (automatic)
		cmd = exec.Command("networksetup", "-setdnsservers", "Wi-Fi", "Empty")
		if err := cmd.Run(); err != nil {
			log.Printf("[tun] Warning: failed to restore DNS: %v", err)
		} else {
			log.Printf("[tun] DNS restored to automatic (DHCP)")
		}

		// Restore IPv6 if it was enabled before VPN connected
		if t.ipv6WasEnabled {
			cmd = exec.Command("networksetup", "-setv6automatic", "Wi-Fi")
			if err := cmd.Run(); err != nil {
				log.Printf("[tun] Warning: failed to restore IPv6: %v", err)
			} else {
				log.Printf("[tun] IPv6 restored to automatic")
			}
		}
	} else {
		// Delete the server-specific route that was added to prevent routing loops
		if t.serverPublicIP != "" {
			exec.Command("ip", "route", "del", t.serverPublicIP).Run()
			log.Printf("[tun] Deleted server route: %s", t.serverPublicIP)
		}

		exec.Command("ip", "route", "del", "default", "dev", t.name).Run()
		cmd := exec.Command("ip", "route", "add", "default", "via", t.originalGW)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restore default route: %v", err)
		}
	}

	log.Printf("[tun] Routing restored to original gateway: %s", t.originalGW)
	return nil
}

// IsValidIPPacket checks if data is a valid IPv4 or IPv6 packet.
func IsValidIPPacket(data []byte) bool {
	if len(data) < 1 {
		return false
	}
	version := data[0] >> 4
	return version == 4 || version == 6
}

// GetDestinationIP extracts the destination IP from an IP packet.
func GetDestinationIP(packet []byte) net.IP {
	if len(packet) < 20 {
		return nil
	}
	// IPv4 destination is at bytes 16-19
	return net.IPv4(packet[16], packet[17], packet[18], packet[19])
}

// GetSourceIP extracts the source IP from an IP packet.
func GetSourceIP(packet []byte) net.IP {
	if len(packet) < 16 {
		return nil
	}
	// IPv4 source is at bytes 12-15
	return net.IPv4(packet[12], packet[13], packet[14], packet[15])
}
