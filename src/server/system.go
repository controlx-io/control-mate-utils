package server

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GetOsRelease reads /etc/os-release and returns the distribution ID
func GetOsRelease() string {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			// ID=debian or ID="debian"
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, "\"")
			return strings.ToLower(id)
		}
	}
	return ""
}

// IsJasperMate checks if the system is a JasperMate device
func IsJasperMate() bool {
	// Check for device file
	_, err := os.Stat("/dev/ttyS7")
	if err != nil && os.IsNotExist(err) {
		return false
	}

	// Check if OS is ubuntu
	return GetOsRelease() == "ubuntu"
}

// CheckNmcliAvailable checks if nmcli is installed and available
var execCommand = exec.Command

func CheckNmcliAvailable() bool {
	cmd := execCommand("which", "nmcli")
	err := cmd.Run()
	return err == nil
}

// CheckNetworkConnectivity checks for internet access
func CheckNetworkConnectivity() bool {
	// Try to connect to a reliable external service with a short timeout
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// FormatUptime formats a duration into a human-readable string
func FormatUptime(duration time.Duration) string {
	totalSeconds := int(duration.Seconds())
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	} else {
		return fmt.Sprintf("%dm", minutes)
	}
}
