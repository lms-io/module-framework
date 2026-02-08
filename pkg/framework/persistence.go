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

	enabledInt := 0
	if payload.Enabled { enabledInt = 1 }

	// 1. Prepare Content
	content := fmt.Sprintf(`id = "%s"
name = "%s"
alias = "%s"
enabled = %d
`, payload.ID, payload.Name, payload.Alias, enabledInt)
	
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

	id, _ := starlark.AsString(globals["id"])
	name, _ := starlark.AsString(globals["name"])
	alias, _ := starlark.AsString(globals["alias"])
	
	enabled := false
	if val, ok := globals["enabled"]; ok {
		if i, ok := val.(starlark.Int); ok {
			i64, _ := i.Int64()
			enabled = i64 == 1
		}
	}
	
	inst := InstanceConfig{
		ID:      id,
		Name:    name,
		Alias:   alias,
		Enabled: enabled,
	}
	
	// Load Maps
	if val, ok := globals["config"]; ok {
		if dict, ok := val.(*starlark.Dict); ok { inst.Config = im.starlarkToMap(dict) }
	}
	if val, ok := globals["meta"]; ok {
		if dict, ok := val.(*starlark.Dict); ok { inst.Meta = im.starlarkToMap(dict) }
	}
	if val, ok := globals["controls"]; ok {
		if dict, ok := val.(*starlark.Dict); ok {
			raw := im.starlarkToMap(dict)
			inst.Controls = make(map[string]ControlSpec)
			b, _ := json.Marshal(raw)
			json.Unmarshal(b, &inst.Controls)
		}
	}

	// Load live state from JSON if it exists
	statePath := strings.TrimSuffix(path, ".star") + ".state.json"
	if data, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(data, &inst.State)
	}

	return inst, nil
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