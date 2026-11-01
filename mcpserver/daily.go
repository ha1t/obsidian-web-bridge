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
		if vault.HasHeading(text, "# Smartphone") {
			text = insertBeforeHeading(text, "# Smartphone", memo+"\n")
		} else {
			text = vault.AppendToEnd(text, memo)
		}
	}
	out, err := vault.AppendToSection(text, memo, content)
	if err != nil {
		// 直前で見出しを保証しているため通常は到達しない
		return vault.AppendToEnd(text, content)
	}
	return out
}

// insertBeforeHeading はフェンス外の最初の heading 行の直前に line を挿入する。
func insertBeforeHeading(text, heading, line string) string {
	lines := strings.SplitAfter(text, "\n")
	inFence := false
	for i, l := range lines {
		t := strings.TrimLeft(l, " ")
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.TrimRight(l, "\r\n") == heading {
			return strings.Join(lines[:i], "") + line + strings.Join(lines[i:], "")
		}
	}
	return text
}
