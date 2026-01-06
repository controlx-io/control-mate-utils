package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Reuse TestHelperProcess pattern for main package
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(0)
	}

	cmd, args := args[0], args[1:]
	switch cmd {
	case "nmcli":
		// Handle various nmcli commands
		if len(args) > 0 {
			if args[0] == "-t" {
				// Reading/Listing
				if strings.Contains(strings.Join(args, " "), "wifi list") {
					os.Stdout.WriteString("TestWifi:80:WPA2\n")
					os.Exit(0)
				}
				if strings.Contains(strings.Join(args, " "), "connection show") {
					os.Stdout.WriteString("TestSaved:uuid-123:802-11-wireless:wlan0:yes\n")
					os.Exit(0)
				}
				if strings.Contains(strings.Join(args, " "), "dev wifi") { // current
					os.Stdout.WriteString("yes:TestWifi:80:WPA2\n")
					os.Exit(0)
				}
			}
			if args[0] == "device" && args[1] == "wifi" && args[2] == "rescan" {
				os.Exit(0)
			}
			if args[0] == "dev" && args[1] == "wifi" && args[2] == "connect" {
				os.Exit(0)
			}
		}
	case "ps":
		// Mock ps aux
		os.Stdout.WriteString("USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\n")
		os.Stdout.WriteString("root 1 0.0 0.1 100 100 ? Ss 00:00 0:01 /sbin/init\n")
		os.Exit(0)
	case "systemctl":
		os.Exit(0)
	}
	os.Exit(0)
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestHandlers(t *testing.T) {
	// Mock exec
	oldExec := execCommand
	oldExecCtx := execCommandContext
	defer func() {
		execCommand = oldExec
		execCommandContext = oldExecCtx
	}()
	execCommand = fakeExecCommand
	execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return fakeExecCommand(name, arg...)
	}

	// Setup app
	app := NewApp()
	app.nmcliAvailable = true // Force true for testing handlers

	// Test Health
	t.Run("Health", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/health", nil)
		rr := httptest.NewRecorder()
		app.getSystemHealthHandler(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Health handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
	})

	// Test WiFi List
	t.Run("WiFi List", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/wifi/scan", nil)
		rr := httptest.NewRecorder()
		app.getWiFiNetworksHandler(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("WiFi scan handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
		// Check body
		var networks []WiFiNetwork
		if err := json.NewDecoder(rr.Body).Decode(&networks); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if len(networks) == 0 {
			t.Error("Expected networks")
		} else if networks[0].SSID != "TestWifi" {
			t.Errorf("Expected SSID TestWifi, got %s", networks[0].SSID)
		}
	})

	// Test Processes
	t.Run("Processes", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/processes", nil)
		rr := httptest.NewRecorder()
		app.getProcessesHandler(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Processes handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
		}
		var processes []Process
		if err := json.NewDecoder(rr.Body).Decode(&processes); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		if len(processes) == 0 {
			t.Error("Expected processes")
		}
	})
}

