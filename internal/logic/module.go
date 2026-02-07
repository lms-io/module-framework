package logic

import (
	"encoding/json"
	"fmt"
	"module/internal/framework"
	"net"
	"time"
)

func Start(api framework.ModuleAPI) {
	api.Info("Module logic started. Waiting for discovery...")

	// 1. Setup Database
	_, err := api.DB().Exec("CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY, msg TEXT)")
	if err != nil {
		api.Error("Failed to setup DB: %v", err)
	}

	// 2. Continuous Discovery Loop
	go func() {
		for {
			select {
			case <-api.Context().Done():
				return
			case <-time.After(5 * time.Second):
				if len(api.GetInstances()) == 0 {
					discoverHardware(api)
				}
			}
		}
	}()

	// 3. Handle existing instances
	for _, inst := range api.GetInstances() {
		go runDeviceHandler(api, inst)
	}

	<-api.Context().Done()
}

func discoverHardware(api framework.ModuleAPI) {
	// In this example, we check a specific local port
	conn, err := net.DialTimeout("tcp", "127.0.0.1:9000", 1*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	json.NewEncoder(conn).Encode(map[string]any{"cmd": "identify"})
	var resp map[string]any
	json.NewDecoder(conn).Decode(&resp)

	api.Info("Discovered hardware: %v. Registering...", resp["name"])

	id, err := api.RegisterInstance(map[string]any{
		"ip":   "127.0.0.1",
		"port": 9000,
		"name": resp["name"],
	})

	if err == nil {
		api.Info("Instance created: %s", id)
		// Usually we'd reload or just start the handler
		go runDeviceHandler(api, framework.InstanceConfig{
			ID:     id,
			Config: map[string]any{"ip": "127.0.0.1", "port": 9000},
		})
	}
}

func runDeviceHandler(api framework.ModuleAPI, inst framework.InstanceConfig) {
	api.Info("Starting handler for %s", inst.ID)
	
	// Subscribe to commands for this specific ID
	cmdTopic := fmt.Sprintf("commands/%s", inst.ID)
	cmds := api.Subscribe(cmdTopic)

	for {
		select {
		case <-api.Context().Done():
			return
		case ev := <-cmds:
			if ev.Type == "toggle" {
				state := ev.Data["state"].(bool)
				api.Info("[%s] Setting power to %v", inst.ID, state)
				
				// Talk to the mock device
				addr := fmt.Sprintf("%v:%v", inst.Config["ip"], inst.Config["port"])
				conn, _ := net.Dial("tcp", addr)
				if conn != nil {
					json.NewEncoder(conn).Encode(map[string]any{
						"cmd":   "set_power",
						"state": state,
					})
					
					// Listen for an immediate 'unsolicited' response/override
					var override map[string]any
					decoder := json.NewDecoder(conn)
					if !state {
						// If we sent OFF, the mock is programmed to send a manual ON back
						decoder.Decode(&override) 
						api.Info("[%s] MANUAL OVERRIDE DETECTED: Device is now ON", inst.ID)
						api.Publish("state/update", "status", map[string]any{
							"id": inst.ID,
							"on": true,
							"source": "manual",
						})
					}
					conn.Close()
				}
				
				api.Publish("state/update", "status", map[string]any{
					"id": inst.ID,
					"on": state,
					"source": "api",
				})
			}
		}
	}
}