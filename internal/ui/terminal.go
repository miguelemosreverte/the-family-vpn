package ui

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin (localhost)
	},
}

// TerminalRequest is sent by the frontend to start an SSH session.
type TerminalRequest struct {
	Host     string `json:"host"`     // VPN IP address
	User     string `json:"user"`     // SSH username
	Password string `json:"password"` // SSH password
}

// TerminalResize is sent by the frontend to resize the terminal.
type TerminalResize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// handleTerminal handles WebSocket connections for SSH terminal sessions.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Read the initial connection request
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Error reading terminal request: %v", err)
		return
	}

	var req TerminalRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: invalid request: %v\r\n", err)))
		return
	}

	if req.Host == "" || req.User == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: host and user are required\r\n"))
		return
	}

	// Start SSH session
	s.startSSHSession(conn, req)
}

// startSSHSession starts an SSH session and proxies I/O to the WebSocket.
func (s *Server) startSSHSession(conn *websocket.Conn, req TerminalRequest) {
	// Build SSH command with sshpass for password auth
	var cmd *exec.Cmd
	if req.Password != "" {
		cmd = exec.Command("sshpass", "-p", req.Password, "ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ServerAliveInterval=30",
			fmt.Sprintf("%s@%s", req.User, req.Host))
	} else {
		// Try without password (key-based auth)
		cmd = exec.Command("ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ServerAliveInterval=30",
			fmt.Sprintf("%s@%s", req.User, req.Host))
	}

	// Set environment
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Start the command with a pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error starting SSH: %v\r\n", err)))
		return
	}
	defer func() {
		ptmx.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Set initial size
	setWinsize(ptmx, 80, 24)

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Read from PTY -> WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
				n, err := ptmx.Read(buf)
				if err != nil {
					return
				}
				if n > 0 {
					if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
						return
					}
				}
			}
		}
	}()

	// Read from WebSocket -> PTY
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			switch msgType {
			case websocket.TextMessage:
				// Check for resize message
				var resize TerminalResize
				if err := json.Unmarshal(msg, &resize); err == nil && resize.Cols > 0 && resize.Rows > 0 {
					setWinsize(ptmx, resize.Cols, resize.Rows)
					continue
				}
				// Regular text input
				if _, err := ptmx.Write(msg); err != nil {
					return
				}
			case websocket.BinaryMessage:
				if _, err := ptmx.Write(msg); err != nil {
					return
				}
			}
		}
	}()

	// Wait for process to exit
	cmd.Wait()
	close(done)
	wg.Wait()

	conn.WriteMessage(websocket.TextMessage, []byte("\r\n[Connection closed]\r\n"))
}

// setWinsize sets the terminal window size.
func setWinsize(f *os.File, cols, rows int) {
	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
}
