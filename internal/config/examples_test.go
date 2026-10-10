package config

import (
	"path/filepath"
	"testing"
)

func TestExampleConfigsAreValid(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "configs", "examples", "*.json"))
	if err != nil {
		t.Fatalf("find example configs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no example configs found")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg, err := LoadConfig(path)
			if err != nil {
				t.Fatalf("load example config: %v", err)
			}
			if err := ValidateConfig(cfg); err != nil {
				t.Fatalf("validate example config: %v", err)
			}
		})
	}
}
