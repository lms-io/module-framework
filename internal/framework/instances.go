package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
)

type ControlSpec struct {
	Type    string   `json:"type"`              // switch, range, select, sensor
	Min     float64  `json:"min,omitempty"`     // for range
	Max     float64  `json:"max,omitempty"`     // for range
	Step    float64  `json:"step,omitempty"`    // for range
	Unit    string   `json:"unit,omitempty"`    // for range/sensor
	Options []string `json:"options,omitempty"` // for select
	ReadOnly bool    `json:"read_only,omitempty"`
}

type InstanceConfig struct {
	ID       string                 `json:"id"`
	Enabled  bool                   `json:"enabled"`
	Config   map[string]any         `json:"config"`
	State    map[string]any         `json:"state"`
	Controls map[string]ControlSpec `json:"controls,omitempty"`
}

type InstanceManager struct {
	stateDir string
	moduleID string
}

func NewInstanceManager(stateDir, moduleID string) *InstanceManager {
	return &InstanceManager{
		stateDir: stateDir,
		moduleID: moduleID,
	}
}

func (im *InstanceManager) ModuleID() string {
	return im.moduleID
}

func (im *InstanceManager) RegisterInstance(config map[string]any) (string, error) {
	instancesDir := filepath.Join(im.stateDir, "instances")
	os.MkdirAll(instancesDir, 0755)

	// Extract controls if present to keep config clean
	var controls any
	if c, ok := config["controls"]; ok {
		controls = c
		delete(config, "controls")
	}

	// 1. Find the next hex ID
	files, _ := os.ReadDir(instancesDir)
	maxID := -1
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".star" {
			name := strings.TrimSuffix(f.Name(), ".star")
			idx := strings.LastIndex(name, "-")
			if idx != -1 {
				hexPart := name[idx+1:]
				var id int
				_, err := fmt.Sscanf(hexPart, "%X", &id)
				if err == nil && id > maxID {
					maxID = id
				}
			}
		}
	}

	nextID := maxID + 1
	hexID := fmt.Sprintf("%s-%04X", im.moduleID, nextID)
	
	// 2. Generate Starlark file
	content := fmt.Sprintf("# USER LOGIC FILE\nid = %q\nenabled = True\n", hexID)
	if controls != nil {
		// Serialize controls to Starlark dict
		if cMap, ok := controls.(map[string]ControlSpec); ok {
			// Convert struct to map for formatting
			// Quick hack: marshal to json then to map to reuse formatStarlarkDict
			b, _ := json.Marshal(cMap)
			var m map[string]any
			json.Unmarshal(b, &m)
			content += fmt.Sprintf("controls = %s\n", im.formatStarlarkDict(m))
		} else if cMap, ok := controls.(map[string]any); ok {
			content += fmt.Sprintf("controls = %s\n", im.formatStarlarkDict(cMap))
		}
	}
	content += fmt.Sprintf("config = %s\n", im.formatStarlarkDict(config))
	
	filePath := filepath.Join(instancesDir, hexID+".star")
	return hexID, os.WriteFile(filePath, []byte(content), 0644)
}

func (im *InstanceManager) UpdateState(id string, state map[string]any) error {
	instancesDir := filepath.Join(im.stateDir, "instances")
	filePath := filepath.Join(instancesDir, id+".state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

func (im *InstanceManager) GetState(id string) (map[string]any, error) {
	instancesDir := filepath.Join(im.stateDir, "instances")
	filePath := filepath.Join(instancesDir, id+".state.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var state map[string]any
	err = json.Unmarshal(data, &state)
	return state, err
}

func (im *InstanceManager) GetInstances() ([]InstanceConfig, error) {
	instancesDir := filepath.Join(im.stateDir, "instances")
	files, err := os.ReadDir(instancesDir)
	if err != nil {
		if os.IsNotExist(err) { return []InstanceConfig{}, nil }
		return nil, err
	}

	var instances []InstanceConfig
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".star" {
			cfg, err := im.loadStarlarkInstance(filepath.Join(instancesDir, f.Name()))
			if err == nil {
				instances = append(instances, cfg)
			}
		}
	}
	return instances, nil
}

func (im *InstanceManager) loadStarlarkInstance(path string) (InstanceConfig, error) {
	// 1. Load System Sidecar State if it exists
	statePath := strings.TrimSuffix(path, ".star") + ".state.json"
	stateData := make(map[string]any)
	if data, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(data, &stateData)
	}

	// 2. Setup Starlark environment with state injected as 'context'
	thread := &starlark.Thread{Name: "instance-loader"}
	predeclared := starlark.StringDict{
		"context": im.mapToStarlarkDict(stateData),
	}

	globals, err := starlark.ExecFile(thread, path, nil, predeclared)
	if err != nil { return InstanceConfig{}, err }

	cfg := InstanceConfig{Enabled: true, State: stateData}
	
	if val, ok := globals["id"]; ok {
		if s, ok := starlark.AsString(val); ok { cfg.ID = s }
	}
	if val, ok := globals["enabled"]; ok {
		if b, ok := val.(starlark.Bool); ok { cfg.Enabled = bool(b) }
	}
	if val, ok := globals["config"]; ok {
		if dict, ok := val.(*starlark.Dict); ok { cfg.Config = im.starlarkToMap(dict) }
	}
	if val, ok := globals["controls"]; ok {
		if dict, ok := val.(*starlark.Dict); ok {
			// Convert generic map to ControlSpec map
			raw := im.starlarkToMap(dict)
			cfg.Controls = make(map[string]ControlSpec)
			b, _ := json.Marshal(raw)
			json.Unmarshal(b, &cfg.Controls)
		}
	}

	return cfg, nil
}

func (im *InstanceManager) mapToStarlarkDict(m map[string]any) *starlark.Dict {
	dict := starlark.NewDict(len(m))
	for k, v := range m {
		var sv starlark.Value
		switch val := v.(type) {
		case string: sv = starlark.String(val)
		case float64: sv = starlark.Float(val)
		case int64: sv = starlark.MakeInt64(val)
		case int: sv = starlark.MakeInt(val)
		case bool: sv = starlark.Bool(val)
		case map[string]any: sv = im.mapToStarlarkDict(val)
		default: sv = starlark.String(fmt.Sprintf("%v", v))
		}
		dict.SetKey(starlark.String(k), sv)
	}
	return dict
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
		case starlark.Bool: res[key] = bool(val)
		case *starlark.Dict: res[key] = im.starlarkToMap(val)
		default: res[key] = v.String()
		}
	}
	return res
}

func (im *InstanceManager) formatStarlarkDict(m map[string]any) string {
	b, _ := json.MarshalIndent(m, "", "    ")
	// JSON is mostly valid Starlark for simple dicts
	s := string(b)
	s = strings.ReplaceAll(s, "null", "None")
	s = strings.ReplaceAll(s, "true", "True")
	s = strings.ReplaceAll(s, "false", "False")
	return s
}

func (im *InstanceManager) ExecInstance(id string, funcName string, args ...any) (any, error) {
	path := filepath.Join(im.stateDir, "instances", id+".star")
	thread := &starlark.Thread{Name: "instance-exec"}
	globals, err := starlark.ExecFile(thread, path, nil, nil)
	if err != nil { return nil, err }

	f, ok := globals[funcName]
	if !ok { return nil, fmt.Errorf("function %s not found", funcName) }

	starArgs := make(starlark.Tuple, len(args))
	for i, a := range args {
		// Basic conversion
		switch v := a.(type) {
		case string: starArgs[i] = starlark.String(v)
		case int:    starArgs[i] = starlark.MakeInt(v)
		case bool:   starArgs[i] = starlark.Bool(v)
		default:     starArgs[i] = starlark.String(fmt.Sprintf("%v", v))
		}
	}

	res, err := starlark.Call(thread, f, starArgs, nil)
	if err != nil { return nil, err }

	return res.String(), nil
}
