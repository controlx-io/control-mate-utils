package discovery

import (
	"bytes"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"time"

	"control-mate-utils/src/server"
	"control-mate-utils/src/server/config"
)

const (
	// ServerURL      = "https://jasperx.io/api/v1.0/device/jaspermate"
	serverURL      = "http://localhost:8080/api/v1.0/device/jaspermate"
	reportInterval = 5 * time.Minute
)

type Payload struct {
	DeviceID string   `json:"deviceId"`
	LocalIP  string   `json:"localIp"`
	OtherIPs []string `json:"otherIPs"`
	Type     string   `json:"type"`
}

// Start begins the discovery agent in a background goroutine
func Start() {
	go run()
}

func run() {
	log.Println("Starting Discovery Agent...")

	// 1. Send immediately on startup
	reportStatus()

	// 2. Ticker for the 5-minute heartbeat
	ticker := time.NewTicker(reportInterval)

	// 3. Network Change Monitor (Simplified Polling)
	// Real Netlink is complex; polling IP changes every 10s is robust and lightweight.
	monitorTicker := time.NewTicker(10 * time.Second)
	lastIP := getOutboundIP()

	for {
		select {
		case <-ticker.C:
			reportStatus()
		case <-monitorTicker.C:
			currentIP := getOutboundIP()
			if currentIP != lastIP {
				log.Printf("Network change detected: %s -> %s\n", lastIP, currentIP)
				lastIP = currentIP
				reportStatus()
			}
		}
	}
}

// getOutboundIP determines the preferred local IP for internet traffic
func getOutboundIP() string {
	// We don't actually connect, just ask the kernel for the routing preference
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Println("Error resolving outbound IP:", err)
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// getAllNetworkIPs collects all non-loopback IPv4 addresses from all network interfaces
func getAllNetworkIPs() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var allIPs []string
	for _, iface := range interfaces {
		// Skip loopback interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				// Only include IPv4 addresses
				if ipNet.IP.To4() != nil {
					allIPs = append(allIPs, ipNet.IP.String())
				}
			}
		}
	}

	return allIPs
}

func reportStatus() {
	localIP := getOutboundIP()
	if localIP == "" {
		return // No internet connection
	}

	// Get all network IPs
	allIPs := getAllNetworkIPs()

	// Separate local IP from other IPs
	var otherIPs []string
	for _, ip := range allIPs {
		if ip != localIP {
			otherIPs = append(otherIPs, ip)
		}
	}

	deviceType := "control-mate"
	if server.IsJasperMate() {
		deviceType = "jasper-mate"
	}

	data := Payload{
		DeviceID: config.GetDeviceID(),
		LocalIP:  localIP,
		OtherIPs: otherIPs,
		Type:     deviceType,
	}

	jsonData, _ := json.Marshal(data)
	log.Printf("Reported: %s\n", string(jsonData))

	// temporary disable
	// resp, err := httpPost(serverURL, jsonData)
	// if err != nil {
	// 	log.Printf("Failed to report to cloud: %v\n", err)
	// } else {
	// 	log.Printf("Reported status: localIp=%s, otherIPs=%v (Status: %d)\n", localIP, otherIPs, resp.StatusCode)
	// 	resp.Body.Close()
	// }
}

// Simple wrapper for HTTP POST with timeout
func httpPost(url string, data []byte) (*http.Response, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

