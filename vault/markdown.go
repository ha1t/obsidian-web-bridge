package vault

import (
	"errors"
	"strings"
)

// ErrHeadingNotFound は指定した見出しがテキスト中に無いときに返る。
var ErrHeadingNotFound = errors.New("heading not found")

// parseHeading は ATX 見出し行を (レベル, テキスト) に分解する。見出しでなければ ok=false。
func parseHeading(line string) (level int, text string, ok bool) {
	line = strings.TrimRight(line, "\r")
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return 0, "", false
	}
	rest := line[n:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	return n, strings.TrimSpace(rest), true
}

func isFence(line string) bool {
	l := strings.TrimLeft(line, " ")
	return strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~")
}

// headingLevels は各行について、フェンス外の見出しなら level>0 を返す。
func headingLevels(lines []string) ([]int, []string) {
	levels := make([]int, len(lines))
	texts := make([]string, len(lines))
	inFence := false
	for i, l := range lines {
		if isFence(l) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if lv, tx, ok := parseHeading(l); ok {
			levels[i] = lv
			texts[i] = tx
		}
	}
	return levels, texts
}

func detectEOL(text string) string {
	if strings.Contains(text, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// contentLines は content の末尾改行を落として行に分け、各行に \r を付け直す(CRLF時)。
func contentLines(content, eol string) []string {
	content = strings.TrimRight(content, "\r\n")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	parts := strings.Split(content, "\n")
	if eol == "\r\n" {
		for i := range parts {
			parts[i] += "\r"
		}
	}
	return parts
}

// AppendToSection は heading ("# Memo" など) のセクション末尾に content を追記する。
func AppendToSection(text, heading, content string) (string, error) {
	hl, ht, ok := parseHeading(heading)
	if !ok {
		return "", ErrHeadingNotFound
	}
	eol := detectEOL(text)
	lines := strings.Split(text, "\n")
	levels, texts := headingLevels(lines)

	start := -1
	for i := range lines {
		if levels[i] == hl && texts[i] == ht {
			start = i
			break
		}
	}
	if start < 0 {
		return "", ErrHeadingNotFound
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if levels[i] > 0 && levels[i] <= hl {
			end = i
			break
		}
	}

	body := lines[start+1 : end]
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	blank := ""
	if eol == "\r\n" {
		blank = "\r"
	}

	out := make([]string, 0, len(lines)+len(content)+3)
	out = append(out, lines[:start+1]...)
	out = append(out, body...)
	out = append(out, blank)
	out = append(out, contentLines(content, eol)...)
	if end < len(lines) {
		out = append(out, blank)
		out = append(out, lines[end:]...)
	} else {
		out = append(out, "")
	}
	return strings.Join(out, "\n"), nil
}

// AppendToEnd は text の末尾(空行を除いた後)に空行を1つ挟んで content を追記する。
func AppendToEnd(text, content string) string {
	eol := detectEOL(text)
	body := strings.TrimRight(text, "\r\n")
	added := strings.Join(contentLines(content, eol), "\n") + "\n"
	if body == "" {
		return added
	}
	return body + eol + eol + added
}

// HasHeading は heading がフェンス外に存在するかを返す。
func HasHeading(text, heading string) bool {
	hl, ht, ok := parseHeading(heading)
	if !ok {
		return false
	}
	lines := strings.Split(text, "\n")
	levels, texts := headingLevels(lines)
	for i := range lines {
		if levels[i] == hl && texts[i] == ht {
			return true
		}
	}
	return false
}
