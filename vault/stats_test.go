package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeBytes(t *testing.T, root, rel string, n int) {
	t.Helper()
	writeNote(t, root, rel, strings.Repeat("x", n), time.Now())
}

func findFolder(s *Stats, name string) *FolderStats {
	for i := range s.Folders {
		if s.Folders[i].Name == name {
			return &s.Folders[i]
		}
	}
	return nil
}

func TestStats_FoldersAndCategories(t *testing.T) {
	root := t.TempDir()
	writeBytes(t, root, "Notes/a.md", 10)
	writeBytes(t, root, "Notes/sub/b.MD", 5)
	writeBytes(t, root, "Attachments/p.PNG", 100)
	writeBytes(t, root, "Attachments/q.jpeg", 50)
	writeBytes(t, root, "Attachments/doc.pdf", 200)
	writeBytes(t, root, "Attachments/data.zip", 7)
	writeBytes(t, root, "root.md", 3)

	s, err := New(root).Stats()
	if err != nil {
		t.Fatal(err)
	}

	notes := findFolder(s, "Notes")
	if notes == nil || notes.Files != 2 || notes.Bytes != 15 || notes.Markdown != (Count{2, 15}) {
		t.Errorf("Notes = %+v", notes)
	}
	att := findFolder(s, "Attachments")
	want := FolderStats{Name: "Attachments", Count: Count{4, 357}, Image: Count{2, 150}, PDF: Count{1, 200}, Other: Count{1, 7}}
	if att == nil || *att != want {
		t.Errorf("Attachments = %+v, want %+v", att, want)
	}
	rootF := findFolder(s, RootFolderName)
	if rootF == nil || rootF.Files != 1 || rootF.Markdown != (Count{1, 3}) {
		t.Errorf("root = %+v", rootF)
	}
	// サイズの大きい順
	if s.Folders[0].Name != "Attachments" {
		t.Errorf("order = %+v", s.Folders)
	}
	if s.Total != (Count{7, 375}) {
		t.Errorf("Total = %+v", s.Total)
	}
}

func TestStats_GitAndHiddenAreSeparateTotals(t *testing.T) {
	root := t.TempDir()
	writeBytes(t, root, "Notes/a.md", 10)
	writeBytes(t, root, ".git/objects/pack/p.pack", 1000)
	writeBytes(t, root, ".git/HEAD", 20)
	writeBytes(t, root, ".obsidian/app.json", 30)
	writeBytes(t, root, ".gitignore", 4)
	writeBytes(t, root, "Notes/.hidden.md", 6) // フォルダ内のドットファイルはそのフォルダに数える

	s, err := New(root).Stats()
	if err != nil {
		t.Fatal(err)
	}
	if s.Git != (Count{2, 1020}) {
		t.Errorf("Git = %+v", s.Git)
	}
	if s.Hidden != (Count{2, 34}) {
		t.Errorf("Hidden = %+v", s.Hidden)
	}
	for _, f := range s.Folders {
		if strings.HasPrefix(f.Name, ".") {
			t.Errorf("hidden folder listed: %+v", f)
		}
	}
	if n := findFolder(s, "Notes"); n == nil || n.Files != 2 {
		t.Errorf("Notes = %+v", n)
	}
	if s.Total != (Count{6, 1070}) {
		t.Errorf("Total = %+v", s.Total)
	}
}

func TestStats_DoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeBytes(t, root, "Notes/a.md", 10)
	writeBytes(t, outside, "big.bin", 5000)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "big.bin"), filepath.Join(root, "Notes", "s.bin")); err != nil {
		t.Fatal(err)
	}
	s, err := New(root).Stats()
	if err != nil {
		t.Fatal(err)
	}
	if findFolder(s, "link") != nil {
		t.Errorf("symlinked folder listed: %+v", s.Folders)
	}
	if s.Total != (Count{1, 10}) {
		t.Errorf("Total = %+v", s.Total)
	}
}

func TestStats_EmptyFolderListed(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Empty"), 0755); err != nil {
		t.Fatal(err)
	}
	s, err := New(root).Stats()
	if err != nil {
		t.Fatal(err)
	}
	if e := findFolder(s, "Empty"); e == nil || e.Files != 0 {
		t.Errorf("Empty = %+v", e)
	}
}
