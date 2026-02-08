package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.starlark.net/starlark"
)

// InstanceManager handles saving/loading device configurations and states to disk.
type InstanceManager struct {
	stateDir string
	moduleID string
	mu       sync.RWMutex
}

func NewInstanceManager(stateDir, moduleID string) *InstanceManager {
	return &InstanceManager{
		stateDir: stateDir,
		moduleID: moduleID,
	}
}

func (im *InstanceManager) RegisterInstance(payload InstanceConfig) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	dir := filepath.Join(im.stateDir, "instances")
	os.MkdirAll(dir, 0755)

	// We use Starlark (.star) for Config + Controls (Static)
	// and JSON (.state.json) for live State (Dynamic)
	
	// 1. Save Static Config + Controls
	content := fmt.Sprintf("# AUTOMATICALLY GENERATED\nid = %q\nenabled = %v\n", payload.ID, payload.Enabled)
	
	controlsJSON, _ := json.MarshalIndent(payload.Controls, "", "    ")
	content += fmt.Sprintf("controls = %s\n", im.jsonToStarlark(controlsJSON))
	
	configJSON, _ := json.MarshalIndent(payload.Config, "", "    ")
	content += fmt.Sprintf("config = %s\n", im.jsonToStarlark(configJSON))

	metaJSON, _ := json.MarshalIndent(payload.Meta, "", "    ")
	content += fmt.Sprintf("meta = %s\n", im.jsonToStarlark(metaJSON))

	starPath := filepath.Join(dir, payload.ID+".star")
	if err := os.WriteFile(starPath, []byte(content), 0644); err != nil {
		return err
	}

	// 2. Save live state separately if provided
	if len(payload.State) > 0 {
		return im.saveState(payload.ID, payload.State)
	}
	
	return nil
}

func (im *InstanceManager) UpdateState(id string, state map[string]any) error {
	return im.saveState(id, state)
}

func (im *InstanceManager) saveState(id string, state map[string]any) error {
	dir := filepath.Join(im.stateDir, "instances")
	path := filepath.Join(dir, id+".state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, data, 0644)
}

func (im *InstanceManager) GetInstances() ([]InstanceConfig, error) {
	dir := filepath.Join(im.stateDir, "instances")
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) { return []InstanceConfig{}, nil }
		return nil, err
	}

	var instances []InstanceConfig
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".star" {
			cfg, err := im.loadStarlarkInstance(filepath.Join(dir, f.Name()))
			if err == nil {
				instances = append(instances, cfg)
			}
		}
	}
	return instances, nil
}

func (im *InstanceManager) loadStarlarkInstance(path string) (InstanceConfig, error) {
	thread := &starlark.Thread{Name: "instance-loader"}
	globals, err := starlark.ExecFile(thread, path, nil, nil)
	if err != nil { return InstanceConfig{}, err }

	cfg := InstanceConfig{Enabled: true}
	if val, ok := globals["id"]; ok {
		if s, ok := starlark.AsString(val); ok { cfg.ID = s }
	}
	if val, ok := globals["enabled"]; ok {
		if b, ok := val.(starlark.Bool); ok { cfg.Enabled = bool(b) }
	}
	
	// Load Maps
	if val, ok := globals["config"]; ok {
		if dict, ok := val.(*starlark.Dict); ok { cfg.Config = im.starlarkToMap(dict) }
	}
	if val, ok := globals["meta"]; ok {
		if dict, ok := val.(*starlark.Dict); ok { cfg.Meta = im.starlarkToMap(dict) }
	}
	if val, ok := globals["controls"]; ok {
		if dict, ok := val.(*starlark.Dict); ok {
			raw := im.starlarkToMap(dict)
			cfg.Controls = make(map[string]ControlSpec)
			// Efficiently convert map to typed struct
			b, _ := json.Marshal(raw)
			json.Unmarshal(b, &cfg.Controls)
		}
	}

	// Load live state from JSON if it exists
	statePath := strings.TrimSuffix(path, ".star") + ".state.json"
	if data, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(data, &cfg.State)
	}

	return cfg, nil
}

func (im *InstanceManager) starlarkToMap(dict *starlark.Dict) map[string]any {
	res := make(map[string]any)
	for _, k := range dict.Keys() {
		v, _, _ := dict.Get(k)
		key, _ := starlark.AsString(k)
		switch val := v.(type) {
		case starlark.String: res[key] = string(val)
		case starlark.Int: 
			i, _ := val.Int64()
			res[key] = i
		case starlark.Float: res[key] = float64(val)
		case starlark.Bool: res[key] = bool(val)
		case *starlark.Dict: res[key] = im.starlarkToMap(val)
		default: res[key] = v.String()
		}
	}
	return res
}

func (im *InstanceManager) jsonToStarlark(data []byte) string {
	s := string(data)
	s = strings.ReplaceAll(s, "null", "None")
	s = strings.ReplaceAll(s, "true", "True")
	s = strings.ReplaceAll(s, "false", "False")
	return s
}
