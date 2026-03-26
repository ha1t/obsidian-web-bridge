package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/ha1t/obsidian-web-bridge/client"
	"github.com/spf13/cobra"
)

var (
	createPath  string
	createTitle string
	createTags  []string
	createBody  string
	createFile  string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "ノートを作成",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := resolveBody(createFile, createBody)
		if err != nil {
			return err
		}

		fm := buildFrontmatter(createTitle, createTags)

		c := client.New(getServerAddr(), getAPIKey())
		if err := c.Create(createPath, fm, body); err != nil {
			return err
		}
		fmt.Printf("Created: %s\n", createPath)
		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&createPath, "path", "", "ノートのパス (必須)")
	createCmd.MarkFlagRequired("path")
	createCmd.Flags().StringVar(&createTitle, "title", "", "タイトル (frontmatter)")
	createCmd.Flags().StringSliceVar(&createTags, "tag", nil, "タグ (複数指定可)")
	createCmd.Flags().StringVar(&createBody, "body", "", "本文")
	createCmd.Flags().StringVar(&createFile, "file", "", "本文をファイルから読み込み")
	rootCmd.AddCommand(createCmd)
}

func resolveBody(filePath, bodyFlag string) (string, error) {
	// 優先順位: --file > stdin > --body
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("ファイル読み込み失敗: %w", err)
		}
		return string(data), nil
	}

	// stdinにデータがあるか確認
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("stdin読み込み失敗: %w", err)
		}
		return string(data), nil
	}

	if bodyFlag != "" {
		return bodyFlag, nil
	}

	return "", fmt.Errorf("本文を指定してください (--body, --file, またはstdin)")
}

func buildFrontmatter(title string, tags []string) map[string]interface{} {
	fm := make(map[string]interface{})
	if title != "" {
		fm["title"] = title
	}
	if len(tags) > 0 {
		tagList := make([]interface{}, len(tags))
		for i, t := range tags {
			tagList[i] = t
		}
		fm["tags"] = tagList
	}
	if len(fm) == 0 {
		return nil
	}
	return fm
}
