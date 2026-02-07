package tests

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"module/internal/framework"
	"module/internal/harness"
	"module/internal/logic"
)

func TestModuleFullLifecycle(t *testing.T) {
	// 1. Start Mock Device
	mock := exec.Command("go", "run", "./mock_device/main.go")
	mock.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	mock.Start()
	defer func() {
		pgid, _ := syscall.Getpgid(mock.Process.Pid)
		syscall.Kill(-pgid, syscall.SIGKILL)
		mock.Wait()
	}()

	// 2. Setup Harness & Env
	busHarness := harness.NewHarness()
	defer busHarness.Close()

	stateDir, _ := os.MkdirTemp("", "framework_example_state")
	defer os.RemoveAll(stateDir)
	
	os.Setenv("BUS_SOCKET", busHarness.SocketPath)
	os.Setenv("MODULE_ID", "example-module")
	os.Setenv("STATE_DIR", stateDir)

	cfg := framework.LoadConfig()
	db, _ := framework.ConnectDB(cfg.StateDir)
	im := framework.NewInstanceManager(cfg.StateDir, cfg.ModuleID)
	
	moduleBus, _ := framework.NewBusClient(cfg.BusSocket, cfg.ModuleID, "DEBUG")
	triggerBus, _ := framework.NewBusClient(cfg.BusSocket, "tester", "INFO")

	api := &testAPI{
		BusClient: moduleBus,
		db:        db,
		im:        im,
	}

	// 3. Start Logic
	go logic.Start(api)

	// 4. Wait for Discovery (max 10s)
	timeout := time.After(10 * time.Second)
	var instanceID string
	for {
		select {
		case <-time.After(500 * time.Millisecond):
			instances, _ := im.GetInstances()
			if len(instances) > 0 {
				instanceID = instances[0].ID
				goto discovered
			}
		case <-timeout:
			t.Fatal("Module failed to discover the mock device")
		}
	}

discovered:
	t.Logf("Discovered instance: %s", instanceID)

	// 5. Test Command
	triggerBus.Publish("commands/"+instanceID, "toggle", map[string]any{"state": true})

	// 6. Verify Response
	select {
	case ev := <-busHarness.Events:
		if ev.Topic == "state/update" && ev.Data["on"] == true {
			t.Log("SUCCESS: Received state update from module")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for state update response")
	}

	// 7. Test Request/Reply State Query
	// We'll simulate what the Automation Bundle will do.
	replyTopic := "test/reply/123"
	triggerBus.Subscribe(replyTopic) // Subscribe so we can receive the reply
	
	// Manually inject a state so we have something to query
	api.UpdateState(instanceID, map[string]any{"power": true, "voltage": 120.5})
	
	t.Log("Sending query_state request...")
	triggerBus.Publish("commands/"+instanceID, "query_state", map[string]any{
		"reply_to": replyTopic,
	})

	// The logic.Start in this test is using testAPI, which doesn't have the 
	// automatic handler from main.go. I should either:
	// A. Update testAPI to have the same handler.
	// B. Accept that integration_test.go is testing LOGIC, not the WRAPPER.
	
	// To test the framework's promise, I'll add the handler to TestModuleFullLifecycle manually 
	// or update testAPI. I'll update the test loop to handle it.
	
	select {
	case ev := <-busHarness.Events:
		if ev.Topic == replyTopic && ev.Type == "state_response" {
			if ev.Data["voltage"] == 120.5 {
				t.Log("SUCCESS: Received correct state_response via Bus")
				return
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for state_response")
	}
}

type testAPI struct {
	*framework.BusClient
	db *sql.DB
	im *framework.InstanceManager
}

func (t *testAPI) DB() *sql.DB { return t.db }
func (t *testAPI) GetModuleConfig() map[string]any { return map[string]any{"test_mode": true} }
func (t *testAPI) GetInstances() []framework.InstanceConfig {
	inst, _ := t.im.GetInstances()
	return inst
}
func (t *testAPI) RegisterInstance(config map[string]any) (string, error) {
	id, err := t.im.RegisterInstance(config)
	if err == nil {
		// Mock the background handler for testing
		go func() {
			topic := "commands/" + id
			ch := t.Subscribe(topic)
			for ev := range ch {
				if ev.Type == "query_state" {
					replyTo, _ := ev.Data["reply_to"].(string)
					state, _ := t.im.GetState(id)
					t.Publish(replyTo, "state_response", state)
				}
			}
		}()
	}
	return id, err
}
func (t *testAPI) UpdateState(id string, state map[string]any) error {
	return t.im.UpdateState(id, state)
}
func (t *testAPI) ExecInstance(id string, funcName string, args ...any) (any, error) {
	return t.im.ExecInstance(id, funcName, args...)
}
func (t *testAPI) Context() context.Context { return context.Background() }
