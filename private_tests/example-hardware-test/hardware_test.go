//go:build hardware
package example_hardware_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"module/internal/framework"
	"module/internal/harness"
	"module/internal/logic"
)

func TestRealHardware(t *testing.T) {
	ip := os.Getenv("DEVICE_IP")
	port := os.Getenv("DEVICE_PORT")
	if ip == "" || port == "" {
		t.Skip("DEVICE_IP or DEVICE_PORT not set, skipping hardware test")
	}

	t.Logf("Testing hardware at %s:%s", ip, port)

	// 1. Setup harness
	busHarness := harness.NewHarness()
	defer busHarness.Close()

	stateDir, _ := os.MkdirTemp("", "hw_test")
	defer os.RemoveAll(stateDir)
	os.Setenv("BUS_SOCKET", busHarness.SocketPath)
	os.Setenv("MODULE_ID", "hw-test-module")
	os.Setenv("STATE_DIR", stateDir)

	cfg := framework.LoadConfig()
	db, _ := framework.ConnectDB(cfg.StateDir)
	im := framework.NewInstanceManager(cfg.StateDir, cfg.ModuleID)
	
	moduleBus, _ := framework.NewBusClient(cfg.BusSocket, cfg.ModuleID, "DEBUG")
	triggerBus, _ := framework.NewBusClient(cfg.BusSocket, "tester", "INFO")

	api := &testAPI{BusClient: moduleBus, db: db, im: im}

	// 2. Register your real device
	id, _ := api.RegisterInstance(map[string]any{
		"ip":   ip,
		"port": port,
		"name": "Real Device",
	})

	// 3. Start logic
	go logic.Start(api)
	time.Sleep(1 * time.Second)

	// 4. Send OFF command
	t.Logf("SYSTEM: Sending OFF command to %s", id)
	triggerBus.Publish("commands/"+id, "toggle", map[string]any{"state": false})

	// 5. Verify BACK-AND-FORTH
	gotOff := false
	gotManualOn := false
	
	timeout := time.After(5 * time.Second)
	for !gotOff || !gotManualOn {
		select {
		case ev := <-busHarness.Events:
			if ev.Topic == "state/update" {
				isOn := ev.Data["on"].(bool)
				source := ev.Data["source"].(string)
				
				if !isOn && source == "api" {
					t.Log("CONFIRMED: System turned device OFF")
					gotOff = true
				}
				if isOn && source == "manual" {
					t.Log("CONFIRMED: Manual override detected (Device turned itself ON)")
					gotManualOn = true
				}
			}
		case <-timeout:
			t.Fatalf("Failed to verify full communication. GotOff: %v, GotManualOn: %v", gotOff, gotManualOn)
		}
	}
	t.Log("SUCCESS: Full bi-directional communication verified!")
}

type testAPI struct {
	*framework.BusClient
	db *sql.DB
	im *framework.InstanceManager
}

func (t *testAPI) DB() *sql.DB { return t.db }
func (t *testAPI) GetInstances() []framework.InstanceConfig {
	inst, _ := t.im.GetInstances()
	return inst
}
func (t *testAPI) RegisterInstance(config map[string]any) (string, error) {
	return t.im.RegisterInstance(config)
}
func (t *testAPI) ExecInstance(id string, funcName string, args ...any) (any, error) {
	return t.im.ExecInstance(id, funcName, args...)
}
func (t *testAPI) Context() context.Context { return context.Background() }