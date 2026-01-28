package localio

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/goburrow/modbus"
)

// ModbusHandler interface extends modbus.ClientHandler with Connect method and SetSlave
type ModbusHandler interface {
	modbus.ClientHandler
	Connect() error
	SetSlave(slave byte)
}

// rtuWrapper wraps modbus.RTUClientHandler to satisfy ModbusHandler interface
type rtuWrapper struct {
	*modbus.RTUClientHandler
}

func (r *rtuWrapper) SetSlave(slave byte) {
	r.SlaveId = slave
}

type ClientFactory func(handler modbus.ClientHandler) modbus.Client
type HandlerFactory func(path string, cfg serialCfg) (ModbusHandler, error)

// StateChangeCallback is called when card state changes (DI or AI values)
type StateChangeCallback func(cards []*Card)

// SafeStateConfig defines the safe state values for outputs when JN (TCP client) disconnects
// This configuration is easily editable for future changes
type SafeStateConfig struct {
	// DOState is the safe state for all digital outputs (false = open/off)
	DOState bool
	// AOVoltageValue is the safe value for analog outputs configured as 0-10V
	AOVoltageValue float32
	// AOCurrentValue is the safe value for analog outputs configured as 4-20mA
	AOCurrentValue float32
}

// DefaultSafeStateConfig returns the default safe state configuration
// Digital outputs: false (open/off)
// Analog outputs are specified in engineering units and will be written as value * 1000:
// - 0-10V: volts (e.g. 0.0 -> 0)
// - 4-20mA: milliamps (e.g. 4.0 -> 4000)
func DefaultSafeStateConfig() SafeStateConfig {
	return SafeStateConfig{
		DOState:        false, // Digital outputs open/off
		AOVoltageValue: 0.0,   // Volts (will be written as V * 1000)
		AOCurrentValue: 4.0,   // mA (will be written as mA * 1000)
	}
}

type CardState struct {
	Timestamp    time.Time `json:"timestamp"`
	DI           []bool    `json:"di,omitempty"`
	DO           []bool    `json:"do,omitempty"`
	AI           []float32 `json:"ai,omitempty"`
	AO           []float32 `json:"ao,omitempty"`
	AOType       []string  `json:"aoType,omitempty"`
	SerialNumber string    `json:"serialNumber,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type Card struct {
	ID            string    `json:"id"`
	PortPath      string    `json:"portPath"`
	SlaveID       byte      `json:"slaveId"`
	Module        string    `json:"module"`
	Last          CardState `json:"last"`
	needsFullRead bool      // Flag to force full read (AO types, serial number) on next read cycle
}

type writeOpType int

const (
	writeOpDO writeOpType = iota
	writeOpAO
	writeOpAOType
)

// WriteOpType is the exported version of writeOpType for use by TCP server
type WriteOpType = writeOpType

// WriteOpDO, WriteOpAO, WriteOpAOType are exported constants
const (
	WriteOpDO     = writeOpDO
	WriteOpAO     = writeOpAO
	WriteOpAOType = writeOpAOType
)

type writeOperation struct {
	CardID string
	Type   writeOpType
	Index  int     // For DO: uint16 cast, For AO/AOType: int
	Value  float32 // For DO: bool cast (0=false, 1=true), For AO: float32, For AOType: unused
	Mode   string  // For AOType only
}

// WriteOperation is the exported version of writeOperation for use by TCP server
type WriteOperation = writeOperation

type Manager struct {
	ports               map[string]*portClient
	cards               map[string]*Card
	mu                  sync.Mutex
	nextID              int
	serial              serialCfg
	timeout             time.Duration
	cycleDelay          time.Duration       // Delay after write cycle before next loop
	operationDelay      time.Duration       // Delay between each Modbus operation (RS485)
	writeQueue          []writeOperation    // Queue of pending write operations
	stopChan            chan struct{}       // Channel to stop background goroutine
	clientFactory       ClientFactory       // Factory for creating modbus clients
	handlerFactory      HandlerFactory      // Factory for creating modbus handlers
	stateChangeCallback StateChangeCallback // Callback for state changes (DI/AI)
	safeStateConfig     SafeStateConfig     // Safe state configuration for outputs
}

func defaultHandlerFactory(path string, cfg serialCfg) (ModbusHandler, error) {
	h := modbus.NewRTUClientHandler(path)
	h.BaudRate = cfg.Baud
	h.DataBits = cfg.Data
	h.Parity = cfg.Par
	h.StopBits = cfg.Stop
	return &rtuWrapper{h}, nil
}

func NewManager() *Manager {
	return &Manager{
		ports:           make(map[string]*portClient),
		cards:           make(map[string]*Card),
		nextID:          1,
		serial:          serialCfg{Baud: 9600, Par: "N", Stop: 1, Data: 8},
		timeout:         200 * time.Millisecond,
		cycleDelay:      10 * time.Millisecond,
		operationDelay:  2 * time.Millisecond,
		writeQueue:      make([]writeOperation, 0),
		stopChan:        make(chan struct{}),
		clientFactory:   modbus.NewClient,
		handlerFactory:  defaultHandlerFactory,
		safeStateConfig: DefaultSafeStateConfig(),
	}
}

func (m *Manager) ensurePort(path string) (*portClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.ports[path]; ok {
		return p, nil
	}

	h, err := m.handlerFactory(path, m.serial)
	if err != nil {
		return nil, err
	}

	// We need to set timeout on the handler if possible, but ClientHandler interface doesn't have Timeout.
	// However, RTUClientHandler has it.
	// For testing, we might ignore it or assert type.
	if rtu, ok := h.(*rtuWrapper); ok {
		rtu.RTUClientHandler.Timeout = m.timeout
	}

	if err := h.Connect(); err != nil {
		return nil, err
	}

	p := &portClient{
		path:           path,
		handler:        h,
		client:         m.clientFactory(h),
		operationDelay: m.operationDelay,
	}
	m.ports[path] = p
	return p, nil
}

func (m *Manager) AddCard(portPath string, slave byte, module string) (*Card, error) {
	pc, err := m.ensurePort(portPath)
	if err != nil {
		return nil, err
	}

	if module == "" {
		module = detectModel(pc, slave)
		if module == "" {
			return nil, fmt.Errorf("unable to detect module; specify module explicitly")
		}
	}

	spec, ok := ModelTable[module]
	if !ok {
		return nil, fmt.Errorf("unknown module %s", module)
	}

	m.mu.Lock()
	id := m.nextID
	m.nextID++
	c := &Card{
		ID:       strconv.Itoa(id),
		PortPath: portPath,
		SlaveID:  slave,
		Module:   spec.Name,
	}
	m.cards[c.ID] = c
	m.mu.Unlock()

	state, err := pc.readCard(slave, spec, true)
	if err == nil {
		c.Last = state
	}

	return c, nil
}

func (m *Manager) GetCard(id string) (*Card, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cards[id]
	return c, ok
}

func (m *Manager) RemoveCard(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cards[id]; !ok {
		return false
	}
	delete(m.cards, id)
	return true
}

func (m *Manager) RefreshAll() []*Card {
	m.mu.Lock()
	cards := make([]*Card, 0, len(m.cards))
	for _, c := range m.cards {
		cards = append(cards, c)
	}
	m.mu.Unlock()

	// Sort by ID for consistent ordering when returned to HTTP handlers
	sort.Slice(cards, func(i, j int) bool {
		idi, _ := strconv.Atoi(cards[i].ID)
		idj, _ := strconv.Atoi(cards[j].ID)
		return idi < idj
	})

	for _, c := range cards {
		spec := ModelTable[c.Module]

		// Get port directly - ports are created when cards are added via AddCard()
		m.mu.Lock()
		pc, ok := m.ports[c.PortPath]
		m.mu.Unlock()

		if !ok {
			// Port should exist, but handle edge case defensively
			c.Last.Error = fmt.Sprintf("port %s not found", c.PortPath)
			continue
		}

		// Check if we need a full read (e.g., after reboot)
		m.mu.Lock()
		readAll := c.needsFullRead
		if readAll {
			// Clear the flag after we've read it
			c.needsFullRead = false
		}
		m.mu.Unlock()

		state, err := pc.readCard(c.SlaveID, spec, readAll)
		if err != nil {
			c.Last.Error = err.Error()
		} else {
			if readAll {
				// Full read includes AO types and serial number, use them directly
				c.Last = state
			} else {
				// Preserve SN and AOType from previous state (read only during AddCard)
				state.SerialNumber = c.Last.SerialNumber
				state.AOType = c.Last.AOType
				c.Last = state
			}
		}
	}
	return cards
}

// GetAllCards returns all cards without reading (uses cached state)
// This is used by HTTP handlers since the cycle already keeps cards up to date
func (m *Manager) GetAllCards() []*Card {
	m.mu.Lock()
	cards := make([]*Card, 0, len(m.cards))
	for _, c := range m.cards {
		cards = append(cards, c)
	}
	m.mu.Unlock()

	// Sort by ID for consistent ordering
	sort.Slice(cards, func(i, j int) bool {
		idi, _ := strconv.Atoi(cards[i].ID)
		idj, _ := strconv.Atoi(cards[j].ID)
		return idi < idj
	})

	return cards
}

// ReadAllAndProcessWrites reads all cards and processes pending writes after each card read
// This minimizes write latency by processing writes immediately as they're queued
func (m *Manager) ReadAllAndProcessWrites() []*Card {
	m.mu.Lock()
	cards := make([]*Card, 0, len(m.cards))
	for _, c := range m.cards {
		cards = append(cards, c)
	}
	m.mu.Unlock()

	// Sort by ID for consistent ordering when returned to HTTP handlers
	sort.Slice(cards, func(i, j int) bool {
		idi, _ := strconv.Atoi(cards[i].ID)
		idj, _ := strconv.Atoi(cards[j].ID)
		return idi < idj
	})

	hasStateChange := false
	for _, c := range cards {
		spec := ModelTable[c.Module]

		// Get port directly - ports are created when cards are added via AddCard()
		m.mu.Lock()
		pc, ok := m.ports[c.PortPath]
		m.mu.Unlock()

		if !ok {
			// Port should exist, but handle edge case defensively
			c.Last.Error = fmt.Sprintf("port %s not found", c.PortPath)
			continue
		}

		// Store previous state for change detection
		prevState := c.Last

		// Check if we need a full read (e.g., after reboot)
		m.mu.Lock()
		readAll := c.needsFullRead
		if readAll {
			// Clear the flag after we've read it
			c.needsFullRead = false
		}
		m.mu.Unlock()

		state, err := pc.readCard(c.SlaveID, spec, readAll)
		if err != nil {
			c.Last.Error = err.Error()
		} else {
			if readAll {
				// Full read includes AO types and serial number, use them directly
				c.Last = state
			} else {
				// Preserve SN and AOType from previous state (read only during AddCard)
				state.SerialNumber = c.Last.SerialNumber
				state.AOType = c.Last.AOType
				c.Last = state
			}
		}

		// Check if DI or AI changed
		if !hasStateChange {
			hasStateChange = m.detectStateChange(&prevState, &c.Last)
		}

		// Process any pending writes after each card read to minimize latency
		m.ProcessWriteQueue()
	}

	// Call state change callback if DI or AI changed
	if hasStateChange {
		m.mu.Lock()
		callback := m.stateChangeCallback
		m.mu.Unlock()
		if callback != nil {
			// Get fresh copy of all cards for callback
			callbackCards := m.GetAllCards()
			callback(callbackCards)
		}
	}

	return cards
}

// detectStateChange checks if DI or AI values have changed between two states
func (m *Manager) detectStateChange(oldState, newState *CardState) bool {
	// Check DI changes
	if len(newState.DI) != len(oldState.DI) {
		return true
	}
	for i := range newState.DI {
		if newState.DI[i] != oldState.DI[i] {
			return true
		}
	}

	// Check AI changes
	if len(newState.AI) != len(oldState.AI) {
		return true
	}
	for i := range newState.AI {
		if newState.AI[i] != oldState.AI[i] {
			return true
		}
	}

	return false
}

// StartCycle starts the continuous read-write cycle: interleaves reads and writes
// This prevents writes from being delayed when there are many cards to read
func (m *Manager) StartCycle() {
	go func() {
		for {
			select {
			case <-m.stopChan:
				return
			default:
				// Read all cards and process writes after each card read
				m.ReadAllAndProcessWrites()
				time.Sleep(m.cycleDelay)
			}
		}
	}()
}

// StopCycle stops the background cycle goroutine
func (m *Manager) StopCycle() {
	close(m.stopChan)
}

// QueueWriteDO queues a DO write operation
func (m *Manager) QueueWriteDO(cardID string, index int, state bool) error {
	c, ok := m.GetCard(cardID)
	if !ok {
		return fmt.Errorf("card not found")
	}

	spec := ModelTable[c.Module]
	if index < 0 || index >= spec.DO {
		return fmt.Errorf("index out of range")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var value float32
	if state {
		value = 1.0
	}
	m.writeQueue = append(m.writeQueue, writeOperation{
		CardID: cardID,
		Type:   writeOpDO,
		Index:  index,
		Value:  value,
	})

	return nil
}

// QueueWriteAO queues an AO write operation
func (m *Manager) QueueWriteAO(cardID string, index int, value float32) error {
	c, ok := m.GetCard(cardID)
	if !ok {
		return fmt.Errorf("card not found")
	}

	spec := ModelTable[c.Module]
	if index < 0 || index >= spec.AO {
		return fmt.Errorf("index out of range")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.writeQueue = append(m.writeQueue, writeOperation{
		CardID: cardID,
		Type:   writeOpAO,
		Index:  index,
		Value:  value,
	})

	return nil
}

// QueueWriteAOType queues an AO type write operation
func (m *Manager) QueueWriteAOType(cardID string, index int, mode string) error {
	c, ok := m.GetCard(cardID)
	if !ok {
		return fmt.Errorf("card not found")
	}

	spec := ModelTable[c.Module]
	if index < 0 || index >= spec.AO {
		return fmt.Errorf("index out of range")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.writeQueue = append(m.writeQueue, writeOperation{
		CardID: cardID,
		Type:   writeOpAOType,
		Index:  index,
		Mode:   mode,
	})

	return nil
}

// ProcessWriteQueue processes all queued write operations using batch optimization
func (m *Manager) ProcessWriteQueue() {
	m.mu.Lock()
	queue := make([]writeOperation, len(m.writeQueue))
	copy(queue, m.writeQueue)
	m.writeQueue = m.writeQueue[:0] // Clear the queue
	m.mu.Unlock()

	if len(queue) == 0 {
		return
	}

	// Use batch processing for better performance
	results := m.ProcessBatchWrite(queue)

	// Log any errors from batch processing
	for i, result := range results {
		if result.Status == "error" {
			log.Printf("write queue: error writing operation %d: %v", i, result.Message)
		}
	}
}

// RebootCard sends a reboot command to the specified card
func (m *Manager) RebootCard(cardID string) error {
	m.mu.Lock()
	c, ok := m.cards[cardID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("card not found")
	}

	// Set flag to read all info (AO types) on next read cycle after reboot
	c.needsFullRead = true
	m.mu.Unlock()

	pc, err := m.ensurePort(c.PortPath)
	if err != nil {
		return err
	}

	return pc.reboot(c.SlaveID)
}

// SetStateChangeCallback sets a callback that will be called when card state changes (DI or AI)
func (m *Manager) SetStateChangeCallback(callback StateChangeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateChangeCallback = callback
}

// CommandResult represents the result of a single command in a batch
type CommandResult struct {
	Index   int    `json:"index"`             // Index in the original commands array
	Status  string `json:"status"`            // "ok" or "error"
	Message string `json:"message,omitempty"` // Optional error message
}

// WriteGroup represents a group of write operations that can be combined
type WriteGroup struct {
	CardID       string
	RegisterType writeOpType
	Operations   []writeOperation
}

// GroupWriteOperations groups write operations by card and register type
func (m *Manager) GroupWriteOperations(ops []writeOperation) []WriteGroup {
	// Group by (cardID, registerType)
	groups := make(map[string]*WriteGroup)

	for _, op := range ops {
		key := fmt.Sprintf("%s:%d", op.CardID, op.Type)
		if group, exists := groups[key]; exists {
			group.Operations = append(group.Operations, op)
		} else {
			groups[key] = &WriteGroup{
				CardID:       op.CardID,
				RegisterType: op.Type,
				Operations:   []writeOperation{op},
			}
		}
	}

	// Convert map to slice
	result := make([]WriteGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}

	return result
}

// shouldWrite checks if a write operation is needed (value changed)
func (m *Manager) shouldWrite(op writeOperation, card *Card) bool {
	switch op.Type {
	case writeOpDO:
		if op.Index >= 0 && op.Index < len(card.Last.DO) {
			currentState := card.Last.DO[op.Index]
			newState := op.Value != 0
			return currentState != newState
		}
	case writeOpAO:
		if op.Index >= 0 && op.Index < len(card.Last.AO) {
			currentValue := card.Last.AO[op.Index]
			return currentValue != op.Value
		}
	case writeOpAOType:
		if op.Index >= 0 && op.Index < len(card.Last.AOType) {
			currentMode := card.Last.AOType[op.Index]
			return currentMode != op.Mode
		}
	}
	return true // Default to writing if we can't determine
}

// ProcessBatchWrite processes a batch of write operations with optimization
func (m *Manager) ProcessBatchWrite(ops []writeOperation) []CommandResult {
	results := make([]CommandResult, len(ops))

	// Validate all operations first
	for i, op := range ops {
		card, ok := m.GetCard(op.CardID)
		if !ok {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: "card not found",
			}
			continue
		}

		// Validate index ranges
		spec := ModelTable[card.Module]
		var maxIndex int
		switch op.Type {
		case writeOpDO:
			maxIndex = spec.DO
		case writeOpAO, writeOpAOType:
			maxIndex = spec.AO
		}

		if op.Index < 0 || op.Index >= maxIndex {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: "index out of range",
			}
			continue
		}

		// Check if value actually changed (skip if unchanged)
		if !m.shouldWrite(op, card) {
			results[i] = CommandResult{
				Index:   i,
				Status:  "ok",
				Message: "value unchanged, skipped",
			}
			continue
		}
	}

	// Filter out operations that failed validation or were skipped
	// Track mapping: validOps index -> original ops index
	validOps := make([]writeOperation, 0)
	validToOrig := make([]int, 0) // Maps validOps[i] -> original ops index
	for i, op := range ops {
		if results[i].Status == "" { // Not yet processed (valid operation)
			validOps = append(validOps, op)
			validToOrig = append(validToOrig, i)
		}
	}

	if len(validOps) == 0 {
		return results
	}

	// Group operations by (cardID, registerType)
	groups := m.GroupWriteOperations(validOps)

	// Process each group
	for _, group := range groups {
		groupResults := m.processWriteGroup(group)

		// Map group results back to original indices
		// Find which validOps indices correspond to this group
		for j, groupOp := range group.Operations {
			if j >= len(groupResults) {
				continue
			}
			// Find the index in validOps array
			validIdx := -1
			for k, validOp := range validOps {
				if validOp.CardID == groupOp.CardID &&
					validOp.Type == groupOp.Type &&
					validOp.Index == groupOp.Index {
					validIdx = k
					break
				}
			}
			// Map back to original index
			if validIdx >= 0 && validIdx < len(validToOrig) {
				origIdx := validToOrig[validIdx]
				results[origIdx] = groupResults[j]
				results[origIdx].Index = origIdx // Update index to match original position
			}
		}
	}

	return results
}

// processWriteGroup processes a group of write operations for the same card and register type
func (m *Manager) processWriteGroup(group WriteGroup) []CommandResult {
	card, ok := m.GetCard(group.CardID)
	if !ok {
		// All operations in group fail
		results := make([]CommandResult, len(group.Operations))
		for i := range results {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: "card not found",
			}
		}
		return results
	}

	pc, err := m.ensurePort(card.PortPath)
	if err != nil {
		results := make([]CommandResult, len(group.Operations))
		for i := range results {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: fmt.Sprintf("failed to get port: %v", err),
			}
		}
		return results
	}

	results := make([]CommandResult, len(group.Operations))

	switch group.RegisterType {
	case writeOpDO:
		m.processBatchDO(pc, card, group.Operations, results)
	case writeOpAO:
		m.processBatchAO(pc, card, group.Operations, results)
	case writeOpAOType:
		m.processBatchAOType(pc, card, group.Operations, results)
	}

	return results
}

// processBatchDO processes multiple DO write operations
func (m *Manager) processBatchDO(pc *portClient, card *Card, ops []writeOperation, results []CommandResult) {
	if len(ops) == 0 {
		return
	}

	// Find min and max indices
	minIdx := ops[0].Index
	maxIdx := ops[0].Index
	for _, op := range ops {
		if op.Index < minIdx {
			minIdx = op.Index
		}
		if op.Index > maxIdx {
			maxIdx = op.Index
		}
	}

	// Create array covering all indices from min to max
	count := maxIdx - minIdx + 1
	values := make([]bool, count)

	// Initialize with cached values
	for i := 0; i < count; i++ {
		idx := minIdx + i
		if idx < len(card.Last.DO) {
			values[i] = card.Last.DO[idx]
		}
	}

	// Override with new values from operations
	for _, op := range ops {
		idx := op.Index - minIdx
		values[idx] = op.Value != 0
	}

	// Write all coils at once
	err := pc.writeMultipleDO(card.SlaveID, uint16(minIdx), values)

	// Set results
	for i := range ops {
		if err != nil {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: err.Error(),
			}
		} else {
			results[i] = CommandResult{
				Index:  i,
				Status: "ok",
			}
		}
	}
}

// processBatchAO processes multiple AO write operations
func (m *Manager) processBatchAO(pc *portClient, card *Card, ops []writeOperation, results []CommandResult) {
	if len(ops) == 0 {
		return
	}

	// Find min and max indices
	minIdx := ops[0].Index
	maxIdx := ops[0].Index
	for _, op := range ops {
		if op.Index < minIdx {
			minIdx = op.Index
		}
		if op.Index > maxIdx {
			maxIdx = op.Index
		}
	}

	// Create array covering all indices from min to max
	count := maxIdx - minIdx + 1
	values := make([]float32, count)

	// Initialize with cached values
	for i := 0; i < count; i++ {
		idx := minIdx + i
		if idx < len(card.Last.AO) {
			values[i] = card.Last.AO[idx]
		}
	}

	// Override with new values from operations
	for _, op := range ops {
		idx := op.Index - minIdx
		values[idx] = op.Value
	}

	// Write all AO values at once
	err := pc.writeMultipleAO(card.SlaveID, minIdx, values)

	// Set results
	for i := range ops {
		if err != nil {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: err.Error(),
			}
		} else {
			results[i] = CommandResult{
				Index:  i,
				Status: "ok",
			}
		}
	}
}

// processBatchAOType processes multiple AOType write operations
func (m *Manager) processBatchAOType(pc *portClient, card *Card, ops []writeOperation, results []CommandResult) {
	// AOType writes are to different register addresses (0x0190 + index)
	// They cannot be combined into a single WriteMultipleRegisters if addresses are non-contiguous
	// For now, process individually but could be optimized if addresses are contiguous

	for i, op := range ops {
		err := pc.writeAOType(card.SlaveID, op.Index, op.Mode)
		if err != nil {
			results[i] = CommandResult{
				Index:   i,
				Status:  "error",
				Message: err.Error(),
			}
		} else {
			results[i] = CommandResult{
				Index:  i,
				Status: "ok",
			}
		}

		// Add delay between writes if there are more
		if i < len(ops)-1 {
			time.Sleep(pc.operationDelay)
		}
	}
}

// WriteAllOutputsToSafeState writes all DO and AO outputs to their safe state values
// This is called when JN (TCP client) disconnects to ensure all outputs are in a safe state
func (m *Manager) WriteAllOutputsToSafeState() error {
	m.mu.Lock()
	cards := make([]*Card, 0, len(m.cards))
	for _, c := range m.cards {
		cards = append(cards, c)
	}
	safeConfig := m.safeStateConfig
	m.mu.Unlock()

	var firstErr error
	for _, card := range cards {
		spec := ModelTable[card.Module]

		// Get port for this card
		pc, err := m.ensurePort(card.PortPath)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("card %s: failed to get port: %v", card.ID, err)
			}
			log.Printf("WriteAllOutputsToSafeState: card %s port error: %v", card.ID, err)
			continue
		}

		// Write all DO outputs to safe state (false = open/off)
		if spec.DO > 0 {
			doValues := make([]bool, spec.DO)
			for i := range doValues {
				doValues[i] = safeConfig.DOState
			}
			err := pc.writeMultipleDO(card.SlaveID, 0, doValues)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("card %s: failed to write DO to safe state: %v", card.ID, err)
				}
				log.Printf("WriteAllOutputsToSafeState: card %s DO write error: %v", card.ID, err)
			} else {
				log.Printf("WriteAllOutputsToSafeState: card %s - set all %d DO outputs to safe state (%v)", card.ID, spec.DO, safeConfig.DOState)
			}
		}

		// Write all AO outputs to safe state based on their type
		if spec.AO > 0 {
			// Read current AO types if not already cached
			m.mu.Lock()
			cardState := card.Last
			m.mu.Unlock()

			aoValues := make([]float32, spec.AO)
			for i := 0; i < spec.AO; i++ {
				// Determine safe value based on AO type
				if i < len(cardState.AOType) && cardState.AOType[i] == "4-20mA" {
					// Safe config is in mA; module expects raw value = mA * 1000
					aoValues[i] = safeConfig.AOCurrentValue * 1000
				} else {
					// Default to voltage value (0-10V or unknown type)
					// Safe config is in V; module expects raw value = V * 1000
					aoValues[i] = safeConfig.AOVoltageValue * 1000
				}
			}

			err := pc.writeMultipleAO(card.SlaveID, 0, aoValues)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("card %s: failed to write AO to safe state: %v", card.ID, err)
				}
				log.Printf("WriteAllOutputsToSafeState: card %s AO write error: %v", card.ID, err)
			} else {
				log.Printf("WriteAllOutputsToSafeState: card %s - set all %d AO outputs to safe state", card.ID, spec.AO)
			}
		}
	}

	if firstErr != nil {
		return fmt.Errorf("WriteAllOutputsToSafeState completed with errors: %v", firstErr)
	}

	log.Printf("WriteAllOutputsToSafeState: all outputs set to safe state successfully")
	return nil
}
