package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ha1t/obsidian-web-bridge/vault"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var testDefaultReadonly = []string{"DailyReports", "MorningBrief", "Calendar", "Garmin", "MonthlyReview", "Templates", "bases"}

func testOptions(t *testing.T, dir string, now time.Time) Options {
	return Options{
		Vault:    vault.New(dir),
		Policy:   NewPolicy(testDefaultReadonly),
		Location: jst,
		Now:      func() time.Time { return now },
	}
}

// newTestSession は一時 Vault に対する MCP サーバーとクライアントをメモリ上でつなぐ。
func newTestSession(t *testing.T, now time.Time) (*mcp.ClientSession, string) {
	t.Helper()
	dir := t.TempDir()
	srv := NewMCPServer(testOptions(t, dir, now))
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, dir
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func mustOK(t *testing.T, name string, res *mcp.CallToolResult) {
	t.Helper()
	if res.IsError {
		t.Fatalf("%s: unexpected error: %s", name, resultText(res))
	}
}

func mustErr(t *testing.T, name string, res *mcp.CallToolResult, want string) {
	t.Helper()
	if !res.IsError {
		t.Fatalf("%s: want error containing %q, got success: %s", name, want, resultText(res))
	}
	if !strings.Contains(resultText(res), want) {
		t.Fatalf("%s: error %q does not contain %q", name, resultText(res), want)
	}
	if res.StructuredContent != nil {
		t.Fatalf("%s: error result should not carry structured content: %v", name, res.StructuredContent)
	}
}

func structured(t *testing.T, res *mcp.CallToolResult, v any) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("structured content %s: %v", b, err)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	fp := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var testNow = time.Date(2026, 11, 1, 14, 30, 0, 0, time.UTC) // JST 23:30

func TestListTools(t *testing.T) {
	cs, _ := newTestSession(t, testNow)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		tools[tl.Name] = tl
	}
	for _, name := range []string{"list_notes", "search_notes", "read_note", "create_note", "append_note", "update_note", "append_daily_memo"} {
		if tools[name] == nil {
			t.Errorf("tool %s missing", name)
		}
	}
	if len(tools) != 7 {
		t.Errorf("tool count = %d, want 7", len(tools))
	}
	for _, name := range []string{"list_notes", "search_notes", "read_note"} {
		if a := tools[name].Annotations; a == nil || !a.ReadOnlyHint {
			t.Errorf("%s should be readOnly", name)
		}
	}
	if a := tools["update_note"].Annotations; a == nil || a.DestructiveHint == nil || !*a.DestructiveHint {
		t.Errorf("update_note should be destructive")
	}
	for _, name := range []string{"create_note", "append_note", "append_daily_memo"} {
		a := tools[name].Annotations
		if a == nil || a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s should be explicitly non-destructive write tool: %+v", name, a)
		}
	}
}

func TestCreateAndRead(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	res := call(t, cs, "create_note", map[string]any{
		"path":        "Notes/軽トラ車検.md",
		"frontmatter": map[string]any{"title": "車検", "created": "2020-01-01"},
		"body":        "本文です\n",
	})
	mustOK(t, "create_note", res)
	var cout struct{ Path string }
	structured(t, res, &cout)
	if cout.Path != "Notes/軽トラ車検.md" {
		t.Errorf("path = %q", cout.Path)
	}

	content := readFile(t, dir, "Notes/軽トラ車検.md")
	if !strings.Contains(content, "source/claude-desktop") {
		t.Errorf("missing source tag:\n%s", content)
	}
	if strings.Contains(content, "created") {
		t.Errorf("created must not be written:\n%s", content)
	}

	res = call(t, cs, "read_note", map[string]any{"path": "Notes/軽トラ車検.md"})
	mustOK(t, "read_note", res)
	var rout struct {
		Path        string
		Content     string
		Frontmatter map[string]any
		Hash        string
		Modified    string
	}
	structured(t, res, &rout)
	if rout.Hash != vault.HashContent(content) {
		t.Errorf("hash mismatch")
	}
	if rout.Content != content {
		t.Errorf("content mismatch")
	}
	if rout.Frontmatter["title"] != "車検" {
		t.Errorf("frontmatter = %v", rout.Frontmatter)
	}
	if !strings.HasSuffix(rout.Modified, "+09:00") {
		t.Errorf("modified = %q", rout.Modified)
	}
}

func TestReadNote_NotFoundAndBadFrontmatter(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	mustErr(t, "read_note", call(t, cs, "read_note", map[string]any{"path": "Notes/none.md"}), "存在しません")
	mustErr(t, "read_note", call(t, cs, "read_note", map[string]any{"path": "../x.md"}), "パス")

	writeFile(t, dir, "Notes/bad.md", "---\n: [\n---\nbody\n")
	res := call(t, cs, "read_note", map[string]any{"path": "Notes/bad.md"})
	mustOK(t, "read_note", res)
	var rout struct {
		Content     string
		Frontmatter map[string]any
	}
	structured(t, res, &rout)
	if rout.Frontmatter != nil || rout.Content == "" {
		t.Errorf("want nil frontmatter and content, got %+v", rout)
	}
}

func TestCreateNote_Exists(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "Notes/a.md", "x\n")
	res := call(t, cs, "create_note", map[string]any{"path": "Notes/a.md", "body": "y"})
	mustErr(t, "create_note", res, "append_note")
	mustErr(t, "create_note", res, "Notes/a.md")
}

func TestWriteTools_ReadonlyFolder(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "DailyReports/x.md", "orig\n")
	p := "DailyReports/x.md"
	mustErr(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "DailyReports/y.md", "body": "b"}), "読み取り専用")
	mustErr(t, "append_note", call(t, cs, "append_note", map[string]any{"path": p, "content": "c"}), "読み取り専用")
	mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": p, "content": "c", "hash": vault.HashContent("orig\n")}), "読み取り専用")
	if got := readFile(t, dir, p); got != "orig\n" {
		t.Errorf("file changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "DailyReports", "y.md")); err == nil {
		t.Errorf("y.md must not be created")
	}
	// 大文字小文字が違っても拒否
	mustErr(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "dailyreports/z.md", "body": "b"}), "読み取り専用")
}

func TestWriteTools_DailyNotes(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	p := "DailyNotes/2026-11-01.md"
	writeFile(t, dir, p, "orig\n")
	mustErr(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "DailyNotes/2026-11-03.md", "body": "b"}), "append_daily_memo")
	mustErr(t, "append_note", call(t, cs, "append_note", map[string]any{"path": p, "content": "c"}), "append_daily_memo")
	mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": p, "content": "c", "hash": vault.HashContent("orig\n")}), "append_daily_memo")
	if got := readFile(t, dir, p); got != "orig\n" {
		t.Errorf("file changed: %q", got)
	}
}

func TestAppendNote(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "Notes/a.md", "# A\n\none\n\n# B\n\ntwo\n")
	mustOK(t, "append_note", call(t, cs, "append_note", map[string]any{"path": "Notes/a.md", "content": "added", "heading": "# A"}))
	got := readFile(t, dir, "Notes/a.md")
	if !strings.Contains(got, "one\n\nadded\n") || strings.Index(got, "added") > strings.Index(got, "# B") {
		t.Errorf("append to heading failed:\n%s", got)
	}
	mustErr(t, "append_note", call(t, cs, "append_note", map[string]any{"path": "Notes/a.md", "content": "x", "heading": "# Z"}), "見出し")
	mustErr(t, "append_note", call(t, cs, "append_note", map[string]any{"path": "Notes/none.md", "content": "x"}), "存在しません")
	mustErr(t, "append_note", call(t, cs, "append_note", map[string]any{"path": "Notes/a.md", "content": "  \n"}), "content")
}

func TestUpdateNote(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "Notes/a.md", "old\n")
	res := call(t, cs, "update_note", map[string]any{"path": "Notes/a.md", "content": "new\n", "hash": vault.HashContent("old\n")})
	mustOK(t, "update_note", res)
	var out struct{ Path, Hash string }
	structured(t, res, &out)
	if out.Hash != vault.HashContent("new\n") {
		t.Errorf("hash = %q", out.Hash)
	}
	if got := readFile(t, dir, "Notes/a.md"); got != "new\n" {
		t.Errorf("content = %q", got)
	}
}

func TestUpdateNote_HashMismatch(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "Notes/a.md", "old\n")
	res := call(t, cs, "update_note", map[string]any{"path": "Notes/a.md", "content": "new\n", "hash": "deadbeef"})
	mustErr(t, "update_note", res, "read_note で読み直して")
	if got := readFile(t, dir, "Notes/a.md"); got != "old\n" {
		t.Errorf("file changed: %q", got)
	}
}

type dailyOut struct {
	Path    string
	Created bool
}

func TestAppendDailyMemo_CreatesToday(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	res := call(t, cs, "append_daily_memo", map[string]any{"content": "メモです"})
	mustOK(t, "append_daily_memo", res)
	var out dailyOut
	structured(t, res, &out)
	if out.Path != "DailyNotes/2026-11-01.md" || !out.Created {
		t.Errorf("out = %+v", out)
	}
	if !strings.Contains(resultText(res), "最長1時間") {
		t.Errorf("text = %q", resultText(res))
	}
	got := readFile(t, dir, "DailyNotes/2026-11-01.md")
	memo := strings.Index(got, "# Memo")
	m := strings.Index(got, "メモです")
	sp := strings.Index(got, "# Smartphone")
	if memo < 0 || m < memo || m > sp {
		t.Errorf("memo not under # Memo:\n%s", got)
	}
}

func TestAppendDailyMemo_TimezoneBoundary(t *testing.T) {
	cs, dir := newTestSession(t, time.Date(2026, 11, 1, 15, 30, 0, 0, time.UTC))
	mustOK(t, "append_daily_memo", call(t, cs, "append_daily_memo", map[string]any{"content": "x"}))
	if _, err := os.Stat(filepath.Join(dir, "DailyNotes", "2026-11-02.md")); err != nil {
		t.Errorf("2026-11-02.md not created: %v", err)
	}
}

func TestAppendDailyMemo_Existing(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	p := "DailyNotes/2026-10-30.md"
	writeFile(t, dir, p, "# Diary\n\n日記の内容\n\n# Memo\n\n# Smartphone\n")
	res := call(t, cs, "append_daily_memo", map[string]any{"content": "追記", "date": "2026-10-30"})
	mustOK(t, "append_daily_memo", res)
	var out dailyOut
	structured(t, res, &out)
	if out.Created || out.Path != p {
		t.Errorf("out = %+v", out)
	}
	if strings.Contains(resultText(res), "最長1時間") {
		t.Errorf("should not mention sync for existing note")
	}
	got := readFile(t, dir, p)
	if !strings.Contains(got, "日記の内容") || !strings.Contains(got, "追記") {
		t.Errorf("content:\n%s", got)
	}
}

func TestAppendDailyMemo_BadDate(t *testing.T) {
	cs, _ := newTestSession(t, testNow)
	mustErr(t, "append_daily_memo", call(t, cs, "append_daily_memo", map[string]any{"content": "x", "date": "2026/11/01"}), "YYYY-MM-DD")
	mustErr(t, "append_daily_memo", call(t, cs, "append_daily_memo", map[string]any{"content": " "}), "content")
}

func TestListAndSearch(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	for i := 0; i < 201; i++ {
		writeFile(t, dir, fmt.Sprintf("Notes/n%03d.md", i), "hello world\n")
	}
	res := call(t, cs, "list_notes", map[string]any{"folder": "Notes", "limit": 1000})
	mustOK(t, "list_notes", res)
	var lout struct {
		Notes []struct {
			Path     string
			Modified string
			Size     int64
		}
	}
	structured(t, res, &lout)
	if len(lout.Notes) != 200 {
		t.Fatalf("list len = %d, want 200", len(lout.Notes))
	}
	n := lout.Notes[0]
	if !strings.HasPrefix(n.Path, "Notes/") || n.Size == 0 {
		t.Errorf("note = %+v", n)
	}
	if _, err := time.Parse(time.RFC3339, n.Modified); err != nil || !strings.HasSuffix(n.Modified, "+09:00") {
		t.Errorf("modified = %q", n.Modified)
	}

	res = call(t, cs, "list_notes", map[string]any{"folder": "Notes"})
	structured(t, res, &lout)
	if len(lout.Notes) != 30 {
		t.Errorf("default list len = %d, want 30", len(lout.Notes))
	}

	res = call(t, cs, "search_notes", map[string]any{"query": "HELLO", "limit": 1000})
	mustOK(t, "search_notes", res)
	var sout struct {
		Hits []struct{ Path, Modified, Excerpt string }
	}
	structured(t, res, &sout)
	if len(sout.Hits) != 100 {
		t.Fatalf("search len = %d, want 100", len(sout.Hits))
	}
	h := sout.Hits[0]
	if !strings.HasPrefix(h.Path, "Notes/") || !strings.HasSuffix(h.Modified, "+09:00") || !strings.Contains(h.Excerpt, "hello") {
		t.Errorf("hit = %+v", h)
	}

	res = call(t, cs, "search_notes", map[string]any{"query": "hello"})
	structured(t, res, &sout)
	if len(sout.Hits) != 20 {
		t.Errorf("default search len = %d, want 20", len(sout.Hits))
	}

	mustErr(t, "search_notes", call(t, cs, "search_notes", map[string]any{"query": "  "}), "query")
}

func TestInstructionsListReadonlyFolders(t *testing.T) {
	cs, _ := newTestSession(t, testNow)
	ins := cs.InitializeResult().Instructions
	for _, want := range []string{"DailyReports", "Garmin", "append_daily_memo", "search_notes", "hash"} {
		if !strings.Contains(ins, want) {
			t.Errorf("instructions missing %q:\n%s", want, ins)
		}
	}
}

func TestHTTPHandler_RequiresJWT(t *testing.T) {
	j := newTestJWKS(t)
	h := NewHTTPHandler(testOptions(t, t.TempDir(), testNow), newTestVerifier(j))
	ts := httptest.NewServer(h)
	defer ts.Close()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	post := func(token string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(initBody))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "obsidian.example.com"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Cf-Access-Jwt-Assertion", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}

	if resp := post(""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("no token: status = %d, want 403", resp.StatusCode)
	}
	if resp := post(j.Sign(t, validClaims())); resp.StatusCode != http.StatusOK {
		t.Errorf("valid token: status = %d, want 200", resp.StatusCode)
	}
}

func TestCreateNote_UnsafeFilename(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	for _, p := range []string{
		"Notes/a:b.md",
		"Notes/a*b.md",
		"Notes/a?b.md",
		`Notes/a"b.md`,
		"Notes/a<b.md",
		"Notes/a>b.md",
		"Notes/a|b.md",
		"Notes/a\tb.md",
		"Notes/sub./a.md",
		"Notes/sub /a.md",
		"Notes/CON.md",
		"Notes/con.md",
		"Notes/lpt1.md",
		"Notes/Com9.md",
		"Notes/nul.backup.md",
		"aux/a.md",
	} {
		res := call(t, cs, "create_note", map[string]any{"path": p, "body": "b"})
		mustErr(t, "create_note "+p, res, "ファイル名に使えない文字が含まれています")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("nothing should be created: %v", entries)
	}
	// 予約名に似ているだけの名前は通る
	for _, p := range []string{"Notes/CONSOLE.md", "Notes/COM10.md", "Notes/a.b.md"} {
		mustOK(t, "create_note "+p, call(t, cs, "create_note", map[string]any{"path": p, "body": "b"}))
	}
}

func TestCreateNote_CaseOnlyFolderClash(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "Notes/x.md", "x\n")
	writeFile(t, dir, "Notes/Sub/y.md", "y\n")

	mustErr(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "notes/a.md", "body": "b"}),
		"フォルダ `Notes` が既にあります。大文字小文字を合わせてください")
	mustErr(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "Notes/sub/deeper/a.md", "body": "b"}),
		"フォルダ `Notes/Sub` が既にあります")
	if _, err := os.Stat(filepath.Join(dir, "notes")); err == nil {
		t.Errorf("notes/ must not be created")
	}
	if _, err := os.Stat(filepath.Join(dir, "Notes", "sub")); err == nil {
		t.Errorf("Notes/sub/ must not be created")
	}

	// 新しいサブフォルダ（衝突なし）は作れる
	mustOK(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "Notes/新しいサブ/a.md", "body": "b"}))
	if !strings.Contains(readFile(t, dir, "Notes/新しいサブ/a.md"), "b") {
		t.Errorf("note not created")
	}
	// 既存フォルダに正しい大文字小文字で作るのは通る
	mustOK(t, "create_note", call(t, cs, "create_note", map[string]any{"path": "Notes/Sub/z.md", "body": "b"}))
}

func TestWriteTools_NameTooLong(t *testing.T) {
	cs, _ := newTestSession(t, testNow)
	p := "Notes/" + strings.Repeat("a", 300) + ".md"
	const want = "ファイル名が長すぎます。タイトルを短くしてください"
	mustErr(t, "create_note", call(t, cs, "create_note", map[string]any{"path": p, "body": "b"}), want)
	mustErr(t, "append_note", call(t, cs, "append_note", map[string]any{"path": p, "content": "c"}), want)
	mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": p, "content": "c", "hash": "x"}), want)
}

func TestUpdateNote_EmptyContent(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	writeFile(t, dir, "Notes/a.md", "old\n")
	for _, c := range []string{"", "  \n\t"} {
		mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": "Notes/a.md", "content": c, "hash": vault.HashContent("old\n")}), "content が空です")
	}
	if got := readFile(t, dir, "Notes/a.md"); got != "old\n" {
		t.Errorf("file changed: %q", got)
	}
}

func TestUpdateNote_KeepsCreated(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	const wantMsg = "created 行が消えています。read_note の内容の frontmatter を保ったまま更新してください"
	old := "---\ncreated: 2026-01-01T00:00\ntags: [a]\n---\n\nbody\n"
	writeFile(t, dir, "Notes/a.md", old)
	h := vault.HashContent(old)

	for _, c := range []string{
		"---\ntags: [a]\n---\n\nnew body\n",
		"new body without frontmatter\n",
	} {
		mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": "Notes/a.md", "content": c, "hash": h}), wantMsg)
	}
	if got := readFile(t, dir, "Notes/a.md"); got != old {
		t.Errorf("file changed: %q", got)
	}

	// created を保っていれば通る（CRLF でも）
	keep := "---\r\ncreated: 2026-01-01T00:00\r\ntags: [a]\r\n---\r\n\r\nnew body\r\n"
	mustOK(t, "update_note", call(t, cs, "update_note", map[string]any{"path": "Notes/a.md", "content": keep, "hash": h}))
	if got := readFile(t, dir, "Notes/a.md"); got != keep {
		t.Errorf("content = %q", got)
	}

	// もともと created が無いノートは created なしでも更新できる
	oldB := "---\ntags: [a]\n---\nold\n"
	writeFile(t, dir, "Notes/b.md", oldB)
	mustOK(t, "update_note", call(t, cs, "update_note", map[string]any{"path": "Notes/b.md", "content": "plain\n", "hash": vault.HashContent(oldB)}))

	// 本文中の created: は frontmatter の created とみなさない
	oldC := "---\ncreated: 2026-01-01\n---\nx\n"
	writeFile(t, dir, "Notes/c.md", oldC)
	mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": "Notes/c.md", "content": "---\ntags: [a]\n---\ncreated: 2026-01-01\n", "hash": vault.HashContent(oldC)}), wantMsg)

	// hash が古ければ created の検査より先に hash 不一致を返す
	mustErr(t, "update_note", call(t, cs, "update_note", map[string]any{"path": "Notes/c.md", "content": "x\n", "hash": "deadbeef"}), "read_note で読み直して")
}

func TestAppendDailyMemo_InvalidDates(t *testing.T) {
	cs, dir := newTestSession(t, testNow)
	for _, d := range []string{"../x", "2026-02-30"} {
		mustErr(t, "append_daily_memo", call(t, cs, "append_daily_memo", map[string]any{"content": "x", "date": d}), "YYYY-MM-DD")
	}
	if _, err := os.Stat(filepath.Join(dir, "DailyNotes")); err == nil {
		t.Errorf("DailyNotes must not be created")
	}
}

func TestToolDescriptions_NoTopLevelHeading(t *testing.T) {
	cs, _ := newTestSession(t, testNow)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, tl := range res.Tools {
		if tl.Name != "append_note" && tl.Name != "append_daily_memo" {
			continue
		}
		seen++
		if !strings.Contains(tl.Description, "## 以下") {
			t.Errorf("%s description should mention top-level headings: %s", tl.Name, tl.Description)
		}
	}
	if seen != 2 {
		t.Errorf("seen = %d, want 2", seen)
	}
}

// headerTransport はすべてのリクエストに Cf-Access-Jwt-Assertion を付ける。
type headerTransport struct {
	token string
	base  http.RoundTripper
}

func (h *headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Cf-Access-Jwt-Assertion", h.token)
	return h.base.RoundTrip(r)
}

func TestHTTPHandler_E2E_ReadNote(t *testing.T) {
	j := newTestJWKS(t)
	dir := t.TempDir()
	writeFile(t, dir, "Notes/hello.md", "---\ntags: [x]\n---\n\nこんにちは E2E\n")
	ts := httptest.NewServer(NewHTTPHandler(testOptions(t, dir, testNow), newTestVerifier(j)))
	defer ts.Close()

	// / は JWT なしなら 403
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET / without JWT: status = %d, want 403", resp.StatusCode)
	}

	hc := &http.Client{Transport: &headerTransport{token: j.Sign(t, validClaims()), base: http.DefaultTransport}}
	client := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: ts.URL + "/mcp", HTTPClient: hc, MaxRetries: -1, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "read_note", Arguments: map[string]any{"path": "Notes/hello.md"}})
	if err != nil {
		t.Fatal(err)
	}
	mustOK(t, "read_note", res)
	var out struct{ Path, Content string }
	structured(t, res, &out)
	if out.Path != "Notes/hello.md" || !strings.Contains(out.Content, "こんにちは E2E") {
		t.Errorf("read_note = %+v", out)
	}
}
