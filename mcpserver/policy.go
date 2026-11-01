// Package mcpserver はリモートMCPエンドポイントの実装を提供する。
package mcpserver

import "strings"

// FolderClass はパスが属するフォルダの書き込み権限の種別。
type FolderClass int

const (
	// FolderWritable は通常の書き込み可能フォルダ。
	FolderWritable FolderClass = iota
	// FolderReadonly は読み取り専用フォルダ。
	FolderReadonly
	// FolderDailyNotes は append_daily_memo 経由でのみ書けるフォルダ。
	FolderDailyNotes
)

// Policy はフォルダごとの書き込み権限を判定する。
type Policy struct{ readonly []string }

// NewPolicy は読み取り専用フォルダ名の一覧から Policy を作る。
func NewPolicy(readonlyFolders []string) *Policy {
	return &Policy{readonly: append([]string(nil), readonlyFolders...)}
}

// Classify は正規化済みの相対パスの先頭要素を、大文字小文字を区別せずに判定する。
func (p *Policy) Classify(cleanPath string) FolderClass {
	first, _, found := strings.Cut(cleanPath, "/")
	if !found {
		// ルート直下のファイル名はフォルダではない
		return FolderWritable
	}
	if strings.EqualFold(first, DailyNotesDir) {
		return FolderDailyNotes
	}
	for _, r := range p.readonly {
		if strings.EqualFold(first, r) {
			return FolderReadonly
		}
	}
	return FolderWritable
}
