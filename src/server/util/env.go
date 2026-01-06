package util

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvLocal reads a value from .env.local file by key
// Returns the value if found, empty string otherwise
func LoadEnvLocal(key string) string {
	file, err := os.Open(".env.local")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if k == key {
				return strings.Trim(value, "\"'")
			}
		}
	}
	return ""
}
