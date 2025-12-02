// Package protocol defines wire protocols for VPN communication.
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// Note: PeerInfo is defined in control.go

// Handshake is the initial exchange when connecting to a node.
// Client sends: [1 byte: encryption flag][4 bytes: peer info length][peer info JSON]
// Server responds: [4 bytes: assigned IP length][assigned IP string]

// WriteHandshake sends the client handshake.
func WriteHandshake(w io.Writer, encryption bool, info PeerInfo) error {
	// Encryption flag
	encByte := byte(0)
	if encryption {
		encByte = 1
	}
	if _, err := w.Write([]byte{encByte}); err != nil {
		return fmt.Errorf("failed to write encryption flag: %w", err)
	}

	// Peer info
	infoJSON, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal peer info: %w", err)
	}

	// Length + data
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(infoJSON)))
	if _, err := w.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write peer info length: %w", err)
	}
	if _, err := w.Write(infoJSON); err != nil {
		return fmt.Errorf("failed to write peer info: %w", err)
	}

	return nil
}

// ReadHandshake reads the client handshake.
func ReadHandshake(r io.Reader) (encryption bool, info PeerInfo, err error) {
	// Encryption flag
	encByte := make([]byte, 1)
	if _, err := io.ReadFull(r, encByte); err != nil {
		return false, PeerInfo{}, fmt.Errorf("failed to read encryption flag: %w", err)
	}
	encryption = encByte[0] == 1

	// Peer info length
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return false, PeerInfo{}, fmt.Errorf("failed to read peer info length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	if length > 4096 { // Sanity check
		return false, PeerInfo{}, fmt.Errorf("peer info too large: %d", length)
	}

	// Peer info
	infoBuf := make([]byte, length)
	if _, err := io.ReadFull(r, infoBuf); err != nil {
		return false, PeerInfo{}, fmt.Errorf("failed to read peer info: %w", err)
	}

	if err := json.Unmarshal(infoBuf, &info); err != nil {
		return false, PeerInfo{}, fmt.Errorf("failed to parse peer info: %w", err)
	}

	return encryption, info, nil
}

// WriteAssignedIP sends the assigned VPN IP to the client.
func WriteAssignedIP(w io.Writer, vpnIP string) error {
	ipBytes := []byte(vpnIP)
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(ipBytes)))

	if _, err := w.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write IP length: %w", err)
	}
	if _, err := w.Write(ipBytes); err != nil {
		return fmt.Errorf("failed to write IP: %w", err)
	}

	return nil
}

// ReadAssignedIP reads the assigned VPN IP from the server.
func ReadAssignedIP(r io.Reader) (string, error) {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return "", fmt.Errorf("failed to read IP length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	if length > 64 { // Sanity check
		return "", fmt.Errorf("IP too long: %d", length)
	}

	ipBuf := make([]byte, length)
	if _, err := io.ReadFull(r, ipBuf); err != nil {
		return "", fmt.Errorf("failed to read IP: %w", err)
	}

	return string(ipBuf), nil
}

// ControlMessage is a message sent over the VPN tunnel for signaling.
// Format: "CTRL:" prefix followed by the command.
const ControlPrefix = "CTRL:"

// IsControlMessage checks if a packet is a control message.
func IsControlMessage(data []byte) bool {
	if len(data) < len(ControlPrefix) {
		return false
	}
	return string(data[:len(ControlPrefix)]) == ControlPrefix
}

// ExtractControlCommand extracts the command from a control message.
func ExtractControlCommand(data []byte) string {
	if !IsControlMessage(data) {
		return ""
	}
	return string(data[len(ControlPrefix):])
}

// MakeControlMessage creates a control message.
func MakeControlMessage(command string) []byte {
	return append([]byte(ControlPrefix), []byte(command)...)
}

// Control message types
const (
	// Peer list: "PEER_LIST:" + JSON array of peers
	CmdPeerList = "PEER_LIST:"

	// Update signal: "UPDATE_AVAILABLE"
	CmdUpdateAvailable = "UPDATE_AVAILABLE"
)

// GeoLocation represents geographical coordinates and location info.
type GeoLocation struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	City      string  `json:"city,omitempty"`
	Country   string  `json:"country,omitempty"`
	ISP       string  `json:"isp,omitempty"`
}

// PeerListEntry is a peer in the PEER_LIST message.
type PeerListEntry struct {
	Name       string       `json:"name"`
	VPNAddress string       `json:"vpn_address"`
	Hostname   string       `json:"hostname"`
	OS         string       `json:"os"`
	PublicIP   string       `json:"public_ip,omitempty"`
	Geo        *GeoLocation `json:"geo,omitempty"`
}

// MakePeerListMessage creates a PEER_LIST control message.
func MakePeerListMessage(peers []PeerListEntry) []byte {
	data, _ := json.Marshal(peers)
	return MakeControlMessage(CmdPeerList + string(data))
}

// ParsePeerListMessage extracts peers from a PEER_LIST control message.
func ParsePeerListMessage(data []byte) ([]PeerListEntry, error) {
	cmd := ExtractControlCommand(data)
	if !IsPeerListMessage(cmd) {
		return nil, fmt.Errorf("not a peer list message")
	}

	jsonData := cmd[len(CmdPeerList):]
	var peers []PeerListEntry
	if err := json.Unmarshal([]byte(jsonData), &peers); err != nil {
		return nil, fmt.Errorf("failed to parse peer list: %w", err)
	}
	return peers, nil
}

// IsPeerListMessage checks if a command is a PEER_LIST message.
func IsPeerListMessage(cmd string) bool {
	return len(cmd) >= len(CmdPeerList) && cmd[:len(CmdPeerList)] == CmdPeerList
}
