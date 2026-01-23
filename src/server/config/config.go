package config

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	prodConfigDir  = "/var/lib/cm-utils"
	configFileName = "config.yaml"
)

type Config struct {
	DeviceID        string `yaml:"device_id"`
	Type            string `yaml:"type,omitempty"`
	ServeExternally bool   `yaml:"serve_externally,omitempty"`
}

var (
	cfg     Config
	cfgOnce sync.Once
	cfgMu   sync.RWMutex
)

func init() {
	cfgOnce.Do(func() {
		if err := loadConfig(); err != nil {
			log.Printf("Config: failed to load, using generated values: %v", err)
		}
	})
}

func GetConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg
}

func GetDeviceID() string {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cfg.DeviceID
}

func getConfigPath() string {
	if dir := os.Getenv("CM_UTILS_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, configFileName)
	}
	if info, err := os.Stat(prodConfigDir); err == nil && info.IsDir() {
		testFile := filepath.Join(prodConfigDir, ".write_test")
		if f, err := os.Create(testFile); err == nil {
			f.Close()
			os.Remove(testFile)
			return filepath.Join(prodConfigDir, configFileName)
		}
	}
	return filepath.Join("tmp", configFileName)
}

func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, uuid); err != nil {
		return "", err
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

func loadConfig() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()

	path := getConfigPath()
	fmt.Println("Config:", path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return createDefaultConfig(path)
		}
		return err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if cfg.DeviceID == "" {
		uuid, err := generateUUID()
		if err != nil {
			return err
		}
		cfg.DeviceID = uuid
		return saveConfigLocked(path)
	}

	return nil
}

func createDefaultConfig(path string) error {
	uuid, err := generateUUID()
	if err != nil {
		return err
	}
	cfg.DeviceID = uuid
	return saveConfigLocked(path)
}

func saveConfigLocked(path string) error {
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
