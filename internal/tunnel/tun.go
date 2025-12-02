// Package tunnel handles TUN device creation and IP packet processing.
package tunnel

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"runtime"

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
	iface      *water.Interface
	name       string
	localIP    string
	gatewayIP  string
	originalGW string // Original default gateway before VPN
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

	log.Printf("[tun] All traffic now routed through VPN")
	return nil
}

func (t *TUN) routeAllTrafficLinux(serverPublicIP string) error {
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
		exec.Command("route", "-n", "delete", "default").Run()
		cmd := exec.Command("route", "-n", "add", "-net", "default", t.originalGW)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to restore default route: %v", err)
		}
	} else {
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
