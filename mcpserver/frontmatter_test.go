package mcpserver

import (
	"reflect"
	"testing"
)

func TestBuildFrontmatter_Defaults(t *testing.T) {
	fm := BuildFrontmatter(nil, "Notes/a.md")
	if !reflect.DeepEqual(fm["tags"], []any{"source/claude-desktop"}) {
		t.Errorf("tags=%v", fm["tags"])
	}
	if fm["source"] != "claude-desktop" {
		t.Errorf("source=%v", fm["source"])
	}
}

func TestBuildFrontmatter_MergeTags(t *testing.T) {
	fm := BuildFrontmatter(map[string]any{"tags": []any{"car"}}, "Notes/a.md")
	if !reflect.DeepEqual(fm["tags"], []any{"car", "source/claude-desktop"}) {
		t.Errorf("tags=%v", fm["tags"])
	}
	fm = BuildFrontmatter(map[string]any{"tags": []any{"source/claude-desktop", "car"}}, "Notes/a.md")
	if !reflect.DeepEqual(fm["tags"], []any{"source/claude-desktop", "car"}) {
		t.Errorf("tags=%v", fm["tags"])
	}
	fm = BuildFrontmatter(map[string]any{"tags": "car"}, "Notes/a.md")
	if !reflect.DeepEqual(fm["tags"], []any{"car", "source/claude-desktop"}) {
		t.Errorf("tags=%v", fm["tags"])
	}
}

func TestBuildFrontmatter_SourceURLWins(t *testing.T) {
	fm := BuildFrontmatter(map[string]any{"source": "https://example.com"}, "Notes/a.md")
	if fm["source"] != "https://example.com" {
		t.Errorf("source=%v", fm["source"])
	}
}

func TestBuildFrontmatter_Clippings(t *testing.T) {
	fm := BuildFrontmatter(nil, "Clippings/a.md")
	if !reflect.DeepEqual(fm["tags"], []any{"source/claude-desktop", "clippings"}) {
		t.Errorf("tags=%v", fm["tags"])
	}
}

func TestBuildFrontmatter_StripsDates(t *testing.T) {
	given := map[string]any{"created": "x", "Modified": "y", "title": "t"}
	fm := BuildFrontmatter(given, "Notes/a.md")
	if _, ok := fm["created"]; ok {
		t.Error("created remains")
	}
	if _, ok := fm["Modified"]; ok {
		t.Error("Modified remains")
	}
	if fm["title"] != "t" {
		t.Error("title lost")
	}
	if _, ok := given["created"]; !ok {
		t.Error("caller map mutated")
	}
}
