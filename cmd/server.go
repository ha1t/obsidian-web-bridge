package cmd

import (
	"fmt"
	"os"

	"github.com/ha1t/obsidian-web-bridge/config"
	"github.com/ha1t/obsidian-web-bridge/server"
	"github.com/ha1t/obsidian-web-bridge/vault"
	"github.com/spf13/cobra"
)

var (
	configPath  string
	vaultPath   string
	port        int
	serverKey   string
	bindAddress string
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "APIサーバーを起動",
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *config.Config
		var err error

		if configPath != "" {
			cfg, err = config.Load(configPath)
			if err != nil {
				return fmt.Errorf("設定ファイル読み込み失敗: %w", err)
			}
		} else {
			cfg = &config.Config{
				VaultPath: vaultPath,
				Port:      port,
				APIKey:    serverKey,
				Bind:      bindAddress,
			}
		}

		// フラグで上書き
		if cmd.Flags().Changed("vault") {
			cfg.VaultPath = vaultPath
		}
		if cmd.Flags().Changed("port") {
			cfg.Port = port
		}
		if cmd.Flags().Changed("api-key") && serverKey != "" {
			cfg.APIKey = serverKey
		}
		if cmd.Flags().Changed("bind") {
			cfg.Bind = bindAddress
		}

		if cfg.VaultPath == "" {
			return fmt.Errorf("vault pathを指定してください (--vault または config)")
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("API keyを指定してください (--api-key または config)")
		}
		if cfg.Port == 0 {
			cfg.Port = 18484
		}
		if cfg.Bind == "" {
			cfg.Bind = "0.0.0.0"
		}

		// vault存在確認
		if _, err := os.Stat(cfg.VaultPath); os.IsNotExist(err) {
			return fmt.Errorf("vault pathが存在しません: %s", cfg.VaultPath)
		}

		v := vault.New(cfg.VaultPath)
		s := server.NewServer(v, cfg.APIKey)
		return s.ListenAndServe(cfg.Bind, cfg.Port)
	},
}

func init() {
	serverCmd.Flags().StringVar(&configPath, "config", "", "設定ファイルパス")
	serverCmd.Flags().StringVar(&vaultPath, "vault", "", "Obsidian vaultのパス")
	serverCmd.Flags().IntVar(&port, "port", 18484, "ポート番号")
	serverCmd.Flags().StringVar(&serverKey, "server-api-key", "", "APIキー")
	serverCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0", "バインドアドレス")
	rootCmd.AddCommand(serverCmd)
}
