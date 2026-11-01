package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("書き込み失敗: %v", err)
	}
	return cfgPath
}

const baseConfigYAML = `vault_path: /tmp/vault
api_key: "key"
`

func TestLoad_NoMCPSection(t *testing.T) {
	cfg, err := Load(writeTestConfig(t, baseConfigYAML))
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}
	if cfg.MCP != nil {
		t.Errorf("MCP = %+v, want nil", cfg.MCP)
	}
}

func TestLoad_MCPDefaults(t *testing.T) {
	content := baseConfigYAML + `mcp:
  cf_access_team_domain: example.cloudflareaccess.com
  cf_access_aud: aud-123
`
	cfg, err := Load(writeTestConfig(t, content))
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}
	if cfg.MCP == nil {
		t.Fatal("MCP = nil, want non-nil")
	}
	if cfg.MCP.Bind != "127.0.0.1" {
		t.Errorf("MCP.Bind = %q, want 127.0.0.1", cfg.MCP.Bind)
	}
	if cfg.MCP.Port != 18485 {
		t.Errorf("MCP.Port = %d, want 18485", cfg.MCP.Port)
	}
	if cfg.MCP.Timezone != "Asia/Tokyo" {
		t.Errorf("MCP.Timezone = %q, want Asia/Tokyo", cfg.MCP.Timezone)
	}
	if !reflect.DeepEqual(cfg.MCP.ReadonlyFolders, DefaultReadonlyFolders) {
		t.Errorf("MCP.ReadonlyFolders = %v, want %v", cfg.MCP.ReadonlyFolders, DefaultReadonlyFolders)
	}
	if cfg.MCP.CFAccessTeamDomain != "example.cloudflareaccess.com" {
		t.Errorf("MCP.CFAccessTeamDomain = %q", cfg.MCP.CFAccessTeamDomain)
	}
	if cfg.MCP.CFAccessAUD != "aud-123" {
		t.Errorf("MCP.CFAccessAUD = %q", cfg.MCP.CFAccessAUD)
	}
}

func TestLoad_MCPMissingAccess(t *testing.T) {
	tests := []struct {
		name    string
		mcp     string
		wantKey string
	}{
		{
			name:    "aud missing",
			mcp:     "  cf_access_team_domain: example.cloudflareaccess.com\n",
			wantKey: "cf_access_aud",
		},
		{
			name:    "team_domain missing",
			mcp:     "  cf_access_aud: aud-123\n",
			wantKey: "cf_access_team_domain",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := baseConfigYAML + "mcp:\n" + tt.mcp
			_, err := Load(writeTestConfig(t, content))
			if err == nil {
				t.Fatal("エラーを期待したが nil")
			}
			if !strings.Contains(err.Error(), tt.wantKey) {
				t.Errorf("エラーメッセージに %q が含まれない: %v", tt.wantKey, err)
			}
		})
	}
}

func TestLoad_MCPInvalidTimezone(t *testing.T) {
	content := baseConfigYAML + `mcp:
  cf_access_team_domain: example.cloudflareaccess.com
  cf_access_aud: aud-123
  timezone: "Mars/Base"
`
	_, err := Load(writeTestConfig(t, content))
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

func TestLoad_MCPReadonlyOverride(t *testing.T) {
	content := baseConfigYAML + `mcp:
  cf_access_team_domain: example.cloudflareaccess.com
  cf_access_aud: aud-123
  readonly_folders: [Foo]
`
	cfg, err := Load(writeTestConfig(t, content))
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}
	want := []string{"Foo"}
	if !reflect.DeepEqual(cfg.MCP.ReadonlyFolders, want) {
		t.Errorf("MCP.ReadonlyFolders = %v, want %v", cfg.MCP.ReadonlyFolders, want)
	}
}

func TestLoad_MCPReadonlyNormalize(t *testing.T) {
	content := baseConfigYAML + `mcp:
  cf_access_team_domain: example.cloudflareaccess.com
  cf_access_aud: aud-123
  readonly_folders: [" Foo ", "/Bar/", "Baz"]
`
	cfg, err := Load(writeTestConfig(t, content))
	if err != nil {
		t.Fatalf("Load失敗: %v", err)
	}
	want := []string{"Foo", "Bar", "Baz"}
	if !reflect.DeepEqual(cfg.MCP.ReadonlyFolders, want) {
		t.Errorf("MCP.ReadonlyFolders = %v, want %v", cfg.MCP.ReadonlyFolders, want)
	}
}

func TestLoad_MCPReadonlyInvalid(t *testing.T) {
	for _, entry := range []string{`""`, `"  "`, `"/"`, `"a/b"`, `"/a/b/"`} {
		content := baseConfigYAML + `mcp:
  cf_access_team_domain: example.cloudflareaccess.com
  cf_access_aud: aud-123
  readonly_folders: [Foo, ` + entry + `]
`
		_, err := Load(writeTestConfig(t, content))
		if err == nil {
			t.Errorf("entry %s: エラーを期待したが nil", entry)
			continue
		}
		if !strings.Contains(err.Error(), "readonly_folders") || !strings.Contains(err.Error(), entry) {
			t.Errorf("entry %s: error should name the entry: %v", entry, err)
		}
	}
}
