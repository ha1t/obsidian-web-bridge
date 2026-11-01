package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrHashMismatch は ReplaceRaw の expectedHash が現在の内容と一致しないときに返る。
var ErrHashMismatch = errors.New("hash mismatch")

// RawNote は生テキストのまま読み出したノート。
type RawNote struct {
	Path     string    // 正規化済みの相対パス（スラッシュ区切り）
	Content  string    // ファイル内容そのまま
	Hash     string    // Content の SHA-256（小文字16進）
	Modified time.Time // mtime
}

// HashContent は内容の SHA-256 を小文字16進で返す。
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// CleanPath はノートパスを正規化・検証し、スラッシュ区切りの相対パスを返す。
func (v *Vault) CleanPath(notePath string) (string, error) {
	if notePath == "" {
		return "", fmt.Errorf("path is empty")
	}
	p := strings.ReplaceAll(notePath, "\\", "/")
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("invalid path")
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("absolute path not allowed: %s", notePath)
	}
	if len(p) >= 2 && p[1] == ':' { // Windows ドライブレター
		return "", fmt.Errorf("absolute path not allowed: %s", notePath)
	}
	for _, elem := range strings.Split(p, "/") {
		if elem == ".." {
			return "", fmt.Errorf("path traversal not allowed: %s", notePath)
		}
	}
	p = path.Clean(p)
	if p == "." || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("invalid path: %s", notePath)
	}
	for _, elem := range strings.Split(p, "/") {
		if strings.HasPrefix(elem, ".") {
			return "", fmt.Errorf("hidden path element not allowed: %s", notePath)
		}
	}
	if path.Ext(p) != ".md" {
		return "", fmt.Errorf("only .md files allowed: %s", notePath)
	}
	return p, nil
}

// lockFor は正規化パスごとのロックを返す。
func (v *Vault) lockFor(clean string) *sync.Mutex {
	v.locksMu.Lock()
	defer v.locksMu.Unlock()
	if v.locks == nil {
		v.locks = map[string]*sync.Mutex{}
	}
	m, ok := v.locks[clean]
	if !ok {
		m = &sync.Mutex{}
		v.locks[clean] = m
	}
	return m
}

// within は child が root 配下（root 自身を含む）かを返す。
func within(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// resolve は clean の実パスを検査し、フルパスを返す。
// 存在する一番深い祖先（ファイル自体を含む）を EvalSymlinks で解決し、Vault ルート配下であることを確認する。
func (v *Vault) resolve(clean string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(v.rootPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve vault root: %w", err)
	}
	fp := filepath.Join(v.rootPath, filepath.FromSlash(clean))
	cur := fp
	for {
		if _, err := os.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", fmt.Errorf("failed to resolve path: %w", err)
			}
			if !within(realRoot, real) {
				return "", fmt.Errorf("path escapes vault: %s", clean)
			}
			return fp, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("path escapes vault: %s", clean)
		}
		cur = parent
	}
}

// writeAtomic は同じディレクトリの一時ファイルに書いてから rename する。
// 既存ファイルがあればそのパーミッションを引き継ぎ、無ければ 0644。
func writeAtomic(fp, content string) error {
	dir := filepath.Dir(fp)
	mode := os.FileMode(0644)
	if fi, err := os.Stat(fp); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".owb-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { os.Remove(tmpName) }
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, fp); err != nil {
		cleanup()
		return err
	}
	return nil
}

// readFile は実パス検査のうえファイルを読む。見つからないときも fp を返す。
func (v *Vault) readFile(clean string) (string, string, os.FileInfo, error) {
	fp, err := v.resolve(clean)
	if err != nil {
		return "", "", nil, err
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return fp, "", nil, fmt.Errorf("%w: %s", errNotFound, clean)
		}
		return "", "", nil, err
	}
	fi, err := os.Stat(fp)
	if err != nil {
		return "", "", nil, err
	}
	return fp, string(data), fi, nil
}

func (v *Vault) ReadRaw(notePath string) (*RawNote, error) {
	clean, err := v.CleanPath(notePath)
	if err != nil {
		return nil, err
	}
	_, content, fi, err := v.readFile(clean)
	if err != nil {
		return nil, err
	}
	return &RawNote{Path: clean, Content: content, Hash: HashContent(content), Modified: fi.ModTime()}, nil
}

// CreateRaw は新規ファイルを作る。既存なら errAlreadyExists。
func (v *Vault) CreateRaw(notePath, content string) error {
	clean, err := v.CleanPath(notePath)
	if err != nil {
		return err
	}
	l := v.lockFor(clean)
	l.Lock()
	defer l.Unlock()

	fp, err := v.resolve(clean)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(fp); err == nil {
		return fmt.Errorf("%w: %s", errAlreadyExists, clean)
	}
	return v.writeNew(fp, content)
}

func (v *Vault) writeNew(fp, content string) error {
	if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return writeAtomic(fp, content)
}

// AppendRaw は heading が空ならファイル末尾、そうでなければその見出しセクションの末尾に追記する。
func (v *Vault) AppendRaw(notePath, content, heading string) error {
	clean, err := v.CleanPath(notePath)
	if err != nil {
		return err
	}
	l := v.lockFor(clean)
	l.Lock()
	defer l.Unlock()

	fp, old, _, err := v.readFile(clean)
	if err != nil {
		return err
	}
	var updated string
	if heading == "" {
		updated = AppendToEnd(old, content)
	} else {
		updated, err = AppendToSection(old, heading, content)
		if err != nil {
			return err
		}
	}
	return writeAtomic(fp, updated)
}

// ReplaceRaw は現在の内容の hash が expectedHash と一致するときだけ全文を置き換える。
func (v *Vault) ReplaceRaw(notePath, content, expectedHash string) error {
	clean, err := v.CleanPath(notePath)
	if err != nil {
		return err
	}
	l := v.lockFor(clean)
	l.Lock()
	defer l.Unlock()

	fp, old, _, err := v.readFile(clean)
	if err != nil {
		return err
	}
	if HashContent(old) != expectedHash {
		return fmt.Errorf("%w: %s", ErrHashMismatch, clean)
	}
	return writeAtomic(fp, content)
}

// UpsertRaw はファイルが無ければ initial を元に、あれば既存内容に update を適用して書き込む。
// 全体を同じロックの中で行う。新規作成で update が失敗した場合、ファイルは作られない。
func (v *Vault) UpsertRaw(notePath string, initial string, update func(old string) (string, error)) (bool, error) {
	clean, err := v.CleanPath(notePath)
	if err != nil {
		return false, err
	}
	l := v.lockFor(clean)
	l.Lock()
	defer l.Unlock()

	fp, old, _, err := v.readFile(clean)
	created := false
	if err != nil {
		if !IsErrNotFound(err) {
			return false, err
		}
		created = true
		old = initial
	}
	updated, err := update(old)
	if err != nil {
		return false, err
	}
	if created {
		return true, v.writeNew(fp, updated)
	}
	return false, writeAtomic(fp, updated)
}
