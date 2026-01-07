package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig(t *testing.T) {
	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "cm-utils-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set env var to point to temp dir
	os.Setenv("CM_UTILS_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("CM_UTILS_CONFIG_DIR")

	// Force reload config (since init() already ran with defaults)
	// We need to call loadConfig() again, but it's private and called by init().
	// But getConfigPath() checks env var.
	// And loadConfig is called by init.
	// Since init() runs once, we can't re-run it.
	// But we can call loadConfig() via some exported method or just rely on global state?
	// loadConfig is private.
	// We can't easily reset cfgOnce.
	// However, we can use reflection or just test the exported methods if they are sufficient.
	// But GetDeviceID() returns the cached cfg.

	// Wait, we can call internal functions in test if in same package.
	// loadConfig() is available.

	err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	deviceID := GetDeviceID()
	if deviceID == "" {
		t.Error("Expected DeviceID to be generated")
	}

	// Verify file exists
	path := filepath.Join(tmpDir, "config.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Test persistence
	// Modify struct
	cfgMu.Lock()
	originalID := cfg.DeviceID
	cfg.DeviceID = "new-id"
	cfgMu.Unlock()

	// Save
	err = saveConfigLocked(path)
	if err != nil {
		t.Fatalf("saveConfigLocked failed: %v", err)
	}

	// Reload
	cfgMu.Lock()
	cfg.DeviceID = "" // Clear memory
	cfgMu.Unlock()

	err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig reload failed: %v", err)
	}

	if GetDeviceID() != "new-id" {
		t.Errorf("Expected persisted ID new-id, got %s", GetDeviceID())
	}

	// Restore ID for other tests if any
	cfgMu.Lock()
	cfg.DeviceID = originalID
	cfgMu.Unlock()
}

func TestConfigType(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cm-utils-test-type")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	os.Setenv("CM_UTILS_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("CM_UTILS_CONFIG_DIR")

	// Load config - Type should be empty by default
	err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if GetConfig().Type != "" {
		t.Errorf("Expected Type to be empty by default, got %s", GetConfig().Type)
	}

	// Set Type and save
	path := filepath.Join(tmpDir, "config.yaml")
	cfgMu.Lock()
	cfg.Type = "server"
	cfgMu.Unlock()

	err = saveConfigLocked(path)
	if err != nil {
		t.Fatalf("saveConfigLocked failed: %v", err)
	}

	// Clear and reload
	cfgMu.Lock()
	cfg.Type = ""
	cfgMu.Unlock()

	err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig reload failed: %v", err)
	}

	if GetConfig().Type != "server" {
		t.Errorf("Expected Type 'server', got %s", GetConfig().Type)
	}

	// Cleanup
	cfgMu.Lock()
	cfg.Type = ""
	cfgMu.Unlock()
}
