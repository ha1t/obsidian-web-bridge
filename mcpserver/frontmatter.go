package mcpserver

import "strings"

const (
	sourceTag    = "source/claude-desktop"
	sourceValue  = "claude-desktop"
	clippingsTag = "clippings"
)

// BuildFrontmatter は渡された frontmatter（nil 可）に既定値を補った新しい map を返す。
// created / modified は大文字小文字を区別せず取り除く。渡された map は変更しない。
func BuildFrontmatter(given map[string]any, cleanPath string) map[string]any {
	out := make(map[string]any, len(given)+2)
	for k, v := range given {
		switch strings.ToLower(k) {
		case "created", "modified":
			continue
		}
		out[k] = v
	}

	var tags []any
	switch t := out["tags"].(type) {
	case []any:
		tags = append(tags, t...)
	case []string:
		for _, s := range t {
			tags = append(tags, s)
		}
	case string:
		tags = append(tags, t)
	}
	tags = addTag(tags, sourceTag)
	if first, _, ok := strings.Cut(cleanPath, "/"); ok && strings.EqualFold(first, "Clippings") {
		tags = addTag(tags, clippingsTag)
	}
	out["tags"] = tags

	if _, ok := out["source"]; !ok {
		out["source"] = sourceValue
	}
	return out
}

func addTag(tags []any, tag string) []any {
	for _, t := range tags {
		if s, ok := t.(string); ok && s == tag {
			return tags
		}
	}
	return append(tags, tag)
}
