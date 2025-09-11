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
	"strings"

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

type ConnectionRequest struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
	Security string `json:"security"`
}

type App struct {
	templates      *template.Template
	nmcliAvailable bool
}

var nmcliAvailable bool

func checkNmcliAvailable() bool {
	cmd := exec.Command("which", "nmcli")
	err := cmd.Run()
	return err == nil
}

func NewApp() *App {
	templates := template.Must(template.ParseFS(templateFS, "src/templates/*.html"))
	nmcliAvailable = checkNmcliAvailable()
	return &App{templates: templates, nmcliAvailable: nmcliAvailable}
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
	if !app.nmcliAvailable {
		http.Error(w, "nmcli is not installed or not available", http.StatusServiceUnavailable)
		return
	}

	networks, err := scanWiFiNetworks()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networks)
}

func (app *App) connectWiFiHandler(w http.ResponseWriter, r *http.Request) {
	if !app.nmcliAvailable {
		http.Error(w, "nmcli is not installed or not available", http.StatusServiceUnavailable)
		return
	}

	var req ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err := connectToWiFi(req.SSID, req.Password, req.Security)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (app *App) getCurrentWiFiHandler(w http.ResponseWriter, r *http.Request) {
	if !app.nmcliAvailable {
		http.Error(w, "nmcli is not installed or not available", http.StatusServiceUnavailable)
		return
	}

	currentWiFi, err := getCurrentWiFi()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(currentWiFi)
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
	r.HandleFunc("/api/nmcli/status", app.getNmcliStatusHandler).Methods("GET")
	r.HandleFunc("/api/interfaces", app.getInterfacesHandler).Methods("GET")
	r.HandleFunc("/api/wifi/scan", app.getWiFiNetworksHandler).Methods("GET")
	r.HandleFunc("/api/wifi/current", app.getCurrentWiFiHandler).Methods("GET")
	r.HandleFunc("/api/wifi/connect", app.connectWiFiHandler).Methods("POST")

	fmt.Println("Control Mate Utils starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
