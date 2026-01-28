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

// WriteCommandItem represents a single command in the commands array
type WriteCommandItem struct {
	Type   string  `json:"type"` // "write-do", "write-ao", "write-aotype", "reboot"
	CardID string  `json:"cardId"`
	Index  int     `json:"index"`
	State  bool    `json:"state,omitempty"`
	Value  float32 `json:"value,omitempty"`
	Mode   string  `json:"mode,omitempty"`
}

// WriteCommand is received from TCP clients - always contains an array of commands
type WriteCommand struct {
	Type     string             `json:"type"`     // Always "write"
	Commands []WriteCommandItem `json:"commands"` // Array of individual commands
}

// WriteResponse is sent back to TCP clients
type WriteResponse struct {
	Type        string                  `json:"type"`                  // "write-response"
	Status      string                  `json:"status"`                // "ok" or "error"
	Results     []localio.CommandResult `json:"results,omitempty"`     // Results for each command
	Message     string                  `json:"message,omitempty"`     // Error message if status is "error"
	FailedIndex int                     `json:"failedIndex,omitempty"` // Index of failed command
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

		// Process write command (always expects array of commands)
		if cmd.Type != "write" {
			log.Printf("TCP: unknown message type: %s", cmd.Type)
			continue
		}

		s.processWriteCommand(&cmd, clientConn)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("TCP: client read error: %v", err)
	}
}

// processWriteCommand processes a write command from TCP client (always expects array of commands)
func (s *TCPServer) processWriteCommand(cmd *WriteCommand, clientConn *ClientConnection) {
	if len(cmd.Commands) == 0 {
		response := WriteResponse{
			Type:    "write-response",
			Status:  "error",
			Message: "no commands in batch",
		}
		clientConn.encoder.Encode(response)
		return
	}

	// Separate write operations from reboot commands
	ops := make([]localio.WriteOperation, 0, len(cmd.Commands))
	rebootIndices := make([]int, 0) // Track indices of reboot commands

	for i, cmdItem := range cmd.Commands {
		if cmdItem.Type == "reboot" {
			rebootIndices = append(rebootIndices, i)
			continue
		}

		op := localio.WriteOperation{
			CardID: cmdItem.CardID,
			Index:  cmdItem.Index,
		}

		switch cmdItem.Type {
		case "write-do":
			op.Type = localio.WriteOpDO
			if cmdItem.State {
				op.Value = 1.0
			}
		case "write-ao":
			op.Type = localio.WriteOpAO
			op.Value = cmdItem.Value
		case "write-aotype":
			op.Type = localio.WriteOpAOType
			op.Mode = cmdItem.Mode
		default:
			// Skip unknown command types
			continue
		}

		ops = append(ops, op)
	}

	// Initialize results array for all commands
	results := make([]localio.CommandResult, len(cmd.Commands))

	// Process reboot commands first
	for _, idx := range rebootIndices {
		cmdItem := cmd.Commands[idx]
		err := s.localioMgr.RebootCard(cmdItem.CardID)
		if err != nil {
			results[idx] = localio.CommandResult{
				Index:   idx,
				Status:  "error",
				Message: err.Error(),
			}
		} else {
			results[idx] = localio.CommandResult{
				Index:  idx,
				Status: "ok",
			}
		}
	}

	// Process write operations if any
	if len(ops) > 0 {
		writeResults := s.localioMgr.ProcessBatchWrite(ops)

		// Map write results back to original command indices
		// Create a mapping: original command index -> write operation index
		writeOpIdx := 0
		for i, cmdItem := range cmd.Commands {
			if cmdItem.Type == "reboot" {
				continue // Already processed
			}
			if cmdItem.Type == "write-do" || cmdItem.Type == "write-ao" || cmdItem.Type == "write-aotype" {
				if writeOpIdx < len(writeResults) {
					results[i] = writeResults[writeOpIdx]
					results[i].Index = i // Update index to match original command position
					writeOpIdx++
				}
			}
		}
	}

	// Convert results to response format
	responseResults := make([]localio.CommandResult, len(results))
	for i, result := range results {
		responseResults[i] = localio.CommandResult{
			Index:   result.Index,
			Status:  result.Status,
			Message: result.Message,
		}
	}

	response := WriteResponse{
		Type:    "write-response",
		Status:  "ok",
		Results: responseResults,
	}

	// Check if any command failed
	for i, result := range results {
		if result.Status == "error" {
			response.Status = "error"
			response.FailedIndex = i
			response.Message = result.Message
			break
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
