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

// LifecycleHandler is the interface bundles must implement.
type LifecycleHandler interface {
	ValidateConfig(ctx context.Context, config map[string]any) error
	Init(api ModuleAPI) error
	Stop() error
}

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

func Run(handler LifecycleHandler) {
	cfg := LoadRunnerConfig()
	if cfg.ModuleID == "" || cfg.StateDir == "" {
		log.Fatalf("MODULE_ID and STATE_DIR must be set")
	}

	os.MkdirAll(cfg.StateDir, 0755)

	modConfig := make(map[string]any)
	cfgPath := filepath.Join(cfg.StateDir, "config.json")
	if data, err := os.ReadFile(cfgPath); err == nil {
		json.Unmarshal(data, &modConfig)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer func() {
		handler.Stop()
		cancel()
	}()

	base := NewBaseModule(ctx, cfg.ModuleID, cfg.StateDir, cfg.BusSocket, modConfig)
	if err := base.Start(); err != nil {
		log.Fatalf("Failed to start base module: %v", err)
	}

	go func() {
		if len(modConfig) == 0 {
			base.SetBundleStatus(BundleStatus{State: StateIdling, Message: "Waiting for configuration"})
		} else {
			base.SetBundleStatus(BundleStatus{State: StateReady, Message: "Initialized with saved config", Config: modConfig})
		}

		topic := "commands/" + cfg.ModuleID
		ch := base.Subscribe(topic)
		for {
			select {
			case <-ctx.Done(): return
			case ev := <-ch:
				log.Printf("[%s] Runner received command: %s", cfg.ModuleID, ev.Type)
				switch ev.Type {
				case "set_config":
					newCfg, _ := ev.Data["config"].(map[string]any)
					base.SetBundleStatus(BundleStatus{State: StateValidating, Message: "Validating..."})
					if err := handler.ValidateConfig(ctx, newCfg); err != nil {
						base.SetBundleStatus(BundleStatus{State: StateError, Message: err.Error()})
					} else {
						data, _ := json.MarshalIndent(newCfg, "", "  ")
						os.WriteFile(cfgPath, data, 0644)
						base.modConfig = newCfg
						base.SetBundleStatus(BundleStatus{State: StateReady, Message: "Verified", Config: newCfg})
					}
				case "execute_init":
					base.Info("Triggering managed initialization...")
					if err := handler.Init(base); err != nil {
						base.SetBundleStatus(BundleStatus{State: StateError, Message: "Init failed: " + err.Error()})
					}
				case "get_instances":
					base.Publish("sys/instances_response", "instances", map[string]any{
						"bundle": cfg.ModuleID, "instances": base.GetInstances(),
					})
				case "set_alias":
					id, _ := ev.Data["id"].(string)
					alias, _ := ev.Data["alias"].(string)
					if id != "" {
						insts, _ := base.im.GetInstances()
						for _, inst := range insts {
							if inst.ID == id {
								inst.Alias = alias
								base.RegisterInstance(inst) // Re-register with new alias
								break
							}
						}
					}
				}
			}
		}
	}()

	<-ctx.Done()
	log.Printf("Module %s shutting down.", cfg.ModuleID)
}
