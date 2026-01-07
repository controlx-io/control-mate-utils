package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"control-mate-utils/src/server"
	"control-mate-utils/src/server/discovery"
	"control-mate-utils/src/server/extension"
	"control-mate-utils/src/server/tcp"

	"github.com/gorilla/mux"
)

// For testing purposes
var (
	execCommand        = exec.Command
	execCommandContext = exec.CommandContext
)

//go:embed src/web/templates/*
var templateFS embed.FS

//go:embed build/static/*
var staticFS embed.FS

type NetworkInterface struct {
	Name    string   `json:"name"`
	IPAddrs []string `json:"ip_addresses"`
	Status  string   `json:"status"`
}

type WiFiNetwork struct {
	SSID     string `json:"ssid"`
	Signal   string `json:"signal"`
	Security string `json:"security"`
}

type SavedNetwork struct {
	SSID   string `json:"ssid"`
	UUID   string `json:"uuid"`
	Active bool   `json:"active"`
	Device string `json:"device"`
}

type CurrentWiFi struct {
	SSID      string `json:"ssid"`
	Signal    string `json:"signal"`
	Security  string `json:"security"`
	Connected bool   `json:"connected"`
}

type Process struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	CPU     string `json:"cpu"`
	Memory  string `json:"memory"`
	User    string `json:"user"`
	Command string `json:"command"`
}

type ConnectionRequest struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
	Security string `json:"security"`
}

type SystemHealth struct {
	Status       string `json:"status"`
	Uptime       string `json:"uptime"`
	NetworkCheck bool   `json:"network_check"`
	LastCheck    string `json:"last_check"`
}

type TemplateData struct {
	Title       string
	ActiveNav   string
	Version     string
	PageContent string
	DeviceName  string
	Subtitle    string
	ActivePage  string
	SidebarIcon string
}

type App struct {
	templates      *template.Template
	nmcliAvailable bool
	version        string
	startTime      time.Time
	extensionMgr   *extension.Manager
	tcpServer      *tcp.TCPServer
}

var nmcliAvailable bool

func readVersion() string {
	data, err := staticFS.ReadFile("build/static/version.txt")
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(string(data))
}

func getDeviceName() string {
	deviceType := discovery.GetDeviceType()
	if deviceType == "jaspermate" {
		return "JasperMate"
	}
	return "ControlMate"
}

func NewApp() *App {
	templates := template.Must(template.ParseFS(templateFS, "src/web/templates/*.html"))
	nmcliAvailable = server.CheckNmcliAvailable()
	version := readVersion()

	// Initialize extension manager
	extMgr := extension.InitializeManager()

	// Initialize TCP server
	tcpServer := tcp.NewTCPServer("9081", extMgr, version)
	if err := tcpServer.Start(); err != nil {
		log.Printf("Warning: Failed to start TCP server: %v", err)
	}

	return &App{
		templates:      templates,
		nmcliAvailable: nmcliAvailable,
		version:        version,
		startTime:      time.Now(),
		extensionMgr:   extMgr,
		tcpServer:      tcpServer,
	}
}

func (app *App) homeHandler(w http.ResponseWriter, r *http.Request) {
	deviceName := getDeviceName()
	data := TemplateData{
		Title:       "Network Utils - " + deviceName,
		ActiveNav:   "network",
		Version:     app.version,
		PageContent: "network",
		DeviceName:  deviceName,
		Subtitle:    "Network Utils",
		ActivePage:  "index",
		SidebarIcon: "wifi",
	}
	app.templates.ExecuteTemplate(w, "index.html", data)
}

func (app *App) getNmcliStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"available": app.nmcliAvailable})
}

func (app *App) getInterfacesHandler(w http.ResponseWriter, r *http.Request) {
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interfaces)
}

func (app *App) rescanWiFiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !app.nmcliAvailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "nmcli is not installed or not available"})
		return
	}

	// Trigger a rescan in the background so we don't block the request
	go func() {
		cmd := execCommand("nmcli", "device", "wifi", "rescan")
		if err := cmd.Run(); err != nil {
			log.Printf("Background WiFi rescan failed: %v", err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "scanning_started"})
}

func (app *App) getSavedWiFiNetworksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !app.nmcliAvailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "nmcli is not installed or not available"})
		return
	}

	networks, err := getSavedWiFiNetworks(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(networks)
}

func (app *App) getWiFiNetworksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !app.nmcliAvailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "nmcli is not installed or not available"})
		return
	}

	networks, err := listWiFiNetworks(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(networks)
}

func (app *App) connectWiFiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !app.nmcliAvailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "nmcli is not installed or not available"})
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}

	err := connectToWiFi(req.SSID, req.Password, req.Security)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (app *App) getCurrentWiFiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !app.nmcliAvailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "nmcli is not installed or not available"})
		return
	}

	currentWiFi, err := getCurrentWiFi(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(currentWiFi)
}

func (app *App) getVersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": app.version})
}

func (app *App) getSystemHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check network connectivity
	networkCheck := server.CheckNetworkConnectivity()

	// Calculate uptime
	uptime := time.Since(app.startTime)
	uptimeStr := server.FormatUptime(uptime)

	// Determine overall status
	status := "online"
	if !networkCheck {
		status = "degraded"
	}

	health := SystemHealth{
		Status:       status,
		Uptime:       uptimeStr,
		NetworkCheck: networkCheck,
		LastCheck:    time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(health)
}

func (app *App) processesHandler(w http.ResponseWriter, r *http.Request) {
	deviceName := getDeviceName()
	data := TemplateData{
		Title:       "Processes Utils - " + deviceName,
		ActiveNav:   "processes",
		Version:     app.version,
		PageContent: "processes",
		DeviceName:  deviceName,
		Subtitle:    "Processes Utils",
		ActivePage:  "processes",
		SidebarIcon: "activity",
	}
	app.templates.ExecuteTemplate(w, "processes.html", data)
}

func (app *App) systemHandler(w http.ResponseWriter, r *http.Request) {
	deviceName := getDeviceName()
	data := TemplateData{
		Title:       "System Utils - " + deviceName,
		ActiveNav:   "system",
		Version:     app.version,
		PageContent: "system",
		DeviceName:  deviceName,
		Subtitle:    "System Utils",
		ActivePage:  "system",
		SidebarIcon: "settings",
	}
	app.templates.ExecuteTemplate(w, "system.html", data)
}

func (app *App) extensionHandler(w http.ResponseWriter, r *http.Request) {
	deviceName := getDeviceName()
	data := TemplateData{
		Title:       "Extensions Utils - " + deviceName,
		ActiveNav:   "extensions",
		Version:     app.version,
		PageContent: "extensions",
		DeviceName:  deviceName,
		Subtitle:    "Extensions Utils",
		ActivePage:  "extensions",
		SidebarIcon: "cpu",
	}
	app.templates.ExecuteTemplate(w, "extension.html", data)
}

func (app *App) rediscoverExtensionCardsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Stop existing manager cycle if present
	if app.extensionMgr != nil {
		app.extensionMgr.StopCycle()
	}

	// Re-initialize manager (performs auto-discovery and conditionally starts cycle)
	app.extensionMgr = extension.InitializeManager()

	// Return current cards after rediscovery
	cards := app.extensionMgr.RefreshAll()
	json.NewEncoder(w).Encode(map[string]interface{}{"cards": cards})
}

func (app *App) getExtensionCardsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Use GetAllCards() instead of RefreshAll() to avoid duplicate reads
	// The cycle already keeps cards up to date, so we just return cached data
	cards := app.extensionMgr.GetAllCards()
	tcpConnected := app.tcpServer != nil && app.tcpServer.IsConnected()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"cards":        cards,
		"tcpConnected": tcpConnected,
	})
}

func (app *App) extensionCardHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cardID := vars["id"]

	// Check if TCP client is connected - if so, reject write operations
	if app.tcpServer != nil && app.tcpServer.IsConnected() {
		// Only reject write operations, not read operations
		path := r.URL.Path
		if strings.HasSuffix(path, "/write-do") || strings.HasSuffix(path, "/write-ao") ||
			strings.HasSuffix(path, "/write-aotype") || strings.HasSuffix(path, "/reboot") {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "TCP client is connected, frontend controls are disabled",
			})
			return
		}
	}

	_, ok := app.extensionMgr.GetCard(cardID)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "card not found"})
		return
	}

	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/write-do"):
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Index int  `json:"index"`
			State bool `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
			return
		}
		log.Printf("writing DO request received for card=%s index=%d state=%v", cardID, req.Index, req.State)
		if err := app.extensionMgr.QueueWriteDO(cardID, req.Index, req.State); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case strings.HasSuffix(path, "/write-ao"):
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Index int     `json:"index"`
			Value float32 `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
			return
		}
		if err := app.extensionMgr.QueueWriteAO(cardID, req.Index, req.Value); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case strings.HasSuffix(path, "/write-aotype"):
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Index int    `json:"index"`
			Mode  string `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
			return
		}
		if err := app.extensionMgr.QueueWriteAOType(cardID, req.Index, req.Mode); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	case strings.HasSuffix(path, "/reboot"):
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := app.extensionMgr.RebootCard(cardID); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (app *App) getProcessesHandler(w http.ResponseWriter, r *http.Request) {
	processes, err := getProcesses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processes)
}

func (app *App) rebootHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if running on Windows or macOS (development machines)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		log.Printf("Reboot requested on %s (development machine) - logging action instead of rebooting", runtime.GOOS)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "logged",
			"message": fmt.Sprintf("Reboot action logged for %s development machine", runtime.GOOS),
		})
		return
	}

	// For Linux systems, attempt to reboot
	log.Printf("Reboot requested on %s system", runtime.GOOS)

	// Use systemctl if available (systemd systems)
	cmd := execCommand("systemctl", "reboot", "-i")
	if err := cmd.Run(); err != nil {
		// Fallback to reboot command
		cmd = execCommand("reboot")
		if err := cmd.Run(); err != nil {
			// Last resort: shutdown -r now
			cmd = execCommand("shutdown", "-r", "now")
			if err := cmd.Run(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": "Failed to initiate reboot: " + err.Error(),
				})
				return
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "System reboot initiated",
	})
}

func getNetworkInterfaces() ([]NetworkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var result []NetworkInterface
	for _, iface := range interfaces {
		// Skip loopback interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ipAddrs []string
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				// Only include IPv4 addresses
				if ipNet.IP.To4() != nil {
					ipAddrs = append(ipAddrs, ipNet.IP.String())
				}
			}
		}

		// Ensure ipAddrs is never nil - initialize as empty slice if needed
		if ipAddrs == nil {
			ipAddrs = []string{}
		}

		status := "down"
		if iface.Flags&net.FlagUp != 0 {
			status = "up"
		}

		result = append(result, NetworkInterface{
			Name:    iface.Name,
			IPAddrs: ipAddrs,
			Status:  status,
		})
	}

	return result, nil
}

func listWiFiNetworks(ctx context.Context) ([]WiFiNetwork, error) {
	// Only get the list of WiFi networks (assume scan was triggered separately or using cached)
	// We use --rescan no to avoid blocking if possible, although standard list command
	// might still block if daemon is busy.
	// Using CommandContext allows the request to be cancelled if the user navigates away.
	cmd := execCommandContext(ctx, "nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "dev", "wifi", "list")
	output, err := cmd.Output()
	if err != nil {
		// If context was cancelled, return that error
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to list WiFi networks with nmcli: %v", err)
	}

	return parseNmcliOutput(string(output)), nil
}

func getSavedWiFiNetworks(ctx context.Context) ([]SavedNetwork, error) {
	cmd := execCommandContext(ctx, "nmcli", "-t", "-f", "NAME,UUID,TYPE,DEVICE,ACTIVE", "connection", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get saved networks: %v", err)
	}

	return parseNmcliSavedOutput(string(output)), nil
}

func parseNmcliSavedOutput(output string) []SavedNetwork {
	var networks []SavedNetwork
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// NAME:UUID:TYPE:DEVICE:ACTIVE
		parts := strings.Split(line, ":")
		if len(parts) >= 5 {
			name := parts[0]
			uuid := parts[1]
			connType := parts[2]
			device := parts[3]
			active := parts[4]

			if connType == "802-11-wireless" {
				networks = append(networks, SavedNetwork{
					SSID:   name,
					UUID:   uuid,
					Active: active == "yes",
					Device: device,
				})
			}
		}
	}
	return networks
}

func parseNmcliOutput(output string) []WiFiNetwork {
	var networks []WiFiNetwork
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// nmcli -t output format: SSID:SIGNAL:SECURITY
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			ssid := parts[0]
			signal := parts[1]
			security := parts[2]

			// Skip empty SSIDs
			if ssid == "" || ssid == "--" {
				continue
			}

			// Normalize security type
			normalizedSecurity := normalizeSecurityType(security)

			networks = append(networks, WiFiNetwork{
				SSID:     ssid,
				Signal:   signal + "%",
				Security: normalizedSecurity,
			})
		}
	}

	return networks
}

func normalizeSecurityType(security string) string {
	security = strings.ToUpper(security)
	if strings.Contains(security, "WPA3") {
		return "WPA3"
	} else if strings.Contains(security, "WPA2") {
		return "WPA2"
	} else if strings.Contains(security, "WPA") {
		return "WPA"
	} else if strings.Contains(security, "WEP") {
		return "WEP"
	} else if security == "" || security == "--" {
		return "Open"
	}
	return "Unknown"
}

func getCurrentWiFi(ctx context.Context) (*CurrentWiFi, error) {
	// Get current WiFi connection using nmcli
	// Using CommandContext to allow cancellation
	cmd := execCommandContext(ctx, "nmcli", "-t", "-f", "ACTIVE,SSID,SIGNAL,SECURITY", "dev", "wifi")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return &CurrentWiFi{Connected: false}, nil
	}

	return parseNmcliCurrentOutput(string(output)), nil
}

func parseNmcliCurrentOutput(output string) *CurrentWiFi {
	lines := strings.Split(output, "\n")
	var currentWiFi CurrentWiFi

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// nmcli -t output format: ACTIVE:SSID:SIGNAL:SECURITY
		parts := strings.Split(line, ":")
		if len(parts) >= 4 {
			active := parts[0]
			ssid := parts[1]
			signal := parts[2]
			security := parts[3]

			// Check if this is an active connection
			if active == "yes" && ssid != "" && ssid != "--" {
				currentWiFi.SSID = ssid
				currentWiFi.Signal = signal + "%"
				currentWiFi.Security = normalizeSecurityType(security)
				currentWiFi.Connected = true
				break
			}
		}
	}

	return &currentWiFi
}

func connectToWiFi(ssid, password, security string) error {
	var cmd *exec.Cmd

	switch security {
	case "Open":
		// Connect to open network
		cmd = execCommand("nmcli", "dev", "wifi", "connect", ssid)
	case "WEP":
		// Connect to WEP network
		cmd = execCommand("nmcli", "dev", "wifi", "connect", ssid, "password", password)
	case "WPA", "WPA2", "WPA3":
		// Connect to WPA network
		cmd = execCommand("nmcli", "dev", "wifi", "connect", ssid, "password", password)
	default:
		return fmt.Errorf("unsupported security type: %s", security)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to connect to WiFi network %s: %v (output: %s)", ssid, err, string(output))
	}

	return nil
}

func getProcesses() ([]Process, error) {
	// Use ps command to get process information
	// This is the most reliable cross-platform way to get process info
	cmd := execCommand("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to ps -ef if ps aux fails
		cmd = execCommand("ps", "-ef")
		output, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get process list: %v", err)
		}
		return parsePsEfOutput(string(output))
	}

	return parsePsAuxOutput(string(output))
}

func parsePsAuxOutput(output string) ([]Process, error) {
	var processes []Process
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // Skip header line and empty lines
		}

		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		// ps aux format: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
		user := fields[0]
		pidStr := fields[1]
		cpu := fields[2]
		mem := fields[3]
		status := fields[7]
		command := strings.Join(fields[10:], " ")

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Extract process name from command
		name := command
		if spaceIndex := strings.Index(command, " "); spaceIndex > 0 {
			name = command[:spaceIndex]
		}

		processes = append(processes, Process{
			PID:     pid,
			Name:    name,
			Status:  status,
			CPU:     cpu + "%",
			Memory:  mem + "%",
			User:    user,
			Command: command,
		})
	}

	return processes, nil
}

func parsePsEfOutput(output string) ([]Process, error) {
	var processes []Process
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // Skip header line and empty lines
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		// ps -ef format: UID PID PPID C STIME TTY TIME CMD
		user := fields[0]
		pidStr := fields[1]
		command := strings.Join(fields[7:], " ")

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Extract process name from command
		name := command
		if spaceIndex := strings.Index(command, " "); spaceIndex > 0 {
			name = command[:spaceIndex]
		}

		processes = append(processes, Process{
			PID:     pid,
			Name:    name,
			Status:  "R", // Running (default for ps -ef)
			CPU:     "0%",
			Memory:  "0%",
			User:    user,
			Command: command,
		})
	}

	return processes, nil
}

func main() {
	// Set process title for better identification in process lists
	os.Args[0] = "cm-utils"

	// Start discovery agent
	discovery.Start()

	app := NewApp()

	r := mux.NewRouter()

	// Static files from embedded filesystem
	staticSubFS, _ := fs.Sub(staticFS, "build/static")
	staticHandler := http.FileServer(http.FS(staticSubFS))
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", staticHandler))

	// Routes
	r.HandleFunc("/", app.homeHandler).Methods("GET")
	r.HandleFunc("/processes", app.processesHandler).Methods("GET")
	r.HandleFunc("/system", app.systemHandler).Methods("GET")
	r.HandleFunc("/extensions", app.extensionHandler).Methods("GET")
	r.HandleFunc("/api/version", app.getVersionHandler).Methods("GET")
	r.HandleFunc("/api/health", app.getSystemHealthHandler).Methods("GET")
	r.HandleFunc("/api/nmcli/status", app.getNmcliStatusHandler).Methods("GET")
	r.HandleFunc("/api/interfaces", app.getInterfacesHandler).Methods("GET")
	r.HandleFunc("/api/wifi/saved", app.getSavedWiFiNetworksHandler).Methods("GET")
	r.HandleFunc("/api/wifi/scan", app.getWiFiNetworksHandler).Methods("GET")
	r.HandleFunc("/api/wifi/rescan", app.rescanWiFiHandler).Methods("POST")
	r.HandleFunc("/api/wifi/current", app.getCurrentWiFiHandler).Methods("GET")
	r.HandleFunc("/api/wifi/connect", app.connectWiFiHandler).Methods("POST")
	r.HandleFunc("/api/processes", app.getProcessesHandler).Methods("GET")
	r.HandleFunc("/api/system/reboot", app.rebootHandler).Methods("POST")
	r.HandleFunc("/api/extension/cards", app.getExtensionCardsHandler).Methods("GET")
	r.HandleFunc("/api/extension/cards/rediscover", app.rediscoverExtensionCardsHandler).Methods("POST")
	r.HandleFunc("/api/extension/cards/{id}/write-do", app.extensionCardHandler).Methods("POST")
	r.HandleFunc("/api/extension/cards/{id}/write-ao", app.extensionCardHandler).Methods("POST")
	r.HandleFunc("/api/extension/cards/{id}/write-aotype", app.extensionCardHandler).Methods("POST")
	r.HandleFunc("/api/extension/cards/{id}/reboot", app.extensionCardHandler).Methods("POST")

	fmt.Println("ControlMate Utils starting on :9080")
	log.Fatal(http.ListenAndServe(":9080", r))
}
