package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	errAlreadyExists = errors.New("file already exists")
	errNotFound      = errors.New("file not found")
)

func IsErrAlreadyExists(err error) bool {
	return errors.Is(err, errAlreadyExists)
}

func IsErrNotFound(err error) bool {
	return errors.Is(err, errNotFound)
}

type Vault struct {
	rootPath string

	locksMu sync.Mutex
	locks   map[string]*sync.Mutex
}

func New(rootPath string) *Vault {
	return &Vault{rootPath: rootPath, locks: map[string]*sync.Mutex{}}
}

func (v *Vault) ValidatePath(notePath string) error {
	if notePath == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(notePath) {
		return fmt.Errorf("absolute path not allowed: %s", notePath)
	}
	if strings.Contains(notePath, "..") {
		return fmt.Errorf("path traversal not allowed: %s", notePath)
	}
	if filepath.Ext(notePath) != ".md" {
		return fmt.Errorf("only .md files allowed: %s", notePath)
	}
	return nil
}

func (v *Vault) fullPath(notePath string) string {
	return filepath.Join(v.rootPath, notePath)
}

func (v *Vault) Create(notePath string, frontmatter map[string]interface{}, body string) error {
	if err := v.ValidatePath(notePath); err != nil {
		return err
	}

	fp := v.fullPath(notePath)
	if _, err := os.Stat(fp); err == nil {
		return fmt.Errorf("%w: %s", errAlreadyExists, notePath)
	}

	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	content := MarshalNote(frontmatter, body)
	return os.WriteFile(fp, []byte(content), 0644)
}

func (v *Vault) Read(notePath string) (map[string]interface{}, string, error) {
	if err := v.ValidatePath(notePath); err != nil {
		return nil, "", err
	}

	fp := v.fullPath(notePath)
	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("%w: %s", errNotFound, notePath)
		}
		return nil, "", err
	}

	return UnmarshalNote(string(data))
}

func (v *Vault) Update(notePath string, frontmatter map[string]interface{}, body string) error {
	if err := v.ValidatePath(notePath); err != nil {
		return err
	}

	fp := v.fullPath(notePath)
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", errNotFound, notePath)
	}

	content := MarshalNote(frontmatter, body)
	return os.WriteFile(fp, []byte(content), 0644)
}

func (v *Vault) Delete(notePath string) error {
	if err := v.ValidatePath(notePath); err != nil {
		return err
	}

	fp := v.fullPath(notePath)
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", errNotFound, notePath)
	}

	return os.Remove(fp)
}
