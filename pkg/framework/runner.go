package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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

		// Auto-start to ensure listeners are active
		if err := handler.Init(base); err != nil {
			base.SetBundleStatus(BundleStatus{State: StateError, Message: "Init failed: " + err.Error()})
		}
		if obs, ok := handler.(InstanceLifecycleObserver); ok {
			for _, inst := range base.GetInstances() {
				obs.OnInstanceRegistered(inst)
			}
		}

		topic := "commands/" + cfg.ModuleID
		ch := base.Listen(topic)
		for {
			select {
			case <-ctx.Done():
				return
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
				case "discover":
					if d, ok := handler.(DeviceDiscoverer); ok {
						go d.DiscoverDevice(ev.Data)
					} else {
						log.Printf("[%s] Received discover command but handler does not implement DeviceDiscoverer", cfg.ModuleID)
					}
				case "register_instance":
					if ev.Data == nil {
						break
					}
					payload := InstanceConfig{
						ID:      asString(ev.Data["id"]),
						Name:    asString(ev.Data["name"]),
						Alias:   asString(ev.Data["alias"]),
						Enabled: asBool(ev.Data["enabled"], true),
					}
					if cfgMap, ok := ev.Data["config"].(map[string]any); ok {
						payload.Config = cfgMap
					} else {
						payload.Config = map[string]any{}
					}
					if meta, ok := ev.Data["meta"].(map[string]any); ok {
						payload.Meta = meta
					}
					if raw, ok := ev.Data["raw_entities"].([]RawEntitySpec); ok {
						payload.RawEntities = raw
					}
					if raw, ok := ev.Data["raw_entities"].([]any); ok {
						data, _ := json.Marshal(raw)
						json.Unmarshal(data, &payload.RawEntities)
					}
					if raw, ok := ev.Data["raw_state"].(map[string]map[string]any); ok {
						payload.RawState = raw
					}
					if raw, ok := ev.Data["raw_state"].(map[string]any); ok {
						data, _ := json.Marshal(raw)
						json.Unmarshal(data, &payload.RawState)
					}
					if ents, ok := ev.Data["entities"].([]EntitySpec); ok {
						payload.Entities = ents
					}
					if ents, ok := ev.Data["entities"].([]any); ok {
						data, _ := json.Marshal(ents)
						json.Unmarshal(data, &payload.Entities)
					}
					if state, ok := ev.Data["entity_state"].(map[string]map[string]any); ok {
						payload.EntityState = state
					}
					if state, ok := ev.Data["entity_state"].(map[string]any); ok {
						data, _ := json.Marshal(state)
						json.Unmarshal(data, &payload.EntityState)
					}
					if p, ok := handler.(InstancePreprocessor); ok {
						next, err := p.PrepareInstance(payload)
						if err != nil {
							log.Printf("[%s] register_instance preprocess failed: %v", cfg.ModuleID, err)
							break
						}
						payload = next
					}
					if err := base.RegisterInstance(payload); err != nil {
						log.Printf("[%s] register_instance failed: %v", cfg.ModuleID, err)
					} else if obs, ok := handler.(InstanceLifecycleObserver); ok {
						obs.OnInstanceRegistered(payload)
					}
				case "delete_instance":
					id := asString(ev.Data["id"])
					if id == "" {
						break
					}
					log.Printf("[%s] delete_instance requested id=%s", cfg.ModuleID, id)
					if d, ok := handler.(InstanceDeleter); ok {
						d.DeleteInstance(id)
					}
					if err := base.DeleteInstance(id); err != nil {
						log.Printf("[%s] delete_instance failed: %v", cfg.ModuleID, err)
					} else if obs, ok := handler.(InstanceLifecycleObserver); ok {
						log.Printf("[%s] delete_instance completed id=%s", cfg.ModuleID, id)
						obs.OnInstanceDeleted(id)
					} else {
						log.Printf("[%s] delete_instance completed id=%s", cfg.ModuleID, id)
					}
				case "bundle_api":
					requestID := asString(ev.Data["request_id"])
					action := asString(ev.Data["action"])
					params, _ := ev.Data["params"].(map[string]any)
					resp := map[string]any{
						"bundle":     cfg.ModuleID,
						"request_id": requestID,
						"action":     action,
						"ok":         false,
					}
					result, err := handleBundleAPIRequest(cfg, cfgPath, base, handler, action, params)
					if err != nil {
						resp["error"] = err.Error()
					} else {
						resp["ok"] = true
						for k, v := range result {
							resp[k] = v
						}
					}
					base.Publish("sys/bundle_api_response", "bundle_api", resp)
				}
			}
		}
	}()

	<-ctx.Done()
	log.Printf("Module %s shutting down.", cfg.ModuleID)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any, fallback bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

func handleBundleAPIRequest(cfg RunnerConfig, cfgPath string, base *BaseModule, handler LifecycleHandler, action string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	switch action {
	case "get_config":
		cfgCopy := map[string]any{}
		for k, v := range base.modConfig {
			cfgCopy[k] = v
		}
		return map[string]any{"config": cfgCopy}, nil
	case "get_instance_file":
		id := asString(params["id"])
		fileType := asString(params["file_type"])
		if id == "" {
			return nil, fmt.Errorf("missing id")
		}
		var ext string
		switch fileType {
		case "script":
			ext = ".script"
		case "state":
			ext = ".state.json"
		default:
			return nil, fmt.Errorf("unsupported file_type")
		}
		targetPath := filepath.Join(cfg.StateDir, "instances", id+ext)
		data, err := os.ReadFile(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"found": false, "content": ""}, nil
			}
			return nil, err
		}
		return map[string]any{"found": true, "content": string(data)}, nil
	case "set_instance_script":
		id := asString(params["id"])
		content := asString(params["content"])
		if id == "" {
			return nil, fmt.Errorf("missing id")
		}
		dir := filepath.Join(cfg.StateDir, "instances")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
		targetPath := filepath.Join(dir, id+".script")
		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			return nil, err
		}
		return map[string]any{}, nil
	case "get_bundle_manifest":
		for _, p := range []string{
			filepath.Join(strings.TrimSpace(os.Getenv("MODULE_DIR")), "module.json"),
			filepath.Join(strings.TrimSpace(os.Getenv("MODULE_DIR")), "bundle.json"),
		} {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			return map[string]any{"manifest": string(data)}, nil
		}
		return map[string]any{"manifest": ""}, nil
	case "mcp_describe":
		desc := MCPDescriptor{
			Instructions: []string{
				"Use framework base tools for instances and config whenever possible.",
			},
			Tools: []MCPTool{
				{
					Name:        "instances.list",
					Description: "List bundle instances.",
					Mutating:    false,
				},
				{
					Name:        "instances.add",
					Description: "Create/update an instance.",
					Mutating:    true,
				},
				{
					Name:        "instances.remove",
					Description: "Remove an instance by id.",
					Mutating:    true,
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{"type": "string"},
						},
						"required": []string{"id"},
					},
				},
				{
					Name:        "config.get",
					Description: "Get bundle config.",
					Mutating:    false,
				},
				{
					Name:        "config.set",
					Description: "Set and validate bundle config.",
					Mutating:    true,
				},
			},
		}
		if p, ok := handler.(MCPProvider); ok {
			custom := p.MCPDescribe()
			desc.Tools = append(desc.Tools, custom.Tools...)
			desc.Instructions = append(desc.Instructions, custom.Instructions...)
		}
		return map[string]any{
			"bundle_id": cfg.ModuleID,
			"mcp":       desc,
		}, nil
	case "mcp_invoke":
		tool := strings.TrimSpace(asString(params["tool"]))
		args, _ := params["args"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		switch tool {
		case "instances.list", "devices.list":
			return map[string]any{
				"items": base.GetInstances(),
				"count": len(base.GetInstances()),
			}, nil
		case "instances.add", "instances.update":
			instRaw, ok := args["instance"].(map[string]any)
			if !ok {
				instRaw = args
			}
			payload, err := parseInstanceConfig(instRaw)
			if err != nil {
				return nil, err
			}
			if payload.ID == "" {
				return nil, fmt.Errorf("missing instance id")
			}
			if p, ok := handler.(InstancePreprocessor); ok {
				next, err := p.PrepareInstance(payload)
				if err != nil {
					return nil, err
				}
				payload = next
			}
			if err := base.RegisterInstance(payload); err != nil {
				return nil, err
			}
			if obs, ok := handler.(InstanceLifecycleObserver); ok {
				obs.OnInstanceRegistered(payload)
			}
			return map[string]any{"ok": true, "instance": payload}, nil
		case "instances.remove":
			id := strings.TrimSpace(asString(args["id"]))
			if id == "" {
				return nil, fmt.Errorf("missing id")
			}
			if d, ok := handler.(InstanceDeleter); ok {
				d.DeleteInstance(id)
			}
			if err := base.DeleteInstance(id); err != nil {
				return nil, err
			}
			if obs, ok := handler.(InstanceLifecycleObserver); ok {
				obs.OnInstanceDeleted(id)
			}
			return map[string]any{"ok": true, "id": id}, nil
		case "config.get":
			cfgCopy := map[string]any{}
			for k, v := range base.modConfig {
				cfgCopy[k] = v
			}
			return map[string]any{"config": cfgCopy}, nil
		case "config.set":
			newCfg, _ := args["config"].(map[string]any)
			if newCfg == nil {
				newCfg = map[string]any{}
			}
			if err := handler.ValidateConfig(base.Context(), newCfg); err != nil {
				return nil, err
			}
			data, _ := json.MarshalIndent(newCfg, "", "  ")
			if err := os.WriteFile(cfgPath, data, 0644); err != nil {
				return nil, err
			}
			base.modConfig = newCfg
			base.SetBundleStatus(BundleStatus{State: StateReady, Message: "Verified", Config: newCfg})
			return map[string]any{"ok": true, "config": newCfg}, nil
		default:
			if p, ok := handler.(MCPProvider); ok {
				out, err := p.MCPInvoke(tool, args, base)
				if err != nil {
					return nil, err
				}
				if out == nil {
					out = map[string]any{}
				}
				return out, nil
			}
			return nil, fmt.Errorf("unsupported tool: %s", tool)
		}
	default:
		return nil, fmt.Errorf("unsupported action")
	}
}

func parseInstanceConfig(raw map[string]any) (InstanceConfig, error) {
	if raw == nil {
		return InstanceConfig{}, fmt.Errorf("missing instance")
	}
	payload := InstanceConfig{
		ID:      asString(raw["id"]),
		Name:    asString(raw["name"]),
		Alias:   asString(raw["alias"]),
		Enabled: asBool(raw["enabled"], true),
	}
	if cfgMap, ok := raw["config"].(map[string]any); ok {
		payload.Config = cfgMap
	} else {
		payload.Config = map[string]any{}
	}
	if meta, ok := raw["meta"].(map[string]any); ok {
		payload.Meta = meta
	}
	if rawEnts, ok := raw["raw_entities"].([]any); ok {
		data, _ := json.Marshal(rawEnts)
		_ = json.Unmarshal(data, &payload.RawEntities)
	}
	if rawState, ok := raw["raw_state"].(map[string]any); ok {
		data, _ := json.Marshal(rawState)
		_ = json.Unmarshal(data, &payload.RawState)
	}
	if ents, ok := raw["entities"].([]any); ok {
		data, _ := json.Marshal(ents)
		_ = json.Unmarshal(data, &payload.Entities)
	}
	if state, ok := raw["entity_state"].(map[string]any); ok {
		data, _ := json.Marshal(state)
		_ = json.Unmarshal(data, &payload.EntityState)
	}
	return payload, nil
}
