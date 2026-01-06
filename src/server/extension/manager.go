package extension

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
	ID       string    `json:"id"`
	PortPath string    `json:"portPath"`
	SlaveID  byte      `json:"slaveId"`
	Module   string    `json:"module"`
	Last     CardState `json:"last"`
}

type writeOpType int

const (
	writeOpDO writeOpType = iota
	writeOpAO
	writeOpAOType
)

type writeOperation struct {
	CardID string
	Type   writeOpType
	Index  int     // For DO: uint16 cast, For AO/AOType: int
	Value  float32 // For DO: bool cast (0=false, 1=true), For AO: float32, For AOType: unused
	Mode   string  // For AOType only
}

type Manager struct {
	ports          map[string]*portClient
	cards          map[string]*Card
	mu             sync.Mutex
	nextID         int
	serial         serialCfg
	timeout        time.Duration
	cycleDelay     time.Duration    // Delay after write cycle before next loop
	operationDelay time.Duration    // Delay between each Modbus operation (RS485)
	writeQueue     []writeOperation // Queue of pending write operations
	stopChan       chan struct{}    // Channel to stop background goroutine
	clientFactory  ClientFactory    // Factory for creating modbus clients
	handlerFactory HandlerFactory   // Factory for creating modbus handlers
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
		ports:          make(map[string]*portClient),
		cards:          make(map[string]*Card),
		nextID:         1,
		serial:         serialCfg{Baud: 9600, Par: "N", Stop: 1, Data: 8},
		timeout:        50 * time.Millisecond,
		cycleDelay:     1 * time.Millisecond,
		operationDelay: 1 * time.Millisecond,
		writeQueue:     make([]writeOperation, 0),
		stopChan:       make(chan struct{}),
		clientFactory:  modbus.NewClient,
		handlerFactory: defaultHandlerFactory,
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

		state, err := pc.readCard(c.SlaveID, spec, false)
		if err != nil {
			c.Last.Error = err.Error()
		} else {
			// Preserve SN and AOType from previous state (read only during AddCard)
			state.SerialNumber = c.Last.SerialNumber
			state.AOType = c.Last.AOType
			c.Last = state
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

		state, err := pc.readCard(c.SlaveID, spec, false)
		if err != nil {
			c.Last.Error = err.Error()
		} else {
			// Preserve SN and AOType from previous state (read only during AddCard)
			state.SerialNumber = c.Last.SerialNumber
			state.AOType = c.Last.AOType
			c.Last = state
		}

		// Process any pending writes after each card read to minimize latency
		m.ProcessWriteQueue()
	}
	return cards
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

// ProcessWriteQueue processes all queued write operations
func (m *Manager) ProcessWriteQueue() {
	m.mu.Lock()
	queue := make([]writeOperation, len(m.writeQueue))
	copy(queue, m.writeQueue)
	m.writeQueue = m.writeQueue[:0] // Clear the queue
	m.mu.Unlock()

	for i, op := range queue {
		c, ok := m.GetCard(op.CardID)
		if !ok {
			log.Printf("write queue: card %s not found, skipping", op.CardID)
			continue
		}

		pc, err := m.ensurePort(c.PortPath)
		if err != nil {
			log.Printf("write queue: failed to get port for card %s: %v", op.CardID, err)
			continue
		}

		switch op.Type {
		case writeOpDO:
			state := op.Value != 0
			err = pc.writeDO(c.SlaveID, uint16(op.Index), state)
			if err == nil {
				log.Printf("[WRITE] DO card=%s slave=%d idx=%d state=%v", op.CardID, c.SlaveID, op.Index, state)
			}
		case writeOpAO:
			err = pc.writeAO(c.SlaveID, op.Index, op.Value)
			if err == nil {
				log.Printf("[WRITE] AO card=%s slave=%d idx=%d value=%f", op.CardID, c.SlaveID, op.Index, op.Value)
			}
		case writeOpAOType:
			err = pc.writeAOType(c.SlaveID, op.Index, op.Mode)
			if err == nil {
				log.Printf("[WRITE] AO-TYPE card=%s slave=%d idx=%d mode=%s", op.CardID, c.SlaveID, op.Index, op.Mode)
			}
		}

		if err != nil {
			log.Printf("write queue: error writing to card %s: %v", op.CardID, err)
		}

		// Add delay between writes if there are more writes coming
		// (each write already has operationDelay built-in, but this adds extra spacing for RS485)
		if i < len(queue)-1 {
			time.Sleep(m.operationDelay)
		}
	}
}

// RebootCard sends a reboot command to the specified card
func (m *Manager) RebootCard(cardID string) error {
	c, ok := m.GetCard(cardID)
	if !ok {
		return fmt.Errorf("card not found")
	}

	pc, err := m.ensurePort(c.PortPath)
	if err != nil {
		return err
	}

	return pc.reboot(c.SlaveID)
}
