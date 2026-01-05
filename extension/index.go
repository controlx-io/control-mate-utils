package extension

import "log"

// InitializeManager creates a new manager, performs auto-discovery, and starts the read-write cycle
func InitializeManager() *Manager {
	mgr := NewManager()

	// Auto-discover slaves at startup
	portPath := "/dev/ttyS7"
	maxSlave := 5
	for sid := 1; sid <= maxSlave; sid++ {
		if card, err := mgr.AddCard(portPath, byte(sid), ""); err == nil {
			log.Printf("discovered slave %d on %s module=%s", sid, portPath, card.Module)
		}
	}

	// Start continuous read-write cycle
	mgr.StartCycle()
	log.Printf("started extension read-write cycle")

	return mgr
}
