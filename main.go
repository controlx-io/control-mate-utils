package main

import (
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

	"github.com/gorilla/mux"
)

//go:embed src/templates/*
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

type App struct {
	templates      *template.Template
	nmcliAvailable bool
	version        string
	startTime      time.Time
}

var nmcliAvailable bool

func checkNmcliAvailable() bool {
	cmd := exec.Command("which", "nmcli")
	err := cmd.Run()
	return err == nil
}

func readVersion() string {
	data, err := staticFS.ReadFile("build/static/version.txt")
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(string(data))
}

func NewApp() *App {
	templates := template.Must(template.ParseFS(templateFS, "src/templates/*.html"))
	nmcliAvailable = checkNmcliAvailable()
	version := readVersion()
	return &App{
		templates:      templates,
		nmcliAvailable: nmcliAvailable,
		version:        version,
		startTime:      time.Now(),
	}
}

func (app *App) homeHandler(w http.ResponseWriter, r *http.Request) {
	app.templates.ExecuteTemplate(w, "index.html", nil)
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

func (app *App) getWiFiNetworksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !app.nmcliAvailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "nmcli is not installed or not available"})
		return
	}

	networks, err := scanWiFiNetworks()
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

	currentWiFi, err := getCurrentWiFi()
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
	networkCheck := checkNetworkConnectivity()

	// Calculate uptime
	uptime := time.Since(app.startTime)
	uptimeStr := formatUptime(uptime)

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
	app.templates.ExecuteTemplate(w, "processes.html", nil)
}

func (app *App) systemHandler(w http.ResponseWriter, r *http.Request) {
	app.templates.ExecuteTemplate(w, "system.html", nil)
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
	cmd := exec.Command("systemctl", "reboot")
	if err := cmd.Run(); err != nil {
		// Fallback to reboot command
		cmd = exec.Command("reboot")
		if err := cmd.Run(); err != nil {
			// Last resort: shutdown -r now
			cmd = exec.Command("shutdown", "-r", "now")
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

func scanWiFiNetworks() ([]WiFiNetwork, error) {
	// First, trigger a rescan to refresh the WiFi network list
	rescanCmd := exec.Command("nmcli", "device", "wifi", "rescan")
	if err := rescanCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to rescan WiFi networks: %v", err)
	}

	// Wait 5 seconds for the rescan to complete
	time.Sleep(5 * time.Second)

	// Now get the updated list of WiFi networks
	cmd := exec.Command("nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY", "dev", "wifi", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to scan WiFi networks with nmcli: %v", err)
	}

	return parseNmcliOutput(string(output)), nil
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

func getCurrentWiFi() (*CurrentWiFi, error) {
	// Get current WiFi connection using nmcli
	cmd := exec.Command("nmcli", "-t", "-f", "ACTIVE,SSID,SIGNAL,SECURITY", "dev", "wifi")
	output, err := cmd.Output()
	if err != nil {
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
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid)
	case "WEP":
		// Connect to WEP network
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid, "password", password)
	case "WPA", "WPA2", "WPA3":
		// Connect to WPA network
		cmd = exec.Command("nmcli", "dev", "wifi", "connect", ssid, "password", password)
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
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to ps -ef if ps aux fails
		cmd = exec.Command("ps", "-ef")
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

func checkNetworkConnectivity() bool {
	// Try to connect to a reliable external service with a short timeout
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func formatUptime(duration time.Duration) string {
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

func main() {
	// Set process title for better identification in process lists
	os.Args[0] = "cm-utils"

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
	r.HandleFunc("/api/version", app.getVersionHandler).Methods("GET")
	r.HandleFunc("/api/health", app.getSystemHealthHandler).Methods("GET")
	r.HandleFunc("/api/nmcli/status", app.getNmcliStatusHandler).Methods("GET")
	r.HandleFunc("/api/interfaces", app.getInterfacesHandler).Methods("GET")
	r.HandleFunc("/api/wifi/scan", app.getWiFiNetworksHandler).Methods("GET")
	r.HandleFunc("/api/wifi/current", app.getCurrentWiFiHandler).Methods("GET")
	r.HandleFunc("/api/wifi/connect", app.connectWiFiHandler).Methods("POST")
	r.HandleFunc("/api/processes", app.getProcessesHandler).Methods("GET")
	r.HandleFunc("/api/system/reboot", app.rebootHandler).Methods("POST")

	fmt.Println("ControlMate Utils starting on :9080")
	log.Fatal(http.ListenAndServe(":9080", r))
}
