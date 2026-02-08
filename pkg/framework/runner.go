package framework

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// RunnerConfig holds the basic environment variables needed to start.
type RunnerConfig struct {
	ModuleID  string
	StateDir  string
	BusSocket string
}

func LoadRunnerConfig() RunnerConfig {
	return RunnerConfig{
		ModuleID:  os.Getenv("MODULE_ID"),
		StateDir:  os.Getenv("STATE_DIR"),
		BusSocket: os.Getenv("BUS_SOCKET"),
	}
}

// Run is the standard entry point for all modules.
func Run(logicFunc func(ModuleAPI)) {
	cfg := LoadRunnerConfig()
	if cfg.ModuleID == "" || cfg.StateDir == "" {
		log.Fatalf("MODULE_ID and STATE_DIR must be set")
	}

	os.MkdirAll(cfg.StateDir, 0755)

	// Load config.json if it exists
	modConfig := make(map[string]any)
	cfgPath := filepath.Join(cfg.StateDir, "config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(data, &modConfig)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	base := NewBaseModule(ctx, cfg.ModuleID, cfg.StateDir, cfg.BusSocket, modConfig)
	if err := base.Start(); err != nil {
		log.Fatalf("Failed to start base module: %v", err)
	}

	// Internal system command handler
	go func() {
		topic := "commands/" + cfg.ModuleID
		ch := base.Subscribe(topic)
		for {
			select {
			case <-ctx.Done(): return
			case ev := <-ch:
				if ev.Type == "get_instances" {
					base.Publish("sys/instances_response", "instances", map[string]any{
						"bundle":    cfg.ModuleID,
						"instances": base.GetInstances(),
					})
				}
			}
		}
	}()

	log.Printf("Module %s starting logic...", cfg.ModuleID)
	logicFunc(base)

	<-ctx.Done()
	log.Printf("Module %s shutting down.", cfg.ModuleID)
}
