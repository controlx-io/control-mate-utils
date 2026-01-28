package localio

import (
	"fmt"
	"testing"

	"github.com/goburrow/modbus"
)

// MockClient implements modbus.Client
type MockClient struct {
	ReadCoilsFunc                  func(address, quantity uint16) ([]byte, error)
	ReadDiscreteInputsFunc         func(address, quantity uint16) ([]byte, error)
	ReadHoldingRegistersFunc       func(address, quantity uint16) ([]byte, error)
	ReadInputRegistersFunc         func(address, quantity uint16) ([]byte, error)
	WriteSingleCoilFunc            func(address, value uint16) ([]byte, error)
	WriteMultipleCoilsFunc         func(address, quantity uint16, value []byte) ([]byte, error)
	WriteSingleRegisterFunc        func(address, value uint16) ([]byte, error)
	WriteMultipleRegistersFunc     func(address, quantity uint16, value []byte) ([]byte, error)
	ReadWriteMultipleRegistersFunc func(readAddress, readQuantity, writeAddress, writeQuantity uint16, value []byte) ([]byte, error)
	MaskWriteRegisterFunc          func(address, andMask, orMask uint16) ([]byte, error)
	ReadFIFOQueueFunc              func(address uint16) ([]byte, error)
}

func (m *MockClient) ReadCoils(address, quantity uint16) ([]byte, error) {
	if m.ReadCoilsFunc != nil {
		return m.ReadCoilsFunc(address, quantity)
	}
	return []byte{}, nil
}
func (m *MockClient) ReadDiscreteInputs(address, quantity uint16) ([]byte, error) {
	if m.ReadDiscreteInputsFunc != nil {
		return m.ReadDiscreteInputsFunc(address, quantity)
	}
	return []byte{}, nil
}
func (m *MockClient) ReadHoldingRegisters(address, quantity uint16) ([]byte, error) {
	if m.ReadHoldingRegistersFunc != nil {
		return m.ReadHoldingRegistersFunc(address, quantity)
	}
	return []byte{}, nil
}
func (m *MockClient) ReadInputRegisters(address, quantity uint16) ([]byte, error) {
	if m.ReadInputRegistersFunc != nil {
		return m.ReadInputRegistersFunc(address, quantity)
	}
	return []byte{}, nil
}
func (m *MockClient) WriteSingleCoil(address, value uint16) ([]byte, error) {
	if m.WriteSingleCoilFunc != nil {
		return m.WriteSingleCoilFunc(address, value)
	}
	return []byte{}, nil
}
func (m *MockClient) WriteMultipleCoils(address, quantity uint16, value []byte) ([]byte, error) {
	if m.WriteMultipleCoilsFunc != nil {
		return m.WriteMultipleCoilsFunc(address, quantity, value)
	}
	return []byte{}, nil
}
func (m *MockClient) WriteSingleRegister(address, value uint16) ([]byte, error) {
	if m.WriteSingleRegisterFunc != nil {
		return m.WriteSingleRegisterFunc(address, value)
	}
	return []byte{}, nil
}
func (m *MockClient) WriteMultipleRegisters(address, quantity uint16, value []byte) ([]byte, error) {
	if m.WriteMultipleRegistersFunc != nil {
		return m.WriteMultipleRegistersFunc(address, quantity, value)
	}
	return []byte{}, nil
}
func (m *MockClient) ReadWriteMultipleRegisters(readAddress, readQuantity, writeAddress, writeQuantity uint16, value []byte) ([]byte, error) {
	return []byte{}, nil
}
func (m *MockClient) MaskWriteRegister(address, andMask, orMask uint16) ([]byte, error) {
	return []byte{}, nil
}
func (m *MockClient) ReadFIFOQueue(address uint16) ([]byte, error) {
	return []byte{}, nil
}

func TestManager_AddCard(t *testing.T) {
	mgr := NewManager()

	// Override factories
	mgr.handlerFactory = func(path string, cfg serialCfg) (ModbusHandler, error) {
		return &MockClientHandler{}, nil
	}
	mgr.clientFactory = func(h modbus.ClientHandler) modbus.Client {
		return &MockClient{
			ReadInputRegistersFunc: func(address, quantity uint16) ([]byte, error) {
				// Mock probing behavior: 8 regs = 16 bytes.
				// For IO4040: DI=4, DO=4, AI=0, AO=0.
				// Probing:
				// probeDI (8) -> fail?
				// The probe functions try to read different things.
				// Let's assume we want to mock a specific card.
				// Or simpler: specify module explicitly.
				return nil, fmt.Errorf("read error")
			},
		}
	}

	// Test adding with explicit module
	// We need the readCard to succeed to populate Last.
	// But readCard will call ReadDiscreteInputs etc depending on module.

	// Let's assume IO4040 (DI=4, DO=4)
	mgr.clientFactory = func(h modbus.ClientHandler) modbus.Client {
		return &MockClient{
			ReadDiscreteInputsFunc: func(address, quantity uint16) ([]byte, error) {
				// 4 DIs = 1 byte (packed)
				return []byte{0x0F}, nil // All ON
			},
			ReadCoilsFunc: func(address, quantity uint16) ([]byte, error) {
				// 4 DOs = 1 byte
				return []byte{0x00}, nil // All OFF
			},
			ReadHoldingRegistersFunc: func(address, quantity uint16) ([]byte, error) {
				// Serial number read
				if address == 0x0070 {
					return make([]byte, 20), nil
				}
				return nil, nil
			},
		}
	}

	card, err := mgr.AddCard("/dev/ttyUSB0", 1, "IO4040")
	if err != nil {
		t.Fatalf("AddCard failed: %v", err)
	}

	if card.Module != "IO4040" {
		t.Errorf("Expected module IO4040, got %s", card.Module)
	}

	if len(card.Last.DI) != 4 {
		t.Errorf("Expected 4 DIs, got %d", len(card.Last.DI))
	}
	if !card.Last.DI[0] {
		t.Error("Expected DI[0] to be true")
	}
}

func TestManager_QueueWriteDO(t *testing.T) {
	mgr := NewManager()

	// Mock factories
	mgr.handlerFactory = func(path string, cfg serialCfg) (ModbusHandler, error) {
		return &MockClientHandler{}, nil
	}

	writeCalled := false
	mgr.clientFactory = func(h modbus.ClientHandler) modbus.Client {
		return &MockClient{
			ReadDiscreteInputsFunc:   func(address, quantity uint16) ([]byte, error) { return []byte{0}, nil },
			ReadCoilsFunc:            func(address, quantity uint16) ([]byte, error) { return []byte{0}, nil },
			ReadHoldingRegistersFunc: func(address, quantity uint16) ([]byte, error) { return make([]byte, 20), nil },
			WriteMultipleCoilsFunc: func(address, quantity uint16, value []byte) ([]byte, error) {
				writeCalled = true
				if address != 1 {
					t.Errorf("Expected address 1, got %d", address)
				}
				if quantity != 1 {
					t.Errorf("Expected quantity 1, got %d", quantity)
				}
				// Check that the coil is set (bit 0 should be set)
				if len(value) == 0 || (value[0]&0x01) == 0 {
					t.Error("Expected coil to be set (bit 0)")
				}
				return []byte{}, nil
			},
		}
	}

	card, err := mgr.AddCard("/dev/ttyUSB0", 1, "IO4040")
	if err != nil {
		t.Fatalf("AddCard failed: %v", err)
	}

	// Queue a write
	err = mgr.QueueWriteDO(card.ID, 1, true)
	if err != nil {
		t.Fatalf("QueueWriteDO failed: %v", err)
	}

	// Process queue (now uses batch processing)
	mgr.ProcessWriteQueue()

	if !writeCalled {
		t.Error("WriteMultipleCoils was not called")
	}
}

func TestManager_AutoDiscover(t *testing.T) {
	// InitializeManager uses NewManager internaly but we can't easily mock InitializeManager
	// because it calls NewManager directly.
	// However, we can test the logic if we manually replicate what InitializeManager does
	// or if we refactor InitializeManager.
	// For now, let's just test AddCard logic which is central.
	// If we want to test detection:

	mgr := NewManager()
	mgr.handlerFactory = func(path string, cfg serialCfg) (ModbusHandler, error) {
		return &MockClientHandler{}, nil
	}
	mgr.clientFactory = func(h modbus.ClientHandler) modbus.Client {
		return &MockClient{
			// Probe logic:
			// probeDI (8) -> fail
			// probeDI (4) -> success
			// probeDO (8) -> fail
			// probeDO (4) -> success
			// probeAI -> fail
			// probeAO -> fail
			ReadDiscreteInputsFunc: func(address, quantity uint16) ([]byte, error) {
				if quantity == 8 {
					return nil, fmt.Errorf("err")
				}
				if quantity == 4 {
					return []byte{0}, nil
				}
				return nil, fmt.Errorf("err")
			},
			ReadCoilsFunc: func(address, quantity uint16) ([]byte, error) {
				if quantity == 8 {
					return nil, fmt.Errorf("err")
				}
				if quantity == 4 {
					return []byte{0}, nil
				}
				return nil, fmt.Errorf("err")
			},
			ReadInputRegistersFunc: func(address, quantity uint16) ([]byte, error) {
				return nil, fmt.Errorf("err")
			},
			ReadHoldingRegistersFunc: func(address, quantity uint16) ([]byte, error) {
				return nil, fmt.Errorf("err")
			},
		}
	}

	// Should detect IO4040
	card, err := mgr.AddCard("/dev/ttyUSB0", 1, "")
	if err != nil {
		t.Fatalf("AddCard auto-detect failed: %v", err)
	}

	if card.Module != "IO4040" {
		t.Errorf("Expected detected module IO4040, got %s", card.Module)
	}
}
