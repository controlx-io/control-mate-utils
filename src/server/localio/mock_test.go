package localio

import (
	"github.com/goburrow/modbus"
)

// MockClientHandler implements ModbusHandler (modbus.ClientHandler + Connect)
type MockClientHandler struct {
	SlaveID byte
}

func (m *MockClientHandler) Connect() error {
	return nil
}
func (m *MockClientHandler) Close() error {
	return nil
}
func (m *MockClientHandler) Send(aduRequest []byte) (aduResponse []byte, err error) {
	return []byte{}, nil
}
func (m *MockClientHandler) Verify(aduRequest []byte, aduResponse []byte) (err error) {
	return nil
}
func (m *MockClientHandler) Decode(aduResponse []byte) (pdu *modbus.ProtocolDataUnit, err error) {
	return &modbus.ProtocolDataUnit{}, nil
}
func (m *MockClientHandler) Encode(pdu *modbus.ProtocolDataUnit) (adu []byte, err error) {
	return []byte{}, nil
}
func (m *MockClientHandler) SetSlave(slave byte) {
	m.SlaveID = slave
}
