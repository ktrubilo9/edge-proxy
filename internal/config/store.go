package config

import (
	"encoding/json"
	"os"
)

type Store interface {
	Load() (*FullConfig, error)
	Save(cfg *FullConfig) error
}

type FileStore struct {
	path string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{
		path: path,
	}
}

func LoadConfig(path string) (*FullConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Runtime code works with resolved values so backends and ports can come from the environment.
	resolved := ResolveEnvVars(raw)
	resolvedData, err := json.Marshal(resolved)
	if err != nil {
		return nil, err
	}

	var fullConfig FullConfig
	if err := json.Unmarshal(resolvedData, &fullConfig); err != nil {
		return nil, err
	}

	ApplyDefaults(&fullConfig)
	return &fullConfig, nil
}

func SaveConfig(path string, cfg *FullConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func (fs FileStore) Load() (*FullConfig, error) {
	return LoadConfig(fs.path)
}

func (fs FileStore) Save(cfg *FullConfig) error {
	return SaveConfig(fs.path, cfg)
}
