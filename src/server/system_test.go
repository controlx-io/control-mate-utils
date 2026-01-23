package server

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestHelperProcess isn't a real test. It's used to mock exec.Command
// For more info: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
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
	case "which":
		if len(args) > 0 && args[0] == "nmcli" {
			// Simulate nmcli found
			os.Exit(0)
		}
		os.Exit(1)
	case "nmcli":
		// Handle nmcli commands
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

func TestCheckNmcliAvailable(t *testing.T) {
	oldExec := execCommand
	defer func() { execCommand = oldExec }()
	execCommand = fakeExecCommand

	if !CheckNmcliAvailable() {
		t.Error("Expected CheckNmcliAvailable to return true")
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{5 * time.Minute, "5m"},
		{2*time.Hour + 30*time.Minute, "2h 30m"},
		{25*time.Hour + 10*time.Minute, "1d 1h 10m"},
		{48 * time.Hour, "2d 0h 0m"},
		{30 * time.Second, "0m"}, // Integer division results in 0m
	}

	for _, tt := range tests {
		result := FormatUptime(tt.duration)
		if result != tt.expected {
			t.Errorf("FormatUptime(%v) = %s; want %s", tt.duration, result, tt.expected)
		}
	}
}

// CheckNetworkConnectivity is hard to mock without refactoring net.Dial
// For now, we skip it or accept it hits real network (which is bad for unit tests)
// Use Integration test tag or similar if we wanted to include it.
