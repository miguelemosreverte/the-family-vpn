package tunnel

import (
	"bufio"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Conn represents a VPN tunnel connection to another node.
type Conn struct {
	NetConn    net.Conn // Exported for protocol handshake access
	reader     *bufio.Reader
	writer     *bufio.Writer
	writerMu   sync.Mutex
	cipher     *Cipher
	encryption bool
	remoteAddr string

	// Statistics
	mu          sync.RWMutex
	bytesSent   uint64
	bytesRecv   uint64
	packetsSent uint64
	packetsRecv uint64
}

// DialConfig holds configuration for dialing a VPN connection.
type DialConfig struct {
	Address    string
	UseTLS     bool
	Key        []byte // 32 bytes for AES-256
	Encryption bool
}

// Dial connects to a VPN node.
func Dial(cfg DialConfig) (*Conn, error) {
	var netConn net.Conn
	var err error

	if cfg.UseTLS {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true, // For self-signed certs
		}
		netConn, err = tls.Dial("tcp", cfg.Address, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("TLS dial failed: %w", err)
		}
		log.Printf("[conn] Connected to %s with TLS", cfg.Address)
	} else {
		netConn, err = net.Dial("tcp", cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("TCP dial failed: %w", err)
		}
		log.Printf("[conn] Connected to %s", cfg.Address)
	}

	// Tune TCP socket
	if err := tuneTCPConn(netConn); err != nil {
		log.Printf("[conn] Warning: failed to tune TCP: %v", err)
	}

	conn := &Conn{
		NetConn:    netConn,
		reader:     bufio.NewReaderSize(netConn, 256*1024), // 256KB buffer
		writer:     bufio.NewWriterSize(netConn, 256*1024),
		remoteAddr: cfg.Address,
		encryption: cfg.Encryption,
	}

	if cfg.Encryption && len(cfg.Key) == 32 {
		cipher, err := NewCipher(cfg.Key)
		if err != nil {
			netConn.Close()
			return nil, fmt.Errorf("failed to create cipher: %w", err)
		}
		conn.cipher = cipher
	}

	return conn, nil
}

// tuneTCPConn optimizes a TCP connection for VPN traffic.
func tuneTCPConn(conn net.Conn) error {
	var tcpConn *net.TCPConn

	// Handle TLS connections
	if tlsConn, ok := conn.(*tls.Conn); ok {
		if underlying, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			tcpConn = underlying
		}
	} else if direct, ok := conn.(*net.TCPConn); ok {
		tcpConn = direct
	}

	if tcpConn == nil {
		return nil
	}

	// 1MB buffers for high throughput
	tcpConn.SetReadBuffer(1024 * 1024)
	tcpConn.SetWriteBuffer(1024 * 1024)

	// Disable Nagle's algorithm for low latency
	tcpConn.SetNoDelay(true)

	// Enable keepalive to prevent NAT/firewall timeouts
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(30 * time.Second)

	log.Printf("[conn] TCP tuned: 1MB buffers, NoDelay, Keepalive 30s")
	return nil
}

// WritePacket sends an encrypted packet.
// Wire format: [4-byte length][encrypted payload]
func (c *Conn) WritePacket(data []byte) error {
	var toSend []byte
	var err error

	if c.encryption && c.cipher != nil {
		toSend, err = c.cipher.Encrypt(data)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
	} else {
		toSend = data
	}

	// Length prefix (4 bytes, big endian)
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(toSend)))

	c.writerMu.Lock()
	defer c.writerMu.Unlock()

	if _, err := c.writer.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write length: %w", err)
	}

	if _, err := c.writer.Write(toSend); err != nil {
		return fmt.Errorf("failed to write packet: %w", err)
	}

	// Always flush immediately - VPN packets need low latency
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	c.mu.Lock()
	c.bytesSent += uint64(len(toSend) + 4)
	c.packetsSent++
	c.mu.Unlock()

	return nil
}

// Flush forces a write of buffered data.
func (c *Conn) Flush() error {
	c.writerMu.Lock()
	defer c.writerMu.Unlock()
	return c.writer.Flush()
}

// ReadPacket reads and decrypts a packet.
// Returns the decrypted payload.
func (c *Conn) ReadPacket() ([]byte, error) {
	// Read length prefix
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.reader, lengthBuf); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	// Sanity check
	if length > MTU*2 || length == 0 {
		return nil, fmt.Errorf("invalid packet length: %d", length)
	}

	// Read packet
	packet := make([]byte, length)
	if _, err := io.ReadFull(c.reader, packet); err != nil {
		return nil, fmt.Errorf("failed to read packet: %w", err)
	}

	c.mu.Lock()
	c.bytesRecv += uint64(length + 4)
	c.packetsRecv++
	c.mu.Unlock()

	// Decrypt if needed
	if c.encryption && c.cipher != nil {
		decrypted, err := c.cipher.Decrypt(packet)
		if err != nil {
			return nil, fmt.Errorf("decryption failed: %w", err)
		}
		return decrypted, nil
	}

	return packet, nil
}

// Stats returns connection statistics.
func (c *Conn) Stats() (bytesSent, bytesRecv, packetsSent, packetsRecv uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bytesSent, c.bytesRecv, c.packetsSent, c.packetsRecv
}

// RemoteAddr returns the remote address.
func (c *Conn) RemoteAddr() string {
	return c.remoteAddr
}

// Close closes the connection.
func (c *Conn) Close() error {
	c.writerMu.Lock()
	c.writer.Flush()
	c.writerMu.Unlock()
	return c.NetConn.Close()
}

// Listener accepts incoming VPN connections.
type Listener struct {
	listener   net.Listener
	tlsConfig  *tls.Config
	key        []byte
	encryption bool
}

// ListenConfig holds configuration for listening.
type ListenConfig struct {
	Address    string
	UseTLS     bool
	CertFile   string
	KeyFile    string
	Key        []byte // Encryption key
	Encryption bool
}

// Listen creates a VPN listener.
func Listen(cfg ListenConfig) (*Listener, error) {
	var listener net.Listener
	var tlsConfig *tls.Config
	var err error

	if cfg.UseTLS {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS cert: %w", err)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		listener, err = tls.Listen("tcp", cfg.Address, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("TLS listen failed: %w", err)
		}
		log.Printf("[conn] Listening on %s with TLS", cfg.Address)
	} else {
		listener, err = net.Listen("tcp", cfg.Address)
		if err != nil {
			return nil, fmt.Errorf("TCP listen failed: %w", err)
		}
		log.Printf("[conn] Listening on %s", cfg.Address)
	}

	return &Listener{
		listener:   listener,
		tlsConfig:  tlsConfig,
		key:        cfg.Key,
		encryption: cfg.Encryption,
	}, nil
}

// Accept accepts a new VPN connection.
func (l *Listener) Accept() (*Conn, error) {
	netConn, err := l.listener.Accept()
	if err != nil {
		return nil, err
	}

	if err := tuneTCPConn(netConn); err != nil {
		log.Printf("[conn] Warning: failed to tune TCP: %v", err)
	}

	conn := &Conn{
		NetConn:    netConn,
		reader:     bufio.NewReaderSize(netConn, 256*1024),
		writer:     bufio.NewWriterSize(netConn, 256*1024),
		remoteAddr: netConn.RemoteAddr().String(),
		encryption: l.encryption,
	}

	if l.encryption && len(l.key) == 32 {
		cipher, err := NewCipher(l.key)
		if err != nil {
			netConn.Close()
			return nil, fmt.Errorf("failed to create cipher: %w", err)
		}
		conn.cipher = cipher
	}

	log.Printf("[conn] Accepted connection from %s", conn.remoteAddr)
	return conn, nil
}

// Close closes the listener.
func (l *Listener) Close() error {
	return l.listener.Close()
}

// Addr returns the listener's address.
func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}
