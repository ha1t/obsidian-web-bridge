package mcpserver

import (
	"strings"
	"testing"
	"time"
)

var jst = time.FixedZone("JST", 9*3600)

const goldenDaily = `---
title: DailyNote 2026-11-01
tags:
  - 2026/11
  - daily
date: 2026-11-01
day_of_the_week: 日曜日
location: hokkaido
weather:
temp_min:
temp_max:
---

[[2026-10-31]] ::: [[2026-11-02]]

# Todo

![[2026年11月にやることリスト]]

# Diary


# Memo

# Smartphone

![[mobile-2026-11-01#Memo]]
`

func TestRenderDailyNote_Golden(t *testing.T) {
	d := time.Date(2026, 11, 1, 10, 0, 0, 0, jst)
	if got := RenderDailyNote(d); got != goldenDaily {
		t.Errorf("mismatch:\n%s", got)
	}
	if DailyNotePath(d) != "DailyNotes/2026-11-01.md" {
		t.Error("path")
	}
}

func TestRenderDailyNote_YearBoundary(t *testing.T) {
	got := RenderDailyNote(time.Date(2026, 12, 31, 23, 0, 0, 0, jst))
	for _, w := range []string{"[[2026-12-30]] ::: [[2027-01-01]]", "2026/12", "2026年12月にやることリスト", "木曜日"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q", w)
		}
	}
	got = RenderDailyNote(time.Date(2027, 1, 1, 0, 0, 0, 0, jst))
	for _, w := range []string{"[[2026-12-31]] ::: [[2027-01-02]]", "2027/01", "2027年01月にやることリスト", "金曜日"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func TestAppendMemo_ToTemplate(t *testing.T) {
	got := AppendMemo(goldenDaily, "メモ")
	if !strings.Contains(got, "# Memo\n\nメモ\n\n# Smartphone") {
		t.Errorf("got:\n%s", got)
	}
}

func TestAppendMemo_SmartphoneTrailingSpace(t *testing.T) {
	got := AppendMemo("# Diary\n日記\n\n# Smartphone \n\nx\n", "メモ")
	want := "# Diary\n日記\n\n# Memo\n\nメモ\n\n# Smartphone \n\nx\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendMemo_NonBlankBeforeSmartphone(t *testing.T) {
	got := AppendMemo("# Diary\n日記\n# Smartphone\nx\n", "メモ")
	want := "# Diary\n日記\n\n# Memo\n\nメモ\n\n# Smartphone\nx\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	got = AppendMemo("# Diary\r\n日記\r\n# Smartphone\r\nx\r\n", "メモ")
	want = "# Diary\r\n日記\r\n\r\n# Memo\r\n\r\nメモ\r\n\r\n# Smartphone\r\nx\r\n"
	if got != want {
		t.Errorf("crlf got %q want %q", got, want)
	}
}

func TestAppendMemo_ExistingMemoContent(t *testing.T) {
	in := "# Diary\n日記\n\n# Memo\n\n既存\n\n# Smartphone\n\nx\n"
	got := AppendMemo(in, "新規")
	want := "# Diary\n日記\n\n# Memo\n\n既存\n\n新規\n\n# Smartphone\n\nx\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendMemo_SmartphoneInFenceIgnored(t *testing.T) {
	in := "# Diary\n```\n# Smartphone\n```\n"
	got := AppendMemo(in, "メモ")
	want := "# Diary\n```\n# Smartphone\n```\n\n# Memo\n\nメモ\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendMemo_NoMemoHeading(t *testing.T) {
	got := AppendMemo("# Diary\n日記\n\n# Smartphone\n\nx\n", "メモ")
	want := "# Diary\n日記\n\n# Memo\n\nメモ\n\n# Smartphone\n\nx\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	got = AppendMemo("# Diary\n日記\n", "メモ")
	want = "# Diary\n日記\n\n# Memo\n\nメモ\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
