package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"module/internal/framework"
	"module/internal/logic"
)

func main() {
	cfg := framework.LoadConfig()
	log.Printf("Starting module %s...", cfg.ModuleID)

	// Ensure state dir exists
	os.MkdirAll(cfg.StateDir, 0755)

	// Initialize Database
	db, err := framework.ConnectDB(cfg.StateDir)
	if err != nil {
		log.Fatalf("Database error: %v", err)
	}
	defer db.Close()

	// Initialize Instance Manager
	im := framework.NewInstanceManager(cfg.StateDir, cfg.ModuleID)

	// Load Global Module Config
	modCfg, err := framework.LoadModuleConfig(cfg.StateDir)
	if err != nil {
		log.Printf("Warning: Failed to load config.json: %v", err)
	}

	bus, err := framework.NewBusClient(cfg.BusSocket, cfg.ModuleID, cfg.LogLevel)
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Wrapper to satisfy ModuleAPI
	api := &moduleAPIWrapper{
		BusClient: bus,
		db:        db,
		im:        im,
		ctx:       ctx,
		config:    modCfg,
	}

	// Start the developer's logic
	log.Println("Initializing logic...")
	
	// Setup automatic query handlers for existing instances
	for _, inst := range api.GetInstances() {
		go setupQueryHandler(api, inst.ID)
	}

	// We use a channel to wait for the logic to finish if it chooses to block
	// or we just wait for the context to be done.
	logicDone := make(chan struct{})
	go func() {
		logic.Start(api)
		close(logicDone)
	}()

	<-ctx.Done()
	log.Println("Module received shutdown signal, waiting for logic to clean up...")
	
	// Give the logic a chance to finish its loop
	select {
	case <-logicDone:
		log.Println("Logic exited cleanly.")
	case <-time.After(2 * time.Second):
		log.Println("Logic timed out during shutdown, forcing exit.")
	}

	bus.Close()
	log.Println("Module shut down.")
}

func setupQueryHandler(api *moduleAPIWrapper, id string) {
	topic := fmt.Sprintf("commands/%s", id)
	ch := api.Subscribe(topic)
	
	for {
		select {
		case <-api.Context().Done():
			return
		case ev := <-ch:
			if ev.Type == "query_state" {
				replyTo, _ := ev.Data["reply_to"].(string)
				if replyTo == "" {
					continue
				}
				
				state, err := api.im.GetState(id)
				if err != nil {
					// Fallback to empty if not found
					state = make(map[string]any)
				}
				
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
}

func (w *moduleAPIWrapper) DB() *sql.DB {
	return w.db
}

func (w *moduleAPIWrapper) GetModuleConfig() map[string]any {
	return w.config
}

func (w *moduleAPIWrapper) GetInstances() []framework.InstanceConfig {
	inst, _ := w.im.GetInstances()
	return inst
}

func (w *moduleAPIWrapper) RegisterInstance(config map[string]any) (string, error) {
	id, err := w.im.RegisterInstance(config)
	if err == nil {
		w.BusClient.Publish("sys/register", "register", map[string]any{
			"id":     id,
			"bundle": w.im.ModuleID(),
		})
		// Auto-setup query handler for the new instance
		go setupQueryHandler(w, id)
	}
	return id, err
}

func (w *moduleAPIWrapper) UpdateState(id string, state map[string]any) error {
	return w.im.UpdateState(id, state)
}

func (w *moduleAPIWrapper) ExecInstance(id string, funcName string, args ...any) (any, error) {
	return w.im.ExecInstance(id, funcName, args...)
}

func (w *moduleAPIWrapper) Context() context.Context {
	return w.ctx
}
