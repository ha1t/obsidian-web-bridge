package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `vault_path: /home/ha1t/src/ObsidianGitVault
port: 18484
api_key: "test-key-123"
bind: "0.0.0.0"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}

	if cfg.VaultPath != "/home/ha1t/src/ObsidianGitVault" {
		t.Errorf("VaultPath = %q, want /home/ha1t/src/ObsidianGitVault", cfg.VaultPath)
	}
	if cfg.Port != 18484 {
		t.Errorf("Port = %d, want 18484", cfg.Port)
	}
	if cfg.APIKey != "test-key-123" {
		t.Errorf("APIKey = %q, want test-key-123", cfg.APIKey)
	}
	if cfg.Bind != "0.0.0.0" {
		t.Errorf("Bind = %q, want 0.0.0.0", cfg.Bind)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `vault_path: /tmp/vault
api_key: "key"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}

	if cfg.Port != 18484 {
		t.Errorf("Port default = %d, want 18484", cfg.Port)
	}
	if cfg.Bind != "0.0.0.0" {
		t.Errorf("Bind default = %q, want 0.0.0.0", cfg.Bind)
	}
}
