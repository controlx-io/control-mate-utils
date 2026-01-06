package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"control-mate-utils/src/server"
	"control-mate-utils/src/server/config"
	"control-mate-utils/src/server/util"
)

const (
	defaultServerURL = "https://base.jasperx.io/api/v1.0/device/jaspermate"
	reportInterval   = 5 * time.Minute
)

var serverURL = defaultServerURL

func init() {
	if url := util.LoadEnvLocal("JASPER_MATE_URL"); url != "" {
		serverURL = url
	}
}

type Payload struct {
	DeviceID string   `json:"deviceId"`
	LocalIP  string   `json:"localIp"`
	OtherIPs []string `json:"otherIPs"`
	Type     string   `json:"type"` // jaspermate or controlmate
}

// Start begins the discovery agent in a background goroutine
func Start() {
	go run()
}

func run() {
	fmt.Println("Starting Discovery Agent...")

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
				fmt.Printf("Network change detected: %s -> %s\n", lastIP, currentIP)
				lastIP = currentIP
				reportStatus()
			}
		}
	}
}

// getOutboundIP determines the preferred local IP for internet traffic
var getOutboundIP = func() string {
	// We don't actually connect, just ask the kernel for the routing preference
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		fmt.Println("Error resolving outbound IP:", err)
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// getAllNetworkIPs collects all non-loopback IPv4 addresses from all network interfaces
var getAllNetworkIPs = func() []string {
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

func createPayload(localIP string, allIPs []string) Payload {
	// Separate local IP from other IPs
	var otherIPs []string
	for _, ip := range allIPs {
		if ip != localIP {
			otherIPs = append(otherIPs, ip)
		}
	}

	deviceType := "controlmate"
	if server.IsJasperMate() {
		deviceType = "jaspermate"
	}

	return Payload{
		DeviceID: config.GetDeviceID(),
		LocalIP:  localIP,
		OtherIPs: otherIPs,
		Type:     deviceType,
	}
}

func reportStatus() {
	localIP := getOutboundIP()
	if localIP == "" {
		return // No internet connection
	}

	// Get all network IPs
	allIPs := getAllNetworkIPs()

	data := createPayload(localIP, allIPs)

	jsonData, _ := json.Marshal(data)
	// Log if using custom URL
	if serverURL != defaultServerURL {
		fmt.Printf("Reported: %s\n", string(jsonData))
	}

	resp, err := httpPost(serverURL, jsonData)
	if err != nil {
		fmt.Printf("Failed to report to cloud: %v\n", err)
	} else {
		resp.Body.Close()
	}
}

// Simple wrapper for HTTP POST with timeout
var httpPost = func(url string, data []byte) (*http.Response, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}
