package localio

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/goburrow/modbus"
)

type serialCfg struct {
	Baud int
	Par  string
	Stop int
	Data int
}

type portClient struct {
	path           string
	handler        ModbusHandler
	client         modbus.Client
	mu             sync.Mutex
	operationDelay time.Duration // Delay between Modbus operations for RS485
}

func detectModel(pc *portClient, slave byte) string {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Modbus handlers usually store SlaveID.
	// RTUClientHandler has SlaveId. TCPClientHandler also has it.
	// But ClientHandler interface doesn't expose it.
	// We need to type assert or use SetSlave if available (goburrow doesn't have SetSlave on interface)
	// Actually, goburrow/modbus RTUClientHandler has SlaveId field.
	// If we use a mock, we need to handle this.
	// For now, let's type assert to RTUClientHandler if possible, or use a custom interface.

	setSlaveID(pc.handler, slave)

	di, doCount, ai, ao := probeCounts(pc)
	return guessModel(di, doCount, ai, ao)
}

func setSlaveID(h ModbusHandler, slave byte) {
	h.SetSlave(slave)
}

// probeCounts detects DI/DO/AI/AO counts similar to read_di.go
func probeCounts(pc *portClient) (int, int, int, int) {
	di := probeDI(pc)
	doCount := probeDO(pc)
	ai := probeAI(pc)
	ao := probeAO(pc)
	return di, doCount, ai, ao
}

func probeDI(pc *portClient) int {
	if _, err := pc.client.ReadDiscreteInputs(0x0000, 8); err == nil {
		return 8
	}
	if _, err := pc.client.ReadDiscreteInputs(0x0000, 4); err == nil {
		return 4
	}
	return 0
}

func probeDO(pc *portClient) int {
	if _, err := pc.client.ReadCoils(0x0000, 8); err == nil {
		return 8
	}
	if _, err := pc.client.ReadCoils(0x0000, 4); err == nil {
		return 4
	}
	return 0
}

func probeAI(pc *portClient) int {
	// Known modules have up to 4 AI; read 4 channels (8 registers)
	if _, err := pc.client.ReadInputRegisters(0x0000, 8); err == nil {
		return 4
	}
	return 0
}

func probeAO(pc *portClient) int {
	if _, err := pc.client.ReadHoldingRegisters(0x0190, 4); err == nil {
		return 4
	}
	return 0
}

// unpackBits converts packed coil/DI bytes into a bool slice of length count.
func unpackBits(raw []byte, count int) []bool {
	out := make([]bool, count)
	for i := 0; i < count; i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		if byteIdx < len(raw) {
			out[i] = (raw[byteIdx] & (1 << bitIdx)) != 0
		}
	}
	return out
}

func (pc *portClient) readCard(slave byte, spec ModelSpec, readAll bool) (CardState, error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	setSlaveID(pc.handler, slave)
	state := CardState{Timestamp: time.Now()}

	if spec.DI > 0 {
		raw, err := pc.client.ReadDiscreteInputs(0x0000, uint16(spec.DI))
		if err != nil {
			state.Error = fmt.Sprintf("DI read error: %v", err)
			return state, err
		}
		state.DI = unpackBits(raw, spec.DI)
		time.Sleep(pc.operationDelay) // RS485 delay
	}

	if spec.DO > 0 {
		raw, err := pc.client.ReadCoils(0x0000, uint16(spec.DO))
		if err != nil {
			state.Error = fmt.Sprintf("DO read error: %v", err)
			return state, err
		}
		state.DO = unpackBits(raw, spec.DO)
		time.Sleep(pc.operationDelay) // RS485 delay
	}

	if spec.AI > 0 {
		quantity := uint16(spec.AI * 2)
		raw, err := pc.client.ReadInputRegisters(0x0000, quantity)
		if err != nil {
			state.Error = fmt.Sprintf("AI read error: %v", err)
			return state, err
		}
		state.AI = make([]float32, spec.AI)
		for i := 0; i < spec.AI; i++ {
			bits := binary.BigEndian.Uint32(raw[i*4 : i*4+4])
			state.AI[i] = math.Float32frombits(bits)
		}
		time.Sleep(pc.operationDelay) // RS485 delay
	}

	if spec.AO > 0 {
		quantity := uint16(spec.AO * 2)
		raw, err := pc.client.ReadHoldingRegisters(0x0000, quantity)
		if err != nil {
			state.Error = fmt.Sprintf("AO read error: %v", err)
			return state, err
		}
		state.AO = make([]float32, spec.AO)
		for i := 0; i < spec.AO; i++ {
			bits := binary.BigEndian.Uint32(raw[i*4 : i*4+4])
			state.AO[i] = math.Float32frombits(bits)
		}
		time.Sleep(pc.operationDelay) // RS485 delay

		if readAll {
			typeRaw, err := pc.client.ReadHoldingRegisters(0x0190, uint16(spec.AO))
			if err == nil {
				state.AOType = make([]string, spec.AO)
				for i := 0; i < spec.AO; i++ {
					val := binary.BigEndian.Uint16(typeRaw[i*2 : i*2+2])
					if val == 0x0001 {
						state.AOType[i] = "0-10V"
					} else if val == 0x0004 {
						state.AOType[i] = "4-20mA"
					} else {
						state.AOType[i] = fmt.Sprintf("0x%04X", val)
					}
				}
			}
			time.Sleep(pc.operationDelay) // RS485 delay
		}
	}

	if readAll {
		state.SerialNumber = pc.readSerialNumber()
		time.Sleep(pc.operationDelay) // RS485 delay
	}

	return state, nil
}

// readSerialNumber reads the serial number from Modbus registers 0x0070-0x0079
// Returns empty string if read fails or no serial number is found
func (pc *portClient) readSerialNumber() string {
	// Read Serial Number (10 words = 20 bytes = 20 characters)
	// Register address 0x0070-0x0079 (112-121 decimal)
	snRaw, err := pc.client.ReadHoldingRegisters(0x0070, 10)
	if err != nil || len(snRaw) < 20 {
		return ""
	}

	// ReadHoldingRegisters returns bytes, each register is 2 bytes
	// Convert to string, removing null terminators
	snBytes := make([]byte, 20)
	copy(snBytes, snRaw[:20])

	// Find null terminator or end of string
	nullIdx := 0
	for nullIdx < len(snBytes) && snBytes[nullIdx] != 0 {
		nullIdx++
	}

	return string(snBytes[:nullIdx])
}

func (pc *portClient) writeDO(slave byte, index uint16, state bool) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	setSlaveID(pc.handler, slave)

	var coil uint16 = 0x0000
	if state {
		coil = 0xFF00
	}
	_, err := pc.client.WriteSingleCoil(index, coil)
	if err == nil {
		time.Sleep(pc.operationDelay) // RS485 delay
	}
	return err
}

func (pc *portClient) writeAO(slave byte, index int, value float32) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	setSlaveID(pc.handler, slave)

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, math.Float32bits(value))

	// quantity is 2 registers (4 bytes)
	_, err := pc.client.WriteMultipleRegisters(uint16(index*2), 2, buf)
	if err == nil {
		time.Sleep(pc.operationDelay) // RS485 delay
	}
	return err
}

func (pc *portClient) writeAOType(slave byte, index int, mode string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	setSlaveID(pc.handler, slave)

	var val uint16
	if mode == "0-10V" {
		val = 0x0001
	} else {
		val = 0x0004
	}
	_, err := pc.client.WriteSingleRegister(uint16(0x0190+index), val)
	if err == nil {
		time.Sleep(pc.operationDelay) // RS485 delay
	}
	return err
}

func (pc *portClient) reboot(slave byte) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	setSlaveID(pc.handler, slave)

	// Register address 0x0010 (16 decimal), value 0xFF00
	_, err := pc.client.WriteSingleRegister(0x0010, 0xFF00)
	if err == nil {
		time.Sleep(pc.operationDelay) // RS485 delay
	}
	return err
}

// packBits converts a bool slice to packed bytes for Modbus WriteMultipleCoils
func packBits(values []bool) []byte {
	byteCount := (len(values) + 7) / 8
	bytes := make([]byte, byteCount)

	for i, val := range values {
		if val {
			byteIdx := i / 8
			bitIdx := uint(i % 8)
			bytes[byteIdx] |= (1 << bitIdx)
		}
	}

	return bytes
}

// writeMultipleDO writes multiple coils at once
func (pc *portClient) writeMultipleDO(slave byte, startIndex uint16, values []bool) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	setSlaveID(pc.handler, slave)

	// Convert bool slice to byte slice for Modbus
	quantity := uint16(len(values))
	bytes := packBits(values)

	_, err := pc.client.WriteMultipleCoils(startIndex, quantity, bytes)
	if err == nil {
		time.Sleep(pc.operationDelay) // RS485 delay
	}
	return err
}

// writeMultipleAO writes multiple AO values at once
func (pc *portClient) writeMultipleAO(slave byte, startIndex int, values []float32) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	setSlaveID(pc.handler, slave)

	// Each AO value is 2 registers (4 bytes)
	quantity := uint16(len(values) * 2)
	buf := make([]byte, len(values)*4)

	for i, val := range values {
		binary.BigEndian.PutUint32(buf[i*4:(i+1)*4], math.Float32bits(val))
	}

	_, err := pc.client.WriteMultipleRegisters(uint16(startIndex*2), quantity, buf)
	if err == nil {
		time.Sleep(pc.operationDelay) // RS485 delay
	}
	return err
}
