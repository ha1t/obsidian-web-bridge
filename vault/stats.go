package vault

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// RootFolderName は Vault 直下に置かれたファイルをまとめる Stats 上のフォルダ名。
const RootFolderName = "(root)"

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true,
	".bmp": true, ".heic": true, ".heif": true, ".avif": true, ".tif": true, ".tiff": true,
}

// Count はファイル数と合計バイト数（見かけのサイズ）。
type Count struct {
	Files int
	Bytes int64
}

func (c *Count) add(size int64) {
	c.Files++
	c.Bytes += size
}

// FolderStats は Vault 直下のフォルダ1つ分の集計。Count はフォルダ全体。
type FolderStats struct {
	Name string
	Count
	Markdown Count
	Image    Count
	PDF      Count
	Other    Count
}

func (f *FolderStats) add(name string, size int64) {
	f.Count.add(size)
	switch ext := strings.ToLower(filepath.Ext(name)); {
	case ext == ".md":
		f.Markdown.add(size)
	case imageExts[ext]:
		f.Image.add(size)
	case ext == ".pdf":
		f.PDF.add(size)
	default:
		f.Other.add(size)
	}
}

// Stats は Vault 全体の容量の集計。
type Stats struct {
	Folders []FolderStats // 直下のフォルダごと（ドットで始まるものを除く）。サイズの大きい順
	Git     Count         // .git の合計
	Hidden  Count         // .git 以外のドットで始まる直下のエントリの合計
	Total   Count         // 上記すべての合計
}

// Stats は Vault 全体を走査して、直下のフォルダごとの件数とサイズを集計する。
// シンボリックリンクは辿らず、数えない。読めないエントリは飛ばす。
func (v *Vault) Stats() (*Stats, error) {
	root, err := filepath.EvalSymlinks(v.rootPath)
	if err != nil {
		return nil, err
	}
	folders := map[string]*FolderStats{}
	s := &Stats{}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		if p == root || d.Type()&fs.ModeSymlink != 0 {
			return nil // WalkDir はシンボリックリンクのディレクトリに入らない
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		first, rest, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if d.IsDir() {
			if rest == "" && !strings.HasPrefix(first, ".") && folders[first] == nil {
				folders[first] = &FolderStats{Name: first}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := info.Size()
		s.Total.add(size)
		switch {
		case first == ".git":
			s.Git.add(size)
		case strings.HasPrefix(first, "."):
			s.Hidden.add(size)
		default:
			name := first
			if rest == "" {
				name = RootFolderName
			}
			f := folders[name]
			if f == nil {
				f = &FolderStats{Name: name}
				folders[name] = f
			}
			f.add(d.Name(), size)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		s.Folders = append(s.Folders, *f)
	}
	sort.Slice(s.Folders, func(i, j int) bool {
		a, b := s.Folders[i], s.Folders[j]
		if a.Bytes != b.Bytes {
			return a.Bytes > b.Bytes
		}
		return a.Name < b.Name
	})
	return s, nil
}
