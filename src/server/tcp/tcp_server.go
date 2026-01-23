package tcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"control-mate-utils/src/server/localio"
)

// TCPServer manages TCP connections for local IO card automation
type TCPServer struct {
	listener   net.Listener
	clientConn *ClientConnection
	mu         sync.RWMutex
	localioMgr *localio.Manager
	stopChan   chan struct{}
	port       string
	version    string
	localOnly  bool // If true, only accept connections from localhost
}

// ClientConnection represents a connected TCP client
type ClientConnection struct {
	conn     net.Conn
	writer   *bufio.Writer
	encoder  *json.Encoder
	lastSent map[string]*localio.CardState // Track last sent state for change detection
	mu       sync.Mutex
}

// CardUpdateMessage is sent to TCP clients
type CardUpdateMessage struct {
	Type  string          `json:"type"`
	Cards []*localio.Card `json:"cards"`
}

// WelcomeMessage is sent to clients when they connect
type WelcomeMessage struct {
	Type        string `json:"type"`
	Server      string `json:"server"`
	Version     string `json:"version,omitempty"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
}

// WriteCommand is received from TCP clients
type WriteCommand struct {
	Type   string  `json:"type"`
	CardID string  `json:"cardId"`
	Index  int     `json:"index"`
	State  bool    `json:"state,omitempty"`
	Value  float32 `json:"value,omitempty"`
	Mode   string  `json:"mode,omitempty"`
}

// WriteResponse is sent back to TCP clients
type WriteResponse struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NewTCPServer creates a new TCP server instance
func NewTCPServer(port string, localioMgr *localio.Manager, version string, serveExternally bool) *TCPServer {
	return &TCPServer{
		localioMgr: localioMgr,
		stopChan:   make(chan struct{}),
		port:       port,
		version:    version,
		localOnly:  !serveExternally,
	}
}

// Start starts the TCP server
func (s *TCPServer) Start() error {
	var addr string
	if s.localOnly {
		addr = "127.0.0.1:" + s.port
	} else {
		addr = "0.0.0.0:" + s.port
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server on %s: %v", addr, err)
	}

	s.listener = listener
	if s.localOnly {
		log.Printf("TCP server listening on %s (localhost only)", addr)
	} else {
		log.Printf("TCP server listening on %s (all interfaces)", addr)
	}

	// Register callback for immediate updates on DI/AI changes
	s.localioMgr.SetStateChangeCallback(s.onStateChange)

	go s.acceptLoop()
	go s.updateLoop()

	return nil
}

// onStateChange is called immediately when DI or AI values change
func (s *TCPServer) onStateChange(cards []*localio.Card) {
	s.mu.RLock()
	clientConn := s.clientConn
	s.mu.RUnlock()

	if clientConn != nil && len(cards) > 0 {
		s.sendUpdate(clientConn, cards)
	}
}

// Stop stops the TCP server
func (s *TCPServer) Stop() {
	close(s.stopChan)
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Lock()
	if s.clientConn != nil {
		s.clientConn.conn.Close()
		s.clientConn = nil
	}
	s.mu.Unlock()
}

// IsConnected returns whether a TCP client is currently connected
func (s *TCPServer) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientConn != nil
}

// acceptLoop accepts incoming connections
func (s *TCPServer) acceptLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.stopChan:
					return
				default:
					log.Printf("TCP accept error: %v", err)
					continue
				}
			}

			// Verify client is from localhost if localOnly is enabled
			remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
			if s.localOnly {
				if !remoteAddr.IP.IsLoopback() && remoteAddr.IP.String() != "127.0.0.1" {
					log.Printf("TCP connection rejected: non-localhost IP %s", remoteAddr.IP.String())
					conn.Close()
					continue
				}
			}

			// Check if already have a client
			s.mu.Lock()
			if s.clientConn != nil {
				log.Printf("TCP connection rejected: client already connected")
				conn.Close()
				s.mu.Unlock()
				continue
			}

			// Accept the connection
			clientConn := &ClientConnection{
				conn:     conn,
				writer:   bufio.NewWriter(conn),
				encoder:  json.NewEncoder(conn),
				lastSent: make(map[string]*localio.CardState),
			}
			s.clientConn = clientConn
			s.mu.Unlock()

			log.Printf("TCP client connected from %s", remoteAddr.String())

			// Send welcome message to identify server
			s.sendWelcomeMessage(clientConn)

			// Handle client in separate goroutine
			go s.handleClient(clientConn)
		}
	}
}

// handleClient handles communication with a connected client
func (s *TCPServer) handleClient(clientConn *ClientConnection) {
	defer func() {
		s.mu.Lock()
		if s.clientConn == clientConn {
			s.clientConn = nil
		}
		s.mu.Unlock()
		clientConn.conn.Close()
		log.Printf("TCP client disconnected")
	}()

	scanner := bufio.NewScanner(clientConn.conn)
	for scanner.Scan() {
		var cmd WriteCommand
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			log.Printf("TCP: failed to parse command: %v", err)
			continue
		}

		// Process write command
		s.processWriteCommand(&cmd, clientConn)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("TCP: client read error: %v", err)
	}
}

// processWriteCommand processes a write command from TCP client
func (s *TCPServer) processWriteCommand(cmd *WriteCommand, clientConn *ClientConnection) {
	var err error
	var response WriteResponse

	switch cmd.Type {
	case "write-do":
		// Check if value actually changed
		card, ok := s.localioMgr.GetCard(cmd.CardID)
		if !ok {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: "card not found",
			}
			clientConn.encoder.Encode(response)
			return
		}

		// Check if DO value changed
		if cmd.Index >= 0 && cmd.Index < len(card.Last.DO) {
			currentState := card.Last.DO[cmd.Index]
			if currentState == cmd.State {
				// Value unchanged, skip write
				response = WriteResponse{
					Type:    "write-response",
					Status:  "ok",
					Message: "value unchanged, skipped",
				}
				clientConn.encoder.Encode(response)
				return
			}
		}

		err = s.localioMgr.QueueWriteDO(cmd.CardID, cmd.Index, cmd.State)
		if err == nil {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "ok",
				Message: "write queued",
			}
		} else {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: err.Error(),
			}
		}

	case "write-ao":
		// Check if value actually changed
		card, ok := s.localioMgr.GetCard(cmd.CardID)
		if !ok {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: "card not found",
			}
			clientConn.encoder.Encode(response)
			return
		}

		// Check if AO value changed
		if cmd.Index >= 0 && cmd.Index < len(card.Last.AO) {
			currentValue := card.Last.AO[cmd.Index]
			if currentValue == cmd.Value {
				// Value unchanged, skip write
				response = WriteResponse{
					Type:    "write-response",
					Status:  "ok",
					Message: "value unchanged, skipped",
				}
				clientConn.encoder.Encode(response)
				return
			}
		}

		err = s.localioMgr.QueueWriteAO(cmd.CardID, cmd.Index, cmd.Value)
		if err == nil {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "ok",
				Message: "write queued",
			}
		} else {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: err.Error(),
			}
		}

	case "write-aotype":
		// Check if AO type actually changed
		card, ok := s.localioMgr.GetCard(cmd.CardID)
		if !ok {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: "card not found",
			}
			clientConn.encoder.Encode(response)
			return
		}

		// Check if AO type changed
		if cmd.Index >= 0 && cmd.Index < len(card.Last.AOType) {
			currentMode := card.Last.AOType[cmd.Index]
			if currentMode == cmd.Mode {
				// Value unchanged, skip write
				response = WriteResponse{
					Type:    "write-response",
					Status:  "ok",
					Message: "value unchanged, skipped",
				}
				clientConn.encoder.Encode(response)
				return
			}
		}

		err = s.localioMgr.QueueWriteAOType(cmd.CardID, cmd.Index, cmd.Mode)
		if err == nil {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "ok",
				Message: "write queued",
			}
		} else {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: err.Error(),
			}
		}

	case "reboot":
		err = s.localioMgr.RebootCard(cmd.CardID)
		if err == nil {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "ok",
				Message: "reboot command sent",
			}
		} else {
			response = WriteResponse{
				Type:    "write-response",
				Status:  "error",
				Message: err.Error(),
			}
		}

	default:
		response = WriteResponse{
			Type:    "write-response",
			Status:  "error",
			Message: "unknown command type",
		}
	}

	clientConn.encoder.Encode(response)
}

// updateLoop sends periodic updates (500ms) for all card data
// Immediate updates on DI/AI changes are handled by onStateChange callback
func (s *TCPServer) updateLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.RLock()
			clientConn := s.clientConn
			s.mu.RUnlock()

			if clientConn == nil {
				continue
			}

			// Get current cards and send periodic update
			cards := s.localioMgr.GetAllCards()
			if len(cards) > 0 {
				s.sendUpdate(clientConn, cards)
			}
		}
	}
}

// sendWelcomeMessage sends a welcome/identification message to newly connected client
func (s *TCPServer) sendWelcomeMessage(clientConn *ClientConnection) {
	clientConn.mu.Lock()
	defer clientConn.mu.Unlock()

	msg := WelcomeMessage{
		Type:        "welcome",
		Server:      "ControlMate TCP Server",
		Version:     s.version,
		Protocol:    "JSON",
		Description: "ControlMate Extension cards TCP server - sends card state updates and accepts write commands",
	}

	if err := clientConn.encoder.Encode(msg); err != nil {
		log.Printf("TCP: failed to send welcome message: %v", err)
	}
}

// sendUpdate sends card update to TCP client
func (s *TCPServer) sendUpdate(clientConn *ClientConnection, cards []*localio.Card) {
	clientConn.mu.Lock()
	defer clientConn.mu.Unlock()

	msg := CardUpdateMessage{
		Type:  "card-update",
		Cards: cards,
	}

	if err := clientConn.encoder.Encode(msg); err != nil {
		log.Printf("TCP: failed to send update: %v", err)
		// Connection might be broken, will be cleaned up in handleClient
		return
	}

	// Update last sent state for change tracking
	for _, card := range cards {
		stateCopy := card.Last
		clientConn.lastSent[card.ID] = &stateCopy
	}
}
