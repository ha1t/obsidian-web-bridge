package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeNote(t *testing.T, root, rel, content string, mod time.Time) {
	t.Helper()
	fp := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fp, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func paths(ns []NoteInfo) string {
	var r []string
	for _, n := range ns {
		r = append(r, n.Path)
	}
	return strings.Join(r, ",")
}

func TestList_SortedByMtime(t *testing.T) {
	root := t.TempDir()
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	writeNote(t, root, "old.md", "x", base)
	writeNote(t, root, "new.md", "x", base.Add(2*time.Minute))
	writeNote(t, root, "mid.md", "x", base.Add(time.Minute))
	writeNote(t, root, "mid2.md", "x", base.Add(time.Minute))
	writeNote(t, root, "ignore.txt", "x", base.Add(30*time.Minute))
	got, err := New(root).List("", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := "new.md,mid.md,mid2.md,old.md"; paths(got) != want {
		t.Errorf("got %v want %v", paths(got), want)
	}
	if got[0].Size != 1 {
		t.Errorf("size = %d", got[0].Size)
	}
}

func TestList_NonRecursiveAndRecursive(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	writeNote(t, root, "N/a.md", "x", now)
	writeNote(t, root, "N/sub/b.md", "x", now.Add(-time.Minute))
	v := New(root)
	got, err := v.List("N", false, 0)
	if err != nil || paths(got) != "N/a.md" {
		t.Errorf("non-recursive: %v %v", paths(got), err)
	}
	got, err = v.List("N", true, 0)
	if err != nil || paths(got) != "N/a.md,N/sub/b.md" {
		t.Errorf("recursive: %v %v", paths(got), err)
	}
}

func TestList_Limit(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	for i, n := range []string{"a", "b", "c"} {
		writeNote(t, root, n+".md", "x", now.Add(time.Duration(i)*time.Minute))
	}
	got, _ := New(root).List("", false, 2)
	if paths(got) != "c.md,b.md" {
		t.Errorf("got %v", paths(got))
	}
}

func TestList_SkipsDotDirsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	now := time.Now()
	writeNote(t, root, ".obsidian/x.md", "x", now)
	writeNote(t, root, "a.md", "x", now)
	writeNote(t, outside, "secret.md", "x", now)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "s.md")); err != nil {
		t.Fatal(err)
	}
	got, err := New(root).List("", true, 0)
	if err != nil || paths(got) != "a.md" {
		t.Errorf("got %v %v", paths(got), err)
	}
	if _, err := New(root).List(".obsidian", true, 0); err == nil {
		t.Error("dot folder should be rejected")
	}
	if _, err := New(root).List("link", true, 0); err == nil {
		t.Error("symlink escaping folder should be rejected")
	}
}

func TestList_NotFound(t *testing.T) {
	_, err := New(t.TempDir()).List("Nope", true, 0)
	if !IsErrNotFound(err) {
		t.Errorf("err = %v", err)
	}
}

func TestSearch_BodyAndFilename(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	writeNote(t, root, "body.md", "hello\nthe needle here\nbye", now)
	writeNote(t, root, "needle-name.md", "nothing", now.Add(-time.Minute))
	writeNote(t, root, "other.md", "nothing", now)
	got, err := New(root).Search("needle", "", 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v %v", got, err)
	}
	if got[0].Path != "body.md" || got[0].Excerpt == "" {
		t.Errorf("hit0 = %+v", got[0])
	}
	if got[1].Path != "needle-name.md" || got[1].Excerpt != "" {
		t.Errorf("hit1 = %+v", got[1])
	}
	if _, err := New(root).Search("", "", 0); err == nil {
		t.Error("empty query should fail")
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "talked to claude today", time.Now())
	got, _ := New(root).Search("Claude", "", 0)
	if len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestSearch_NFDFilename(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "ガ.md", "x", time.Now()) // NFD の「ガ」
	got, _ := New(root).Search("ガ", "", 0)
	if len(got) != 1 {
		t.Errorf("got %v", got)
	}
}

func TestSearch_FolderFilterAndLimit(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	writeNote(t, root, "A/x.md", "needle", now)
	writeNote(t, root, "B/y.md", "needle", now)
	writeNote(t, root, "B/z.md", "needle", now.Add(-time.Minute))
	writeNote(t, root, ".obsidian/h.md", "needle", now)
	v := New(root)
	got, _ := v.Search("needle", "B", 0)
	if len(got) != 2 || got[0].Path != "B/y.md" {
		t.Errorf("got %v", got)
	}
	got, _ = v.Search("needle", "", 0)
	if len(got) != 3 {
		t.Errorf("dot dir leaked: %v", got)
	}
	got, _ = v.Search("needle", "", 1)
	if len(got) != 1 {
		t.Errorf("limit: %v", got)
	}
	if _, err := v.Search("needle", "Nope", 0); !IsErrNotFound(err) {
		t.Errorf("err = %v", err)
	}
}

func TestSearch_ExcerptLines(t *testing.T) {
	root := t.TempDir()
	writeNote(t, root, "a.md", "l1\nl2 hit\nl3\nl4 hit", time.Now())
	got, _ := New(root).Search("hit", "", 0)
	if got[0].Excerpt != "l1\nl2 hit\nl3" {
		t.Errorf("excerpt = %q", got[0].Excerpt)
	}
	writeNote(t, root, "b.md", "hit first\nl2", time.Now().Add(-time.Hour))
	got, _ = New(root).Search("first", "", 0)
	if got[0].Excerpt != "hit first\nl2" {
		t.Errorf("excerpt = %q", got[0].Excerpt)
	}
}

func TestSearch_LongLineAndLargeFile(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("あ", 500) + "NEEDLE" + strings.Repeat("い", 500)
	writeNote(t, root, "a.md", long, time.Now())
	got, _ := New(root).Search("needle", "", 0)
	if len(got) != 1 || len([]rune(got[0].Excerpt)) > 200 || !strings.Contains(got[0].Excerpt, "NEEDLE") {
		t.Errorf("got %+v", got)
	}
	writeNote(t, root, "big.md", strings.Repeat("x", 1<<20+1)+"needle", time.Now())
	got, _ = New(root).Search("needle", "", 0)
	if len(got) != 1 {
		t.Errorf("large file body should be skipped: %v", got)
	}
}
