package framework

import (
	"os"
	"encoding/json"
	"path/filepath"
)

type Config struct {
	ModuleID  string
	BusSocket string
	StateDir  string
	LogLevel  string
}

func LoadConfig() Config {
	return Config{
		ModuleID:  getEnv("MODULE_ID", "unknown"),
		BusSocket: getEnv("BUS_SOCKET", "/tmp/bus.sock"),
		StateDir:  getEnv("STATE_DIR", "./state"),
		LogLevel:  getEnv("LOG_LEVEL", "INFO"),
	}
}

func LoadModuleConfig(stateDir string) (map[string]any, error) {
	path := filepath.Join(stateDir, "config.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return make(map[string]any), nil
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
