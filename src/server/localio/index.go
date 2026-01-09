package localio

import "log"

// InitializeManager creates a new manager, performs auto-discovery, and starts the read-write cycle
func InitializeManager() *Manager {
	mgr := NewManager()

	// Auto-discover slaves at startup
	portPath := "/dev/ttyS7"
	maxSlave := 5
	discovered := 0
	for sid := 1; sid <= maxSlave; sid++ {
		if card, err := mgr.AddCard(portPath, byte(sid), ""); err == nil {
			log.Printf("discovered slave %d on %s module=%s", sid, portPath, card.Module)
			discovered++
		}
	}

	// Only start continuous read-write cycle if at least one card was discovered
	if discovered > 0 {
		mgr.StartCycle()
		log.Printf("started local IO read-write cycle (%d card(s) discovered)", discovered)
	} else {
		log.Printf("no local IO cards discovered on %s; skipping read-write cycle", portPath)
	}

	return mgr
}
