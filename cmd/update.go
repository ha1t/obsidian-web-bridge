package cmd

import (
	"fmt"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
)

var (
	updatePath  string
	updateTitle string
	updateTags  []string
	updateBody  string
	updateFile  string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "ノートを更新",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := resolveBody(updateFile, updateBody)
		if err != nil {
			return err
		}

		fm := buildFrontmatter(updateTitle, updateTags)

		c := client.New(getServerAddr(), getAPIKey())
		if err := c.Update(updatePath, fm, body); err != nil {
			return err
		}
		fmt.Printf("Updated: %s\n", updatePath)
		return nil
	},
}

func init() {
	updateCmd.Flags().StringVar(&updatePath, "path", "", "ノートのパス (必須)")
	updateCmd.MarkFlagRequired("path")
	updateCmd.Flags().StringVar(&updateTitle, "title", "", "タイトル (frontmatter)")
	updateCmd.Flags().StringSliceVar(&updateTags, "tag", nil, "タグ (複数指定可)")
	updateCmd.Flags().StringVar(&updateBody, "body", "", "本文")
	updateCmd.Flags().StringVar(&updateFile, "file", "", "本文をファイルから読み込み")
	rootCmd.AddCommand(updateCmd)
}
