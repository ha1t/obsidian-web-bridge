package vault

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// MarshalNote はfrontmatterとbodyからMarkdown文字列を生成する。
func MarshalNote(frontmatter map[string]interface{}, body string) string {
	var sb strings.Builder

	if len(frontmatter) > 0 {
		sb.WriteString("---\n")
		yamlBytes, err := yaml.Marshal(frontmatter)
		if err == nil {
			sb.Write(yamlBytes)
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString(body)
	return sb.String()
}

// UnmarshalNote はMarkdown文字列をfrontmatterとbodyに分離する。
func UnmarshalNote(content string) (map[string]interface{}, string, error) {
	frontmatter := make(map[string]interface{})

	if !strings.HasPrefix(content, "---\n") {
		return frontmatter, content, nil
	}

	// 2つ目の---を探す
	rest := content[4:] // "---\n" の後
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return frontmatter, content, nil
	}

	yamlStr := rest[:endIdx]
	if err := yaml.Unmarshal([]byte(yamlStr), &frontmatter); err != nil {
		return nil, "", err
	}

	// ---の後の本文を取得（先頭の改行を除去）
	body := rest[endIdx+4:] // "\n---" の後
	body = strings.TrimLeft(body, "\n")

	return frontmatter, body, nil
}
