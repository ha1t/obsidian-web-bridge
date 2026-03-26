package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	VaultPath string `yaml:"vault_path"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key"`
	Bind      string `yaml:"bind"`
	Server    string `yaml:"server"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	cfg := &Config{
		Port: 18484,
		Bind: "0.0.0.0",
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.VaultPath == "" {
		return nil, fmt.Errorf("vault_path is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	return cfg, nil
}
