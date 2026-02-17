package framework

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

	if payload.ID == "" {
		payload.ID = GenerateID()
	}

	enabledInt := 0
	if payload.Enabled {
		enabledInt = 1
	}

	// Persist instance config as JSON
	payload.Enabled = enabledInt == 1
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	instancePath := filepath.Join(dir, payload.ID+".instance.json")
	if err := os.WriteFile(instancePath, data, 0644); err != nil {
		return err
	}

	// 2. Save live entity state separately if provided
	if len(payload.EntityState) > 0 {
		return im.saveEntityState(payload.ID, payload.EntityState)
	}

	return nil
}

func (im *InstanceManager) DeleteInstance(id string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	dir := filepath.Join(im.stateDir, "instances")
	paths := []string{
		filepath.Join(dir, id+".instance.json"),
		filepath.Join(dir, id+".state.json"),
		filepath.Join(dir, id+".script"),
		filepath.Join(dir, id+".script.state.json"),
	}
	log.Printf("[%s] DeleteInstance start id=%s dir=%s", im.moduleID, id, dir)
	var firstErr error
	for _, p := range paths {
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				log.Printf("[%s] DeleteInstance id=%s file missing (already absent): %s", im.moduleID, id, p)
				continue
			}
			log.Printf("[%s] DeleteInstance id=%s remove failed path=%s err=%v", im.moduleID, id, p, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		// Verify the file is actually gone immediately after remove.
		if _, err := os.Stat(p); err == nil {
			verifyErr := fmt.Errorf("delete verification failed, file still exists: %s", p)
			log.Printf("[%s] DeleteInstance id=%s %v", im.moduleID, id, verifyErr)
			if firstErr == nil {
				firstErr = verifyErr
			}
			continue
		} else if !os.IsNotExist(err) {
			log.Printf("[%s] DeleteInstance id=%s stat-after-delete failed path=%s err=%v", im.moduleID, id, p, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		log.Printf("[%s] DeleteInstance id=%s deleted path=%s", im.moduleID, id, p)
	}
	if firstErr != nil {
		log.Printf("[%s] DeleteInstance done id=%s with error=%v", im.moduleID, id, firstErr)
	} else {
		log.Printf("[%s] DeleteInstance done id=%s success", im.moduleID, id)
	}
	return firstErr
}

func (im *InstanceManager) UpdateEntityState(id string, state map[string]map[string]any) error {
	return im.saveEntityState(id, state)
}

func (im *InstanceManager) saveEntityState(id string, state map[string]map[string]any) error {
	dir := filepath.Join(im.stateDir, "instances")
	path := filepath.Join(dir, id+".state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (im *InstanceManager) GetInstances() ([]InstanceConfig, error) {
	dir := filepath.Join(im.stateDir, "instances")
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []InstanceConfig{}, nil
		}
		return nil, err
	}

	var instances []InstanceConfig
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".instance.json") {
			cfg, err := im.loadJSONInstance(filepath.Join(dir, f.Name()))
			if err == nil {
				instances = append(instances, cfg)
			}
		}
	}
	return instances, nil
}

func (im *InstanceManager) loadJSONInstance(path string) (InstanceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return InstanceConfig{}, err
	}
	var inst InstanceConfig
	if err := json.Unmarshal(data, &inst); err != nil {
		return InstanceConfig{}, err
	}

	// Load live entity state from JSON if it exists
	statePath := strings.TrimSuffix(path, ".instance.json") + ".state.json"
	if data, err := os.ReadFile(statePath); err == nil {
		json.Unmarshal(data, &inst.EntityState)
	}
	return inst, nil
}
