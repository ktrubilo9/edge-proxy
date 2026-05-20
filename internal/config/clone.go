package config

import "encoding/json"

func CloneFullConfig(cfg *FullConfig) (*FullConfig, error) {
	if cfg == nil {
		return nil, nil
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var clone FullConfig
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, err
	}

	return &clone, nil
}
