package vweb

import (
	"sync"
	"testing"
)

func TestForward_Compile_Nil(t *testing.T) {
	var f *Forward
	if _, err := f.Compile(); err == nil {
		t.Fatal("nil Forward 应报错")
	}
}

func TestForward_Compile_InvalidRegex(t *testing.T) {
	if _, err := (&Forward{Path: []string{"["}}).Compile(); err == nil {
		t.Fatal("无效 Path 正则应报错")
	}
	if _, err := (&Forward{ExcludePath: []string{"["}}).Compile(); err == nil {
		t.Fatal("无效 ExcludePath 正则应报错")
	}
}

func TestForwardRewriter_Rewrite_Nil(t *testing.T) {
	var r *ForwardRewriter
	p, ok := r.Rewrite("/x")
	if ok || p != "/x" {
		t.Fatalf("nil rewriter 应原样返回，得到 %q, %v", p, ok)
	}
}

func TestForwardRewriter_Rewrite_NoRules(t *testing.T) {
	r, err := (&Forward{}).Compile()
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := r.Rewrite("/x"); ok || p != "/x" {
		t.Fatalf("无规则应原样返回，得到 %q, %v", p, ok)
	}
}

func TestForwardRewriter_Rewrite_ExcludeExact(t *testing.T) {
	r, _ := (&Forward{
		Path:        []string{"/a"},
		ExcludePath: []string{"/skip"},
		RePath:      "/b",
	}).Compile()
	if p, ok := r.Rewrite("/skip"); ok || p != "/skip" {
		t.Fatalf("排除路径应原样返回，得到 %q, %v", p, ok)
	}
	if p, ok := r.Rewrite("/a"); !ok || p != "/b" {
		t.Fatalf("匹配路径应重写，得到 %q, %v", p, ok)
	}
}

func TestForwardRewriter_Rewrite_RegexWithCapture(t *testing.T) {
	r, _ := (&Forward{
		Path:   []string{`^/user/(\d+)$`},
		RePath: "/profile/$1",
	}).Compile()
	if p, ok := r.Rewrite("/user/123"); !ok || p != "/profile/123" {
		t.Fatalf("got %q, %v", p, ok)
	}
}

func TestForwardRewriter_Rewrite_ExcludeRegex(t *testing.T) {
	r, _ := (&Forward{
		Path:        []string{`^/a/(\d+)$`},
		ExcludePath: []string{`^/a/999$`},
		RePath:      "/b/$1",
	}).Compile()
	if p, ok := r.Rewrite("/a/999"); ok || p != "/a/999" {
		t.Fatalf("排除正则命中应原样返回，得到 %q, %v", p, ok)
	}
	if p, ok := r.Rewrite("/a/1"); !ok || p != "/b/1" {
		t.Fatalf("非排除应重写，得到 %q, %v", p, ok)
	}
}

func TestForwardRewriter_Concurrent(t *testing.T) {
	r, _ := (&Forward{
		Path:   []string{`^/x/(\d+)$`},
		RePath: "/y/$1",
	}).Compile()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if p, ok := r.Rewrite("/x/1"); !ok || p != "/y/1" {
					t.Errorf("并发 Rewrite 结果 %q,%v", p, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
}
