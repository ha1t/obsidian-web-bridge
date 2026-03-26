package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverAddr string
	apiKey     string
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
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "", "APIサーバーのアドレス (env: OWB_SERVER)")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "APIキー (env: OWB_API_KEY)")
}

func getServerAddr() string {
	if serverAddr != "" {
		return serverAddr
	}
	if v := os.Getenv("OWB_SERVER"); v != "" {
		return v
	}
	return "http://localhost:18484"
}

func getAPIKey() string {
	if apiKey != "" {
		return apiKey
	}
	return os.Getenv("OWB_API_KEY")
}
