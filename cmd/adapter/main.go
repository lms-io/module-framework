package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"module/internal/framework"
	"module/internal/logic"
)

func main() {
	cfg := framework.LoadConfig()
	log.Printf("Starting module %s...", cfg.ModuleID)

	os.MkdirAll(cfg.StateDir, 0755)
	logCloser, err := framework.SetupLogger(cfg.StateDir)
	if err == nil { defer logCloser.Close() }

	db, err := framework.ConnectDB(cfg.StateDir)
	if err != nil { log.Fatalf("Database error: %v", err) }
	defer db.Close()

	im := framework.NewInstanceManager(cfg.StateDir, cfg.ModuleID)
	modCfg, err := framework.LoadModuleConfig(cfg.StateDir)
	if err != nil { log.Printf("Warning: Failed to load config.json: %v", err) }

	bus, err := framework.NewBusClient(cfg.BusSocket, cfg.ModuleID, cfg.LogLevel)
	if err != nil { log.Fatalf("Fatal: %v", err) }

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	api := &moduleAPIWrapper{
		BusClient: bus,
		db:        db,
		im:        im,
		ctx:       ctx,
		config:    modCfg,
		stateDir:  cfg.StateDir,
	}

	if err := framework.RunSetup(cfg.StateDir, api); err != nil {
		log.Printf("Setup Error: %v", err)
	}

	log.Println("Initializing logic...")
	for _, inst := range api.GetInstances() {
		go setupQueryHandler(api, inst.ID)
	}
	
	// Built-in Bundle Command Handler (Config, Instances, etc)
	go setupBundleCommandHandler(api)

	logicDone := make(chan struct{})
	go func() {
		logic.Start(api)
		close(logicDone)
	}()

	<-ctx.Done()
	select {
	case <-logicDone:
		log.Println("Logic exited cleanly.")
	case <-time.After(2 * time.Second):
		log.Println("Logic timed out during shutdown.")
	}
	bus.Close()
}

func setupBundleCommandHandler(api *moduleAPIWrapper) {
	topic := fmt.Sprintf("commands/%s", api.im.ModuleID())
	ch := api.Subscribe(topic)
	for {
		select {
		case <-api.Context().Done(): return
		case ev := <-ch:
			switch ev.Type {
			case "get_config":
				config := api.GetModuleConfig()
				if config == nil { config = make(map[string]any) }
				api.Publish("sys/config_response", "config", map[string]any{
					"bundle": api.im.ModuleID(),
					"config": config,
				})
			case "set_config":
				config, _ := ev.Data["config"].(map[string]any)
				if config != nil {
					for k, v := range config { api.Set(k, v) }
					cfgPath := filepath.Join(api.stateDir, "config.json")
					data, _ := json.MarshalIndent(config, "", "  ")
					os.WriteFile(cfgPath, data, 0644)
					log.Printf("[Framework] Configuration updated and saved.")
				}
			case "get_instances":
				instances := api.GetInstances()
				api.Publish("sys/instances_response", "instances", map[string]any{
					"bundle": api.im.ModuleID(),
					"instances": instances,
				})
			case "create_instance":
				config, _ := ev.Data["config"].(map[string]any)
				if config != nil {
					id, err := api.RegisterInstance(config)
					if err == nil {
						log.Printf("[Framework] New instance created: %s", id)
					} else {
						log.Printf("[Framework] Failed to create instance: %v", err)
					}
				}
			case "delete_instance":
				id, _ := ev.Data["id"].(string)
				if id != "" {
					api.im.DeleteInstance(id)
					log.Printf("[Framework] Instance deleted: %s", id)
				}
			}
		}
	}
}

func setupQueryHandler(api *moduleAPIWrapper, id string) {
	topic := fmt.Sprintf("commands/%s", id)
	ch := api.Subscribe(topic)
	for {
		select {
		case <-api.Context().Done(): return
		case ev := <-ch:
			if ev.Type == "query_state" {
				replyTo, _ := ev.Data["reply_to"].(string)
				if replyTo == "" { continue }
				state, err := api.im.GetState(id)
				if err != nil { state = make(map[string]any) }
				api.Publish(replyTo, "state_response", state)
			}
		}
	}
}

type moduleAPIWrapper struct {
	*framework.BusClient
	db  *sql.DB
	im  *framework.InstanceManager
	ctx context.Context
	config map[string]any
	stateDir string
}

func (w *moduleAPIWrapper) DB() *sql.DB { return w.db }
func (w *moduleAPIWrapper) GetModuleConfig() map[string]any { return w.config }
func (w *moduleAPIWrapper) GetInstances() []framework.InstanceConfig {
	inst, _ := w.im.GetInstances()
	return inst
}
func (w *moduleAPIWrapper) RegisterInstance(config map[string]any) (string, error) {
	meta, _ := config["meta"].(map[string]any)
	id, err := w.im.RegisterInstance(config)
	if err == nil {
		w.BusClient.Publish("sys/register", "register", map[string]any{
			"id": id, "bundle": w.im.ModuleID(), "meta": meta,
		})
		go setupQueryHandler(w, id)
	}
	return id, err
}
func (w *moduleAPIWrapper) UpdateState(id string, state map[string]any) error { return w.im.UpdateState(id, state) }
func (w *moduleAPIWrapper) DeleteInstance(id string) error { return w.im.DeleteInstance(id) }
func (w *moduleAPIWrapper) ExecInstance(id string, f string, a ...any) (any, error) { return w.im.ExecInstance(id, f, a...) }
func (w *moduleAPIWrapper) Context() context.Context { return w.ctx }
func (w *moduleAPIWrapper) Set(key string, value any) {
	if w.config == nil { w.config = make(map[string]any) }
	w.config[key] = value
}
