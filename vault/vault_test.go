package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestVault(t *testing.T) (string, *Vault) {
	t.Helper()
	dir := t.TempDir()
	v := New(dir)
	return dir, v
}

func TestValidatePath(t *testing.T) {
	_, v := setupTestVault(t)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid path", "Notes/test.md", false},
		{"valid nested", "DailyReports/2026/03/report.md", false},
		{"traversal attack", "../etc/passwd", true},
		{"traversal in middle", "Notes/../../../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
		{"no md extension", "Notes/test.txt", true},
		{"empty path", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	dir, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	err := v.Create("Notes/test.md", fm, "本文です")
	if err != nil {
		t.Fatalf("Create失敗: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "Notes", "test.md"))
	if err != nil {
		t.Fatalf("ファイル読み取り失敗: %v", err)
	}
	if !stringContains(string(content), "本文です") {
		t.Errorf("ファイル内容に本文が含まれない: %s", content)
	}
}

func TestCreateConflict(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "1回目")

	err := v.Create("Notes/test.md", fm, "2回目")
	if err == nil {
		t.Fatal("2回目のCreateがエラーにならなかった")
	}
	if !IsErrAlreadyExists(err) {
		t.Errorf("ErrAlreadyExistsでない: %v", err)
	}
}

func TestRead(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "本文です")

	gotFm, gotBody, err := v.Read("Notes/test.md")
	if err != nil {
		t.Fatalf("Read失敗: %v", err)
	}
	if gotFm["title"] != "テスト" {
		t.Errorf("title = %v, want テスト", gotFm["title"])
	}
	if gotBody != "本文です" {
		t.Errorf("body = %q, want 本文です", gotBody)
	}
}

func TestReadNotFound(t *testing.T) {
	_, v := setupTestVault(t)

	_, _, err := v.Read("Notes/nonexistent.md")
	if err == nil {
		t.Fatal("存在しないファイルのReadがエラーにならなかった")
	}
	if !IsErrNotFound(err) {
		t.Errorf("ErrNotFoundでない: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "元の内容")

	newFm := map[string]interface{}{"title": "更新済み"}
	err := v.Update("Notes/test.md", newFm, "新しい内容")
	if err != nil {
		t.Fatalf("Update失敗: %v", err)
	}

	gotFm, gotBody, err := v.Read("Notes/test.md")
	if err != nil {
		t.Fatalf("Read失敗: %v", err)
	}
	if gotFm["title"] != "更新済み" {
		t.Errorf("title = %v, want 更新済み", gotFm["title"])
	}
	if gotBody != "新しい内容" {
		t.Errorf("body = %q, want 新しい内容", gotBody)
	}
}

func TestUpdateNotFound(t *testing.T) {
	_, v := setupTestVault(t)

	err := v.Update("Notes/nonexistent.md", nil, "内容")
	if err == nil {
		t.Fatal("存在しないファイルのUpdateがエラーにならなかった")
	}
	if !IsErrNotFound(err) {
		t.Errorf("ErrNotFoundでない: %v", err)
	}
}

func TestDelete(t *testing.T) {
	_, v := setupTestVault(t)

	fm := map[string]interface{}{"title": "テスト"}
	_ = v.Create("Notes/test.md", fm, "本文")

	err := v.Delete("Notes/test.md")
	if err != nil {
		t.Fatalf("Delete失敗: %v", err)
	}

	_, _, err = v.Read("Notes/test.md")
	if !IsErrNotFound(err) {
		t.Errorf("削除後のReadがNotFoundでない: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	_, v := setupTestVault(t)

	err := v.Delete("Notes/nonexistent.md")
	if err == nil {
		t.Fatal("存在しないファイルのDeleteがエラーにならなかった")
	}
	if !IsErrNotFound(err) {
		t.Errorf("ErrNotFoundでない: %v", err)
	}
}
