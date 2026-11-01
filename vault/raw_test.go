package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCleanPath(t *testing.T) {
	v := New(t.TempDir())
	ok := map[string]string{
		"./Notes/a.md":   "Notes/a.md",
		"Notes//a.md":    "Notes/a.md",
		"Notes\\a.md":    "Notes/a.md",
		"a.md":           "a.md",
		"Notes/sub/b.md": "Notes/sub/b.md",
	}
	for in, want := range ok {
		got, err := v.CleanPath(in)
		if err != nil || got != want {
			t.Errorf("CleanPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{"../a.md", "Notes/../../a.md", "DailyNotes/../Notes/a.md", "/etc/a.md",
		".obsidian/a.md", "Notes/.trash/a.md", "Notes/a.txt", "", "Notes/.md", "\\etc\\a.md", "Notes/"}
	for _, in := range bad {
		if got, err := v.CleanPath(in); err == nil {
			t.Errorf("CleanPath(%q) = %q; want error", in, got)
		}
	}
}

func TestReadRaw_HashAndContent(t *testing.T) {
	v := New(t.TempDir())
	content := "---\nb: 1\n---\n\n本文\r\n"
	if err := v.CreateRaw("Notes/a.md", content); err != nil {
		t.Fatal(err)
	}
	n, err := v.ReadRaw("./Notes/a.md")
	if err != nil {
		t.Fatal(err)
	}
	if n.Content != content || n.Hash != HashContent(content) || n.Path != "Notes/a.md" || n.Modified.IsZero() {
		t.Errorf("unexpected note: %+v", n)
	}
}

func TestReadRaw_NotFound(t *testing.T) {
	v := New(t.TempDir())
	if _, err := v.ReadRaw("Notes/none.md"); !IsErrNotFound(err) {
		t.Errorf("want not found, got %v", err)
	}
}

func TestReadRaw_SymlinkOutside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0644)
	os.MkdirAll(filepath.Join(root, "Notes"), 0755)
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "Notes/out.md")); err != nil {
		t.Skip(err)
	}
	v := New(root)
	if _, err := v.ReadRaw("Notes/out.md"); err == nil {
		t.Error("want error for symlink escaping vault")
	}
	if err := v.ReplaceRaw("Notes/out.md", "x", HashContent("secret")); err == nil {
		t.Error("want error for replace through symlink escaping vault")
	}
	if b, _ := os.ReadFile(filepath.Join(outside, "secret.md")); string(b) != "secret" {
		t.Error("outside file modified")
	}
}

func TestCreateRaw_Exists(t *testing.T) {
	v := New(t.TempDir())
	if err := v.CreateRaw("a.md", "x"); err != nil {
		t.Fatal(err)
	}
	if err := v.CreateRaw("a.md", "y"); !IsErrAlreadyExists(err) {
		t.Errorf("want already exists, got %v", err)
	}
	n, _ := v.ReadRaw("a.md")
	if n.Content != "x" {
		t.Error("content overwritten")
	}
}

func TestCreateRaw_SymlinkDirOutside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "Link")); err != nil {
		t.Skip(err)
	}
	v := New(root)
	if err := v.CreateRaw("Link/sub/a.md", "x"); err == nil {
		t.Error("want error")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("outside dir polluted: %v", entries)
	}
}

func TestCreateRaw_MakesDirs(t *testing.T) {
	root := t.TempDir()
	v := New(root)
	if err := v.CreateRaw("Notes/sub/a.md", "x"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "Notes/sub/a.md")); err != nil || string(b) != "x" {
		t.Errorf("got %q, %v", b, err)
	}
}

func TestCaseConflictDir(t *testing.T) {
	root := t.TempDir()
	v := New(root)
	if err := os.MkdirAll(filepath.Join(root, "Notes", "Sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Readme"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"a.md":                "",
		"Notes/a.md":          "",
		"Notes/Sub/a.md":      "",
		"Notes/New/x/a.md":    "",
		"notes/a.md":          "Notes",
		"NOTES/x/y/a.md":      "Notes",
		"Notes/sub/a.md":      "Notes/Sub",
		"Notes/SUB/deep/a.md": "Notes/Sub",
		"readme/a.md":         "", // 同名のファイルはフォルダの衝突として扱わない
	}
	for p, want := range cases {
		got, err := v.CaseConflictDir(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", p, got, want)
		}
	}
}

func TestAppendRaw_PreservesFrontmatter(t *testing.T) {
	v := New(t.TempDir())
	fm := "---\nb: 1\na: 2\nmodified: 2026-01-01 10:00\n---\n"
	orig := fm + "\n本文"
	v.CreateRaw("a.md", orig)
	if err := v.AppendRaw("a.md", "追記", ""); err != nil {
		t.Fatal(err)
	}
	n, _ := v.ReadRaw("a.md")
	if !strings.HasPrefix(n.Content, fm) || !strings.Contains(n.Content, "本文") || !strings.Contains(n.Content, "追記") {
		t.Errorf("got %q", n.Content)
	}
}

func TestAppendRaw_Heading(t *testing.T) {
	v := New(t.TempDir())
	v.CreateRaw("a.md", "## Memo\n\nfoo\n\n## Other\n\nbar\n")
	if err := v.AppendRaw("a.md", "NEW", "## Memo"); err != nil {
		t.Fatal(err)
	}
	n, _ := v.ReadRaw("a.md")
	i, j, k := strings.Index(n.Content, "NEW"), strings.Index(n.Content, "## Other"), strings.Index(n.Content, "foo")
	if !(k < i && i < j) {
		t.Errorf("got %q", n.Content)
	}
}

func TestAppendRaw_HeadingNotFound(t *testing.T) {
	v := New(t.TempDir())
	v.CreateRaw("a.md", "## Memo\n")
	err := v.AppendRaw("a.md", "x", "## Nope")
	if !errors.Is(err, ErrHeadingNotFound) {
		t.Errorf("got %v", err)
	}
	n, _ := v.ReadRaw("a.md")
	if n.Content != "## Memo\n" {
		t.Error("file changed")
	}
}

func TestAppendRaw_NotFound(t *testing.T) {
	v := New(t.TempDir())
	if err := v.AppendRaw("none.md", "x", ""); !IsErrNotFound(err) {
		t.Errorf("got %v", err)
	}
}

func TestReplaceRaw_HashMatch(t *testing.T) {
	v := New(t.TempDir())
	v.CreateRaw("a.md", "old")
	if err := v.ReplaceRaw("a.md", "new", HashContent("old")); err != nil {
		t.Fatal(err)
	}
	n, _ := v.ReadRaw("a.md")
	if n.Content != "new" {
		t.Errorf("got %q", n.Content)
	}
}

func TestReplaceRaw_HashMismatch(t *testing.T) {
	v := New(t.TempDir())
	v.CreateRaw("a.md", "old")
	err := v.ReplaceRaw("a.md", "new", HashContent("other"))
	if !errors.Is(err, ErrHashMismatch) {
		t.Errorf("got %v", err)
	}
	n, _ := v.ReadRaw("a.md")
	if n.Content != "old" {
		t.Error("file changed")
	}
	if err := v.ReplaceRaw("none.md", "x", "h"); !IsErrNotFound(err) {
		t.Errorf("got %v", err)
	}
}

func TestUpsertRaw_CreatesThenUpdates(t *testing.T) {
	v := New(t.TempDir())
	upd := func(old string) (string, error) { return old + "+u", nil }
	created, err := v.UpsertRaw("D/a.md", "init", upd)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	n, _ := v.ReadRaw("D/a.md")
	if n.Content != "init+u" {
		t.Errorf("got %q", n.Content)
	}
	created, err = v.UpsertRaw("D/a.md", "init", upd)
	if err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	n, _ = v.ReadRaw("D/a.md")
	if n.Content != "init+u+u" {
		t.Errorf("got %q", n.Content)
	}
}

func TestUpsertRaw_UpdateErrorRemovesNewFile(t *testing.T) {
	v := New(t.TempDir())
	boom := errors.New("boom")
	_, err := v.UpsertRaw("a.md", "init", func(string) (string, error) { return "", boom })
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if _, err := v.ReadRaw("a.md"); !IsErrNotFound(err) {
		t.Errorf("new file should be removed, got %v", err)
	}
	v.CreateRaw("b.md", "keep")
	v.UpsertRaw("b.md", "init", func(string) (string, error) { return "", boom })
	if n, _ := v.ReadRaw("b.md"); n == nil || n.Content != "keep" {
		t.Error("existing file must be untouched")
	}
}

func TestWrite_NoTempLeftover(t *testing.T) {
	root := t.TempDir()
	v := New(root)
	v.CreateRaw("Notes/a.md", "x")
	v.AppendRaw("Notes/a.md", "y", "")
	v.ReplaceRaw("Notes/a.md", "z", HashContent(mustRead(t, v, "Notes/a.md")))
	m, _ := filepath.Glob(filepath.Join(root, "Notes", ".owb-*.tmp"))
	if len(m) != 0 {
		t.Errorf("leftover: %v", m)
	}
}

func TestWrite_PreservesMode(t *testing.T) {
	root := t.TempDir()
	v := New(root)
	v.CreateRaw("a.md", "x")
	os.Chmod(filepath.Join(root, "a.md"), 0600)
	v.AppendRaw("a.md", "y", "")
	fi, _ := os.Stat(filepath.Join(root, "a.md"))
	if fi.Mode().Perm() != 0600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func mustRead(t *testing.T, v *Vault, p string) string {
	t.Helper()
	n, err := v.ReadRaw(p)
	if err != nil {
		t.Fatal(err)
	}
	return n.Content
}

func TestAppendRaw_Concurrent(t *testing.T) {
	v := New(t.TempDir())
	v.CreateRaw("a.md", "start\n")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := v.AppendRaw("a.md", fmt.Sprintf("line-%02d", i), ""); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	c := mustRead(t, v, "a.md")
	for i := 0; i < 20; i++ {
		if !strings.Contains(c, fmt.Sprintf("line-%02d", i)) {
			t.Errorf("missing line %d", i)
		}
	}
}
