package cmd

import (
	"fmt"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var readPath string

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "ノートを取得",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.New(getServerAddr(), getAPIKey())
		fm, body, err := c.Read(readPath)
		if err != nil {
			return err
		}

		// frontmatterがあれば表示
		if len(fm) > 0 {
			fmt.Println("---")
			yamlBytes, _ := yaml.Marshal(fm)
			fmt.Print(string(yamlBytes))
			fmt.Println("---")
			fmt.Println()
		}

		fmt.Print(body)
		return nil
	},
}

func init() {
	readCmd.Flags().StringVar(&readPath, "path", "", "ノートのパス (必須)")
	readCmd.MarkFlagRequired("path")
	rootCmd.AddCommand(readCmd)
}
