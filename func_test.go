package vweb

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"
)

func testExecFunc(a *testing.T, b ...any) *testing.T {
	return a
}

func Test_ExecFunc1(t *testing.T) {
	args := []any{t, t, t}
	rets, err := ExecFunc(testExecFunc, t, args)
	if err != nil {
		t.Fatal(err)
	}
	tt, ok := rets[0].(*testing.T)
	if !ok {
		t.Fatal("error")
	}
	if tt != t {
		t.Fatal("error")
	}
}

func Test_ExecFunc2(t *testing.T) {
	rets, err := ExecFunc(testExecFunc, t, t, t, t, t)
	if err != nil {
		t.Fatal(err)
	}
	tt, ok := rets[0].(*testing.T)
	if !ok {
		t.Fatal("error")
	}
	if tt != t {
		t.Fatal("error")
	}
}

func Test_ExecFunc3(t *testing.T) {
	rets, err := ExecFunc(testExecFunc, t)
	if err != nil {
		t.Fatal(err)
	}
	tt, ok := rets[0].(*testing.T)
	if !ok {
		t.Fatal("error")
	}
	if tt != t {
		t.Fatal("error")
	}
}

func testExecFuncError(v error) error {
	return v
}

func Test_ExecFunc4(t *testing.T) {
	rets, err := ExecFunc(testExecFuncError, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rets[0] != nil {
		t.Fatal("error")
	}
}

func Test_ExecFunc5(t *testing.T) {
	err := errors.New("error")
	rets, e := ExecFunc(testExecFuncError, err)
	if e != nil {
		t.Fatal(e)
	}
	rt, ok := rets[0].(error)
	if !ok {
		t.Fatal("error")
	}
	if rt != err {
		t.Fatal("error")
	}
}

func Benchmark_GenerateRandomString(t *testing.B) {
	length := 40
	for t.Loop() {
		code, err := GenerateRandomString(length)
		if err != nil {
			t.Fatal(err)
		}
		if l := len(code); l != length {
			t.Fatalf("生成长度错误，预定 %d，结果 %d", length, l)
		}
	}
}

func Benchmark_AddSalt_1(t *testing.B) {
	length := 40
	for t.Loop() {
		p, err := GenerateRandom(length)
		if err != nil {
			t.Fatal(err)
		}
		code := AddSalt(p, "dkeinifjperiocjopirem")
		if l := len(code); l != length {
			t.Fatalf("生成长度错误，预定 %d，结果 %d", length, l)
		}
		// t.Log(code)
	}
}

func Benchmark_AddSalt_2(t *testing.B) {
	length := 40
	for t.Loop() {
		p, err := GenerateRandom(length)
		if err != nil {
			t.Fatal(err)
		}
		code := AddSalt(p, "")
		if l := len(code); l != length {
			t.Fatalf("生成长度错误，预定 %d，结果 %d", length, l)
		}
		// t.Log(code)
	}
}

func TestGenerateRandomID_Nil(t *testing.T) {
	if err := GenerateRandomID(nil); err == nil {
		t.Fatal("nil 应报错")
	}
}

func TestGenerateRandom_Edge(t *testing.T) {
	if b, err := GenerateRandom(0); err != nil || len(b) != 0 {
		t.Fatalf("长度 0：%v %v", b, err)
	}
	if _, err := GenerateRandom(-1); err == nil {
		t.Fatal("负数长度应报错")
	}
}

func TestGenerateRandomString_Length(t *testing.T) {
	for _, n := range []int{1, 10, 64, 128} {
		s, err := GenerateRandomString(n)
		if err != nil || len(s) != n {
			t.Fatalf("长度 %d：%q %v", n, s, err)
		}
	}
}

func TestAddSalt_EmptySalt(t *testing.T) {
	if got := AddSalt([]byte{0, 1, 2, 3, 4}, ""); len(got) != 5 {
		t.Fatalf("长度=%d", len(got))
	}
}

func TestEqualDomain_IPv6(t *testing.T) {
	cases := []struct {
		host, domain string
		ok           bool
	}{
		{"[::1]:80", "[::1]:80", true},
		{"[::1]:80", "::1:80", false},
		{"example.com", "example.com", true},
		{"example.com:443", "example.com", false},
	}
	for _, c := range cases {
		if equalDomain(c.host, c.domain) != c.ok {
			t.Errorf("equalDomain(%q, %q) 期望 %v", c.host, c.domain, c.ok)
		}
	}
}

func TestDerogatoryDomain_Trailing(t *testing.T) {
	var got []string
	derogatoryDomain("a.b.c:8080", func(h string) bool { got = append(got, h); return false })
	want := []string{"a.b.c:8080", "*.b.c:8080", "*.*.c:8080", "*.*.*:8080"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDerogatoryDomain_IPShortCircuits(t *testing.T) {
	var got []string
	derogatoryDomain("1.2.3.4:80", func(h string) bool { got = append(got, h); return false })
	if len(got) != 1 {
		t.Fatalf("IP 应只全字匹配，得到 %v", got)
	}
}

func TestPagePath_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)

	if _, p, err := PagePath(dir, "/", []string{"index.html"}); err != nil || p != "/index.html" {
		t.Fatalf("根路径：p=%q err=%v", p, err)
	}
	if _, _, err := PagePath(dir, "/sub/", []string{"index.html"}); err == nil {
		t.Fatal("子目录无索引文件应报错")
	}
	os.WriteFile(filepath.Join(dir, "sub", "index.html"), []byte("y"), 0o644)
	if _, p, err := PagePath(dir, "/sub/", []string{"index.html"}); err != nil || p != "/sub/index.html" {
		t.Fatalf("p=%q err=%v", p, err)
	}
}

func TestAutoCert_NilAC(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	if got := AutoCert(nil, nil, h); got == nil {
		t.Fatal("nil autocert 应直接返回 handler")
	}
}

// 记录：url.URL 参数未使用；仅做编译覆盖
func TestPagePath_CleanTraversal(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := PagePath(dir, "/../etc/passwd", nil); err == nil {
		t.Fatal("路径穿越后应找不到文件")
	}
	_ = url.URL{}
}

func TestIsRegex(t *testing.T) {
	if isRegex(regexp.MustCompile("abc")) {
		t.Fatal("纯字面量不应被识别为正则")
	}
	if !isRegex(regexp.MustCompile("a.c")) {
		t.Fatal("含元字符应识别为正则")
	}
	if !isRegex(regexp.MustCompile("")) {
		t.Fatal("空正则应识别为正则")
	}
}

func TestDelay(t *testing.T) {
	if d := delay(0, 10*time.Millisecond); d <= 0 {
		t.Fatalf("delay(0,...)=%v", d)
	}
	if d := delay(10*time.Millisecond, 10*time.Millisecond); d != 10*time.Millisecond {
		t.Fatalf("delay(max/2,...)=%v", d)
	}
	if d := delay(20*time.Millisecond, 10*time.Millisecond); d != 10*time.Millisecond {
		t.Fatalf("delay(>max/2,...)=%v", d)
	}
}

func TestGenerateRandom_Zero(t *testing.T) {
	b, err := GenerateRandom(0)
	if err != nil || len(b) != 0 {
		t.Fatalf("长度 0：%v %v", b, err)
	}
}
