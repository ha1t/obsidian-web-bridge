package cmd

import (
	"fmt"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
)

var deletePath string

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "ノートを削除",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(getServerAddr(), getAPIKey())
		if err := c.Delete(deletePath); err != nil {
			return err
		}
		fmt.Printf("Deleted: %s\n", deletePath)
		return nil
	},
}

func init() {
	deleteCmd.Flags().StringVar(&deletePath, "path", "", "ノートのパス (必須)")
	deleteCmd.MarkFlagRequired("path")
	rootCmd.AddCommand(deleteCmd)
}
