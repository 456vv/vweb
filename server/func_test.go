package server

import (
	"net/http"
	"testing"

	"github.com/456vv/vweb/v3/server/config"
)

func Test_derogatoryDomain(t *testing.T) {
	derogatoryDomain("www.x.com", func(host string) bool {
		t.Log(host)
		return false
	})
}

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		in, host, port string
	}{
		{"a.com:80", "a.com", ":80"},
		{"[::1]:80", "::1", ":80"},
		{"[::1]", "::1", ""},
		{"a.com", "a.com", ""},
	}
	for _, c := range cases {
		h, p := splitHostPort(c.in)
		if h != c.host || p != c.port {
			t.Errorf("splitHostPort(%q)=(%q,%q) 期望 (%q,%q)", c.in, h, p, c.host, c.port)
		}
	}
}

func TestDerogatoryDomain_IPShortCircuit(t *testing.T) {
	var got []string
	derogatoryDomain("1.2.3.4", func(h string) bool { got = append(got, h); return false })
	if len(got) != 1 {
		t.Fatalf("IP 应只全字匹配一次，得到 %v", got)
	}
}

func TestSiteHeaderType(t *testing.T) {
	h := http.Header{}
	siteHeaderType(h, map[string]config.SiteHeaderType{
		".html": {Header: map[string][]string{"X-Test": {"a"}}},
		"*":     {Header: map[string][]string{"X-Any": {"b"}}},
	}, ".html")
	if h.Get("X-Test") != "a" {
		t.Fatal("精确匹配未生效")
	}
	if h.Get("X-Any") != "" {
		t.Fatal("精确匹配命中时不应再叠加通配")
	}

	h2 := http.Header{}
	siteHeaderType(h2, map[string]config.SiteHeaderType{
		"*": {Header: map[string][]string{"X-Any": {"b"}}},
	}, ".css")
	if h2.Get("X-Any") != "b" {
		t.Fatal("通配未生效")
	}
}

func TestIsLocalPath(t *testing.T) {
	// 只测平台无关的 ".." 与基本路径；绝对路径/盘符检查依赖平台，略过
	good := []string{"", ".", "a", "a/b", "a/b.txt"}
	bad := []string{"..", "../a", "a/..", "a/../b"}
	for _, p := range good {
		if !isLocalPath(p) {
			t.Errorf("isLocalPath(%q) 应返回 true", p)
		}
	}
	for _, p := range bad {
		if isLocalPath(p) {
			t.Errorf("isLocalPath(%q) 应返回 false", p)
		}
	}
}

func TestStrSliceContains(t *testing.T) {
	if !strSliceContains([]string{"a", "b"}, "b") {
		t.Fatal("应包含")
	}
	if strSliceContains([]string{"a"}, "b") {
		t.Fatal("不应包含")
	}
	if strSliceContains(nil, "a") {
		t.Fatal("nil 应返回 false")
	}
}
