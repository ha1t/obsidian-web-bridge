package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	VaultPath string     `yaml:"vault_path"`
	Port      int        `yaml:"port"`
	APIKey    string     `yaml:"api_key"`
	Bind      string     `yaml:"bind"`
	Server    string     `yaml:"server"`
	MCP       *MCPConfig `yaml:"mcp"`
}

// MCPConfig はリモートMCPエンドポイントの設定。mcp セクションが無い場合は nil になる。
type MCPConfig struct {
	Bind               string   `yaml:"bind"`
	Port               int      `yaml:"port"`
	CFAccessTeamDomain string   `yaml:"cf_access_team_domain"`
	CFAccessAUD        string   `yaml:"cf_access_aud"`
	Timezone           string   `yaml:"timezone"`
	ReadonlyFolders    []string `yaml:"readonly_folders"`
}

// DefaultReadonlyFolders は readonly_folders 未指定時に使う読み取り専用フォルダ。
var DefaultReadonlyFolders = []string{"DailyReports", "MorningBrief", "Calendar", "Garmin", "MonthlyReview", "Templates", "bases"}

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

	if cfg.MCP != nil {
		if err := applyMCPDefaults(cfg.MCP); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func applyMCPDefaults(m *MCPConfig) error {
	if m.CFAccessTeamDomain == "" {
		return fmt.Errorf("mcp.cf_access_team_domain is required")
	}
	if m.CFAccessAUD == "" {
		return fmt.Errorf("mcp.cf_access_aud is required")
	}

	if m.Bind == "" {
		m.Bind = "127.0.0.1"
	}
	if m.Port == 0 {
		m.Port = 18485
	}
	if m.Timezone == "" {
		m.Timezone = "Asia/Tokyo"
	}
	if _, err := time.LoadLocation(m.Timezone); err != nil {
		return fmt.Errorf("mcp.timezone is invalid: %w", err)
	}
	if m.ReadonlyFolders == nil {
		m.ReadonlyFolders = append([]string(nil), DefaultReadonlyFolders...)
	}
	folders, err := normalizeReadonlyFolders(m.ReadonlyFolders)
	if err != nil {
		return err
	}
	m.ReadonlyFolders = folders

	return nil
}

// normalizeReadonlyFolders は前後の空白と / を取り除く。
// 空になる項目や、途中に / を含む項目（Vault 直下のフォルダ名でないもの）はエラーにする。
func normalizeReadonlyFolders(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, f := range in {
		n := strings.Trim(strings.TrimSpace(f), "/")
		if n == "" || strings.Contains(n, "/") {
			return nil, fmt.Errorf("mcp.readonly_folders: invalid entry %q (must be a top-level folder name)", f)
		}
		out = append(out, n)
	}
	return out, nil
}
