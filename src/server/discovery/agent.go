package discovery

import (
	"sync"

	"control-mate-utils/src/server"
	"control-mate-utils/src/server/config"
)

var (
	deviceType     string
	deviceTypeOnce sync.Once
)

// GetDeviceType returns the device type (controlmate or jaspermate)
// The result is cached after the first call for performance
func GetDeviceType() string {
	deviceTypeOnce.Do(func() {
		deviceType = "controlmate"
		if server.IsJasperMate() {
			deviceType = "jaspermate"
		}

		// Config override
		if config.GetConfig().Type != "" {
			deviceType = config.GetConfig().Type
		}
	})
	return deviceType
}
