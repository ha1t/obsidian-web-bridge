package mcpserver

import (
	"fmt"
	"strings"
	"time"

	"github.com/ha1t/obsidian-web-bridge/vault"
)

// DailyNotesDir は DailyNote を置くフォルダ名。
const DailyNotesDir = "DailyNotes"

var weekdayJa = [...]string{"日", "月", "火", "水", "木", "金", "土"}

// DailyNotePath は date の DailyNote の相対パスを返す。
func DailyNotePath(date time.Time) string {
	return DailyNotesDir + "/" + date.Format("2006-01-02") + ".md"
}

// RenderDailyNote は date の DailyNote の初期内容を返す。date の暦の値をそのまま使う。
func RenderDailyNote(date time.Time) string {
	day := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
	ymd := day.Format("2006-01-02")
	prev := day.AddDate(0, 0, -1).Format("2006-01-02")
	next := day.AddDate(0, 0, 1).Format("2006-01-02")
	wd := weekdayJa[day.Weekday()] + "曜日"
	slashYM := day.Format("2006/01")
	todo := fmt.Sprintf("%04d年%02d月にやることリスト", day.Year(), int(day.Month()))

	return "---\n" +
		"title: DailyNote " + ymd + "\n" +
		"tags:\n" +
		"  - " + slashYM + "\n" +
		"  - daily\n" +
		"date: " + ymd + "\n" +
		"day_of_the_week: " + wd + "\n" +
		"location: hokkaido\n" +
		"weather:\n" +
		"temp_min:\n" +
		"temp_max:\n" +
		"---\n" +
		"\n" +
		"[[" + prev + "]] ::: [[" + next + "]]\n" +
		"\n" +
		"# Todo\n" +
		"\n" +
		"![[" + todo + "]]\n" +
		"\n" +
		"# Diary\n" +
		"\n" +
		"\n" +
		"# Memo\n" +
		"\n" +
		"# Smartphone\n" +
		"\n" +
		"![[mobile-" + ymd + "#Memo]]\n"
}

// AppendMemo は "# Memo" セクションに content を追記する。
// 見出しが無ければ "# Smartphone" の直前、それも無ければ末尾に "# Memo" を作る。
func AppendMemo(text, content string) string {
	const memo = "# Memo"
	if !vault.HasHeading(text, memo) {
		if idx, ok := vault.HeadingLine(text, "# Smartphone"); ok {
			text = insertHeadingAt(text, idx, memo)
		} else {
			text = vault.AppendToEnd(text, memo)
		}
	}
	out, err := vault.AppendToSection(text, memo, content)
	if err != nil {
		// 見出しを保証できなかった場合も、見出しごと末尾に追記する
		return vault.AppendToEnd(text, memo+"\n\n"+content)
	}
	return out
}

// insertHeadingAt は idx 行目の直前に heading 行を挿入する。直前が空行でなければ空行を足す。
func insertHeadingAt(text string, idx int, heading string) string {
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(text, "\n")
	before := strings.Join(lines[:idx], "")
	if idx > 0 && strings.TrimSpace(lines[idx-1]) != "" {
		before += eol
	}
	return before + heading + eol + strings.Join(lines[idx:], "")
}
