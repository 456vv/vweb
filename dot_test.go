package vweb

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newDotForTest(r *http.Request) (*Dot, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	if r == nil {
		r = httptest.NewRequest("GET", "/", nil)
	}
	return &Dot{R: r, W: w, ctx: r.Context()}, w
}

func TestDot_RootDir_NilSite(t *testing.T) {
	d, _ := newDotForTest(nil)
	if got := d.RootDir("/x"); got != "." {
		t.Fatalf("Site 为 nil 应返回 '.', 得到 %q", got)
	}
	d.Site = &Site{}
	if got := d.RootDir("/x"); got != "." {
		t.Fatalf("RootDir 为 nil 应返回 '.', 得到 %q", got)
	}
	d.Site.RootDir = func(p string) string { return "/root" + p }
	if got := d.RootDir("/x"); got != "/root/x" {
		t.Fatalf("得到 %q", got)
	}
}

func TestDot_Session_Global_Nil(t *testing.T) {
	d, _ := newDotForTest(nil)
	if d.Session() != nil {
		t.Fatal("Site 为 nil 时应返回 nil Session")
	}
	if d.Global() != nil {
		t.Fatal("Site 为 nil 时应返回 nil Global")
	}
	d.Site = &Site{}
	if d.Session() != nil {
		t.Fatal("sessions 为 nil 时应返回 nil Session")
	}
	if d.Global() != nil {
		t.Fatal("global 为 nil 时应返回 nil Global")
	}
}

func TestDot_Swap(t *testing.T) {
	d, _ := newDotForTest(nil)
	m := d.Swap()
	if m == nil {
		t.Fatal("Swap 不应为 nil")
	}
	m.Set("k", "v")
	if v := d.Swap().Get("k"); v != "v" {
		t.Fatalf("Swap 状态未共享：%v", v)
	}
}

func TestDot_Defer_And_Close(t *testing.T) {
	d, _ := newDotForTest(nil)
	var called int
	if err := d.Defer(func() { called++ }); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatal("Close 前不应执行 Defer")
	}
	d.Close()
	if called != 1 {
		t.Fatalf("Close 后 Defer 应执行一次，实际 %d", called)
	}
	d.Close()
	if called != 1 {
		t.Fatal("重复 Close 不应重复执行")
	}
}

func TestDot_Context(t *testing.T) {
	d := &Dot{}
	if d.Context() == nil {
		t.Fatal("空 ctx 应返回 Background")
	}
	type k struct{}
	ctx := context.WithValue(context.Background(), k{}, "v")
	d.WithContext(ctx)
	if d.Context().Value(k{}) != "v" {
		t.Fatal("WithContext 未生效")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("WithContext(nil) 应 panic")
		}
	}()
	d.WithContext(nil)
}

func TestDot_SaveStatic_InvalidPaths(t *testing.T) {
	tmp := t.TempDir()
	d, _ := newDotForTest(nil)
	d.Site = &Site{RootDir: func(p string) string { return tmp }}
	for _, p := range []string{"", ".", "..", "../x", "/abs/path", "a/../../etc/passwd"} {
		if _, err := d.SaveStatic(p); err == nil {
			t.Errorf("SaveStatic(%q) 应报错", p)
		}
	}
}

func TestDot_SaveStatic_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	d, _ := newDotForTest(nil)
	d.Site = &Site{RootDir: func(p string) string { return tmp }}

	w, err := d.SaveStatic("sub/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("Close 后写入应报错")
	}
	got, err := os.ReadFile(filepath.Join(tmp, "sub", "a.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("内容=%q err=%v", got, err)
	}
}

func TestHandleFunc_StopsAfterWrite(t *testing.T) {
	var hits int
	h := HandleFunc{
		func(d Doter) { hits++; d.Response().Write([]byte("1")) },
		func(d Doter) { hits++ },
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if hits != 1 {
		t.Fatalf("写入后应停止，实际执行 %d 个", hits)
	}
}

func TestDot_Response_Cached(t *testing.T) {
	d, _ := newDotForTest(nil)
	if d.Response() != d.Response() {
		t.Fatal("Response() 应缓存")
	}
}

func TestDot_RequestLimitSize(t *testing.T) {
	d, _ := newDotForTest(nil)
	d.R.Body = io.NopCloser(strings.NewReader(strings.Repeat("a", 100)))
	d.RequestLimitSize(10)
	buf := make([]byte, 100)
	n, _ := d.R.Body.Read(buf)
	if n > 10 {
		t.Fatalf("限制 10 字节但读到 %d", n)
	}
}

// TestDot_SaveStatic_Boundary 路径穿越 + 关闭后写入。
func TestDot_SaveStatic_Boundary(t *testing.T) {
	tmp := t.TempDir()
	d := &Dot{R: httptest.NewRequest("GET", "/", nil), W: httptest.NewRecorder()}
	d.Site = &Site{RootDir: func(string) string { return tmp }}

	for _, p := range []string{"", ".", "..", "../x", "/abs", "a/../../etc/passwd", "./../x"} {
		if _, err := d.SaveStatic(p); err == nil {
			t.Errorf("SaveStatic(%q) 应报错", p)
		}
	}
}
