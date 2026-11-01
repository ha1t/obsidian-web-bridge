package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ha1t/obsidian-web-bridge/config"
	"github.com/ha1t/obsidian-web-bridge/mcpserver"
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

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return RunServers(ctx, cfg)
	},
}

// RunServers は API サーバーと（cfg.MCP があれば）MCP サーバーを起動する。
// ctx が cancel されたら両方を Shutdown する。片方が失敗したらもう片方も止めてエラーを返す。
func RunServers(ctx context.Context, cfg *config.Config) error {
	v := vault.New(cfg.VaultPath)

	type entry struct {
		srv *http.Server
		ln  net.Listener
	}
	var entries []entry
	closeAll := func() {
		for _, e := range entries {
			e.ln.Close()
		}
	}

	apiAddr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	apiLn, err := net.Listen("tcp", apiAddr)
	if err != nil {
		return err
	}
	fmt.Printf("Starting server on %s\n", apiAddr)
	entries = append(entries, entry{&http.Server{Handler: server.NewServer(v, cfg.APIKey).Handler()}, apiLn})

	if m := cfg.MCP; m != nil {
		loc, err := time.LoadLocation(m.Timezone)
		if err != nil {
			closeAll()
			return fmt.Errorf("mcp.timezone が不正です: %w", err)
		}
		verifier := mcpserver.NewAccessVerifier(m.CFAccessTeamDomain, m.CFAccessAUD,
			mcpserver.CertsURL(m.CFAccessTeamDomain), &http.Client{Timeout: 10 * time.Second})
		handler := mcpserver.NewHTTPHandler(mcpserver.Options{
			Vault:    v,
			Policy:   mcpserver.NewPolicy(m.ReadonlyFolders),
			Location: loc,
		}, verifier)

		mcpAddr := fmt.Sprintf("%s:%d", m.Bind, m.Port)
		mcpLn, err := net.Listen("tcp", mcpAddr)
		if err != nil {
			closeAll()
			return err
		}
		fmt.Printf("Starting MCP server on %s (/mcp)\n", mcpAddr)
		entries = append(entries, entry{&http.Server{Handler: handler}, mcpLn})
	}

	errCh := make(chan error, len(entries))
	for _, e := range entries {
		go func() { errCh <- e.srv.Serve(e.ln) }()
	}

	var result error
	select {
	case <-ctx.Done():
	case result = <-errCh:
		if errors.Is(result, http.ErrServerClosed) {
			result = nil
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, e := range entries {
		e.srv.Shutdown(shutdownCtx)
	}
	return result
}

func init() {
	serverCmd.Flags().StringVar(&configPath, "config", "", "設定ファイルパス")
	serverCmd.Flags().StringVar(&vaultPath, "vault", "", "Obsidian vaultのパス")
	serverCmd.Flags().IntVar(&port, "port", 18484, "ポート番号")
	serverCmd.Flags().StringVar(&serverKey, "server-api-key", "", "APIキー")
	serverCmd.Flags().StringVar(&bindAddress, "bind", "0.0.0.0", "バインドアドレス")
	rootCmd.AddCommand(serverCmd)
}
