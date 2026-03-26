package cmd

import (
	"fmt"
	"os"

	"github.com/ha1t/obsidian-web-bridge/config"
	"github.com/spf13/cobra"
)

var (
	serverAddr    string
	apiKey        string
	clientCfgPath string
)

var rootCmd = &cobra.Command{
	Use:   "obsidian-web-bridge",
	Short: "Obsidian vault操作用のAPIサーバー & CLI",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&clientCfgPath, "config", "", "設定ファイルパス (デフォルト: config.yaml)")
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "", "APIサーバーのアドレス (env: OWB_SERVER)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "APIキー (env: OWB_API_KEY)")
}

func loadClientConfig() *config.Config {
	path := clientCfgPath
	if path == "" {
		path = "config.yaml"
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil
	}
	return cfg
}

func getServerAddr() string {
	if serverAddr != "" {
		return serverAddr
	}
	if v := os.Getenv("OWB_SERVER"); v != "" {
		return v
	}
	if cfg := loadClientConfig(); cfg != nil {
		if cfg.Server != "" {
			return cfg.Server
		}
		return fmt.Sprintf("http://localhost:%d", cfg.Port)
	}
	return "http://localhost:18484"
}

func getAPIKey() string {
	if apiKey != "" {
		return apiKey
	}
	if v := os.Getenv("OWB_API_KEY"); v != "" {
		return v
	}
	if cfg := loadClientConfig(); cfg != nil {
		return cfg.APIKey
	}
	return ""
}
