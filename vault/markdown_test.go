package vault

import (
	"errors"
	"testing"
)

func TestAppendToSection_EmptyMemo(t *testing.T) {
	in := "# Diary\n\n日記\n\n# Memo\n\n# Smartphone\n\n![[mobile#Memo]]\n"
	got, err := AppendToSection(in, "# Memo", "メモ1")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Diary\n\n日記\n\n# Memo\n\nメモ1\n\n# Smartphone\n\n![[mobile#Memo]]\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendToSection_ExistingContent(t *testing.T) {
	got, err := AppendToSection("# Memo\n\n- a\n\n\n# Smartphone\n", "# Memo", "- b")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Memo\n\n- a\n\n- b\n\n# Smartphone\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendToSection_LastSection(t *testing.T) {
	got, err := AppendToSection("# A\n\nx\n# Memo\n\nold", "# Memo", "new")
	if err != nil {
		t.Fatal(err)
	}
	want := "# A\n\nx\n# Memo\n\nold\n\nnew\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendToSection_SubheadingStaysInside(t *testing.T) {
	in := "# Diary\n\ntext\n\n## 見出し\n\nsub\n\n# Memo\n\n"
	got, err := AppendToSection(in, "# Diary", "追記")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Diary\n\ntext\n\n## 見出し\n\nsub\n\n追記\n\n# Memo\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	got, err = AppendToSection(in, "## 見出し", "x")
	if err != nil {
		t.Fatal(err)
	}
	want = "# Diary\n\ntext\n\n## 見出し\n\nsub\n\nx\n\n# Memo\n\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendToSection_NotFound(t *testing.T) {
	_, err := AppendToSection("# A\n", "# Memo", "x")
	if !errors.Is(err, ErrHeadingNotFound) {
		t.Errorf("err = %v", err)
	}
	_, err = AppendToSection("## Memo\n", "# Memo", "x")
	if !errors.Is(err, ErrHeadingNotFound) {
		t.Errorf("level mismatch err = %v", err)
	}
}

func TestAppendToSection_IgnoresFencedHeading(t *testing.T) {
	_, err := AppendToSection("```\n# Memo\n```\n", "# Memo", "x")
	if !errors.Is(err, ErrHeadingNotFound) {
		t.Errorf("err = %v", err)
	}
	in := "```\n# Memo\n```\n\n# Memo\n\nold\n"
	got, err := AppendToSection(in, "# Memo", "new")
	if err != nil {
		t.Fatal(err)
	}
	want := "```\n# Memo\n```\n\n# Memo\n\nold\n\nnew\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	// フェンス内の見出しはセクション終端にもならない
	in = "# Memo\n\n~~~\n# X\n~~~\n"
	got, err = AppendToSection(in, "# Memo", "new")
	if err != nil {
		t.Fatal(err)
	}
	want = "# Memo\n\n~~~\n# X\n~~~\n\nnew\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendToSection_CRLF(t *testing.T) {
	got, err := AppendToSection("# Memo\r\n\r\n# Smartphone\r\n", "# Memo", "a\nb")
	if err != nil {
		t.Fatal(err)
	}
	want := "# Memo\r\n\r\na\r\nb\r\n\r\n# Smartphone\r\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestAppendToEnd(t *testing.T) {
	cases := [][3]string{
		{"x", "y", "x\n\ny\n"},
		{"x\n\n\n", "y", "x\n\ny\n"},
		{"", "y", "y\n"},
		{"x\r\n", "y\nz", "x\r\n\r\ny\r\nz\r\n"},
	}
	for _, c := range cases {
		if got := AppendToEnd(c[0], c[1]); got != c[2] {
			t.Errorf("AppendToEnd(%q,%q) = %q want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestHasHeading(t *testing.T) {
	if !HasHeading("# A\n\n# Memo \n", "# Memo") {
		t.Error("should match")
	}
	if HasHeading("## Memo\n", "# Memo") {
		t.Error("level differs")
	}
	if HasHeading("```\n# Memo\n```\n", "# Memo") {
		t.Error("fenced")
	}
}

func TestHeadingLine(t *testing.T) {
	text := "a\n```\n# X\n```\n#  X  \n"
	if i, ok := HeadingLine(text, "# X"); !ok || i != 4 {
		t.Errorf("got %d %v", i, ok)
	}
	if _, ok := HeadingLine(text, "# Y"); ok {
		t.Error("unexpected")
	}
}
