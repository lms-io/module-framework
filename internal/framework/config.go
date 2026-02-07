package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ModuleConfig struct {
	ModuleID  string
	BusSocket string
	StateDir  string
	LogLevel  string
}

func LoadConfig() ModuleConfig {
	busSocket := getEnv("BUS_SOCKET", "bus.sock")
	stateDir := getEnv("STATE_DIR", "state")
	moduleID := getEnv("MODULE_ID", "template-module")
	logLevel := getEnv("LOG_LEVEL", "INFO")

	return ModuleConfig{
		ModuleID:  moduleID,
		BusSocket: busSocket,
		StateDir:  stateDir,
		LogLevel:  logLevel,
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

func (c ModuleConfig) Info() string {
	return fmt.Sprintf("ID=%s, Bus=%s, State=%s", c.ModuleID, c.BusSocket, c.StateDir)
}