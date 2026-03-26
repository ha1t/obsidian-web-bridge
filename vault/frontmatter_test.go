package vault

import (
	"testing"
)

func TestMarshalFrontmatter(t *testing.T) {
	fm := map[string]interface{}{
		"title":   "テストノート",
		"created": "2026-03-26 18:00",
		"tags":    []interface{}{"daily-report", "test"},
	}
	body := "## 本文\n\nテスト内容です。"

	result := MarshalNote(fm, body)

	// frontmatterブロックが含まれること
	if len(result) < 3 || result[:3] != "---" {
		t.Fatalf("frontmatterブロックが見つからない: %s", result)
	}

	// bodyが含まれること
	if !containsString(result, "## 本文") {
		t.Fatalf("bodyが含まれていない: %s", result)
	}

	if !containsString(result, "テスト内容です。") {
		t.Fatalf("body内容が含まれていない: %s", result)
	}
}

func TestUnmarshalFrontmatter(t *testing.T) {
	content := `---
title: テストノート
created: "2026-03-26 18:00"
tags:
  - daily-report
  - test
---

## 本文

テスト内容です。`

	fm, body, err := UnmarshalNote(content)
	if err != nil {
		t.Fatalf("UnmarshalNote失敗: %v", err)
	}

	if fm["title"] != "テストノート" {
		t.Errorf("title = %v, want テストノート", fm["title"])
	}

	tags, ok := fm["tags"].([]interface{})
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want [daily-report, test]", fm["tags"])
	}

	if body != "## 本文\n\nテスト内容です。" {
		t.Errorf("body = %q, want %q", body, "## 本文\n\nテスト内容です。")
	}
}

func TestUnmarshalNoFrontmatter(t *testing.T) {
	content := "## ただのMarkdown\n\n本文のみ。"

	fm, body, err := UnmarshalNote(content)
	if err != nil {
		t.Fatalf("UnmarshalNote失敗: %v", err)
	}

	if len(fm) != 0 {
		t.Errorf("fm = %v, want empty map", fm)
	}

	if body != content {
		t.Errorf("body = %q, want %q", body, content)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
