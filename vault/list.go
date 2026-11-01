package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	maxSearchFileSize = 1 << 20 // これを超えるファイルは本文検索の対象外
	maxExcerptRunes   = 200
)

// NoteInfo は一覧用のノート情報。
type NoteInfo struct {
	Path     string // Vault ルートからのスラッシュ区切り相対パス
	Modified time.Time
	Size     int64
}

// SearchHit は検索ヒット。
type SearchHit struct {
	Path     string
	Modified time.Time
	Excerpt  string // ファイル名だけが一致したときは ""
}

// walkNotes は folder 配下の .md を走査して fn に渡す。
// ドットで始まるエントリとシンボリックリンクは辿らない・無視する。
func (v *Vault) walkNotes(folder string, recursive bool, fn func(rel, fullPath string, info fs.FileInfo) error) error {
	clean, err := v.cleanFolder(folder)
	if err != nil {
		return err
	}
	fp, err := v.resolve(clean)
	if err != nil {
		return err
	}
	// フォルダ自体がシンボリックリンクでも、resolve で Vault 内と確認済みの実パスを走査する
	realFP, err := filepath.EvalSymlinks(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", errNotFound, folder)
		}
		return err
	}
	st, err := os.Stat(realFP)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", errNotFound, folder)
		}
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%w: not a directory: %s", errNotFound, folder)
	}

	return filepath.WalkDir(realFP, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == realFP {
				return err
			}
			return nil // 読めないエントリは飛ばす
		}
		if p == realFP {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || path.Ext(name) != ".md" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		sub, err := filepath.Rel(realFP, p)
		if err != nil {
			return nil
		}
		rel := filepath.ToSlash(sub)
		if clean != "" {
			rel = clean + "/" + rel
		}
		return fn(rel, p, info)
	})
}

// List は folder 配下のノートを更新日時の降順（同時刻はパス昇順）で返す。limit <= 0 は無制限。
func (v *Vault) List(folder string, recursive bool, limit int) ([]NoteInfo, error) {
	notes := []NoteInfo{}
	err := v.walkNotes(folder, recursive, func(rel, _ string, info fs.FileInfo) error {
		notes = append(notes, NoteInfo{Path: rel, Modified: info.ModTime(), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(notes, func(i, j int) bool {
		if !notes[i].Modified.Equal(notes[j].Modified) {
			return notes[i].Modified.After(notes[j].Modified)
		}
		return notes[i].Path < notes[j].Path
	})
	if limit > 0 && len(notes) > limit {
		notes = notes[:limit]
	}
	return notes, nil
}

func fold(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// Search は folder 配下を再帰的に探し、ファイル名か本文（行単位）に query を含むノートを
// 更新日時の降順で返す。大文字小文字と NFC/NFD の差は無視する。limit <= 0 は無制限。
func (v *Vault) Search(query, folder string, limit int) ([]SearchHit, error) {
	q := fold(query)
	if q == "" {
		return nil, fmt.Errorf("query is empty")
	}
	hits := []SearchHit{}
	err := v.walkNotes(folder, true, func(rel, full string, info fs.FileInfo) error {
		nameMatch := strings.Contains(fold(path.Base(rel)), q)
		excerpt := ""
		if info.Size() <= maxSearchFileSize {
			if data, err := os.ReadFile(full); err == nil {
				excerpt = findExcerpt(string(data), q)
			}
		}
		if excerpt == "" && !nameMatch {
			return nil
		}
		hits = append(hits, SearchHit{Path: rel, Modified: info.ModTime(), Excerpt: excerpt})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if !hits[i].Modified.Equal(hits[j].Modified) {
			return hits[i].Modified.After(hits[j].Modified)
		}
		return hits[i].Path < hits[j].Path
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// findExcerpt は最初に q（fold 済み）を含む行と前後1行を返す。無ければ ""。
// 複数行にまたがるクエリは本文には一致しない。
func findExcerpt(content, q string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = norm.NFC.String(strings.TrimSuffix(line, "\r"))
		lines[i] = line
	}
	for i, line := range lines {
		lower := strings.ToLower(line)
		idx := strings.Index(lower, q)
		if idx < 0 {
			continue
		}
		matchRune := utf8.RuneCountInString(lower[:idx])
		matchLen := utf8.RuneCountInString(q)
		start, end := i, i+1
		if start > 0 {
			start--
		}
		if end < len(lines) {
			end++
		}
		out := make([]string, 0, 3)
		for j := start; j < end; j++ {
			if j == i {
				out = append(out, clipAround(line, matchRune, matchLen))
			} else {
				out = append(out, clipHead(lines[j]))
			}
		}
		return strings.Join(out, "\n")
	}
	return ""
}

func clipHead(s string) string {
	r := []rune(s)
	if len(r) <= maxExcerptRunes {
		return s
	}
	return string(r[:maxExcerptRunes])
}

// clipAround は一致位置の周辺 maxExcerptRunes に切る。
func clipAround(s string, matchStart, matchLen int) string {
	r := []rune(s)
	if len(r) <= maxExcerptRunes {
		return s
	}
	start := matchStart - (maxExcerptRunes-matchLen)/2
	if start < 0 {
		start = 0
	}
	end := start + maxExcerptRunes
	if end > len(r) {
		end = len(r)
		start = end - maxExcerptRunes
	}
	return string(r[start:end])
}
