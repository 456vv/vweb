package vweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCookie_Get(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "a", Value: "1"})
	r.AddCookie(&http.Cookie{Name: "b", Value: "2"})

	c := &Cookie{R: r, W: httptest.NewRecorder()}
	if got := c.Get("a"); got != "1" {
		t.Fatalf("Get(a)=%q", got)
	}
	if got := c.Get("b"); got != "2" {
		t.Fatalf("Get(b)=%q", got)
	}
	if got := c.Get("missing"); got != "" {
		t.Fatalf("Get(missing)=%q，期望空", got)
	}
}

func TestCookie_ReadAll(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "a", Value: "1"})
	r.AddCookie(&http.Cookie{Name: "b", Value: "2"})

	c := &Cookie{R: r, W: httptest.NewRecorder()}
	all := c.ReadAll()
	if len(all) != 2 || all["a"] != "1" || all["b"] != "2" {
		t.Fatalf("ReadAll=%v", all)
	}
}

func TestCookie_ReadAll_Empty(t *testing.T) {
	c := &Cookie{R: httptest.NewRequest("GET", "/", nil), W: httptest.NewRecorder()}
	if all := c.ReadAll(); len(all) != 0 {
		t.Fatalf("空请求应返回空 map，得到 %v", all)
	}
}

func TestCookie_Add(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Cookie{R: httptest.NewRequest("GET", "/", nil), W: w}
	c.Add("a", "1", "", "", 0, false, false, http.SameSiteDefaultMode)
	got := w.Header().Get("Set-Cookie")
	if !strings.Contains(got, "a=1") {
		t.Fatalf("Set-Cookie=%q", got)
	}
	// path 为空时应默认为 "/"
	if !strings.Contains(got, "Path=/") {
		t.Fatalf("Path 默认值未生效：%q", got)
	}
}

func TestCookie_Add_EmptyName(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Cookie{R: httptest.NewRequest("GET", "/", nil), W: w}
	c.Add("", "1", "/", "", 0, false, false, http.SameSiteDefaultMode)
	if got := w.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("空名不应写入，得到 %q", got)
	}
}

func TestCookie_Add_SameSiteNone(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Cookie{R: httptest.NewRequest("GET", "/", nil), W: w}
	c.Add("a", "1", "/", "", 0, false, false, http.SameSiteNoneMode)
	got := w.Header().Get("Set-Cookie")
	if !strings.Contains(got, "SameSite=None") {
		t.Fatalf("Set-Cookie=%q", got)
	}
}

func TestCookie_Add_InvalidSameSiteFallsBack(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Cookie{R: httptest.NewRequest("GET", "/", nil), W: w}
	c.Add("a", "1", "/", "", 0, false, false, http.SameSite(99))
	got := w.Header().Get("Set-Cookie")
	if !strings.Contains(got, "a=1") {
		t.Fatalf("Set-Cookie=%q", got)
	}
	// 非法 SameSite 不应出现在 Set-Cookie
	if strings.Contains(got, "SameSite") {
		t.Fatalf("非法 SameSite 不应出现在 Set-Cookie：%q", got)
	}
}

func TestCookie_Del(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Cookie{R: httptest.NewRequest("GET", "/", nil), W: w}
	c.Del("a")
	got := w.Header().Get("Set-Cookie")
	if !strings.Contains(got, "a=") {
		t.Fatalf("Set-Cookie=%q", got)
	}
	if !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("删除 cookie 未设置 Max-Age=0：%q", got)
	}
}

func TestCookie_RemoveAll(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "a", Value: "1"})
	r.AddCookie(&http.Cookie{Name: "b", Value: "2"})
	w := httptest.NewRecorder()
	c := &Cookie{R: r, W: w}
	c.RemoveAll()
	if vals := w.Header().Values("Set-Cookie"); len(vals) < 2 {
		t.Fatalf("应删除 2 个 cookie，得到 %v", vals)
	}
}
