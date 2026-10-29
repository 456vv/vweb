package vweb

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoute_ServeHTTP(t *testing.T) {
	t.Run("exact path", func(t *testing.T) {
		var r Route
		r.HandleFunc("/test", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("test response"))
		})
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Body.String() != "test response" {
			t.Fatalf("expected status 200 and body 'test response', got %d / %q", w.Code, w.Body.String())
		}
	})

	t.Run("regex path", func(t *testing.T) {
		var r Route
		r.HandleFunc("^/user/[0-9]+$", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("user"))
		})
		req := httptest.NewRequest("GET", "http://example.com/user/123", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Body.String() != "user" {
			t.Fatalf("expected regex route to match and return 'user', got %d / %q", w.Code, w.Body.String())
		}
	})

	t.Run("wildcard path", func(t *testing.T) {
		var r Route
		r.HandleFunc("/wild/*", func(w http.ResponseWriter, req *http.Request) {
			w.Write([]byte("wildcard"))
		})
		req := httptest.NewRequest("GET", "http://example.com/wild/foo", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Body.String() != "wildcard" {
			t.Fatalf("expected wildcard route to match and return 'wildcard', got %d / %q", w.Code, w.Body.String())
		}
	})

	t.Run("custom handler error", func(t *testing.T) {
		var r Route
		r.HandlerError = func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			w.Write([]byte("custom error"))
		}
		req := httptest.NewRequest("GET", "http://example.com/nothing", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusTeapot || w.Body.String() != "custom error" {
			t.Fatalf("expected custom handler error to run, got %d / %q", w.Code, w.Body.String())
		}
	})

	t.Run("site manager sets request context", func(t *testing.T) {
		var r Route
		sm := new(SiteMan)
		site := new(Site)
		sm.Add("example.com", site)
		r.SetSiteMan(sm)
		r.HandleFuncDot("/dot", func(d Doter) {
			d.Response().Write([]byte("ok"))
		})
		req := httptest.NewRequest("GET", "http://example.com/dot", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK || w.Body.String() != "ok" {
			t.Fatalf("expected dot route to respond 'ok', got %d / %q", w.Code, w.Body.String())
		}
	})

	t.Run("missing site host returns 500", func(t *testing.T) {
		var r Route
		sm := new(SiteMan)
		r.SetSiteMan(sm)
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 when host is not found, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Not supported Hijacker") {
			t.Fatalf("expected hijack unsupported error body, got %q", w.Body.String())
		}
	})

	t.Run("Path placeholder test", func(t *testing.T) {
		var r Route
		r.HandleFunc("/test/{a}/b", func(w http.ResponseWriter, r *http.Request) {
			if params, ok := r.Context().Value("url-params").(map[string]string); ok {
				w.Write(fmt.Appendf(nil, "a: %v", params["a"]))
			}
		})
		req := httptest.NewRequest("GET", "http://example.com/test/123456/b", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Body.String() != "a: 123456" {
			t.Fatalf("expected path placeholder to work, got %q", w.Body.String())
		}
	})
}

func TestRoute_NilHandlerRemoves(t *testing.T) {
	var r Route
	called := 0
	r.HandleFunc("/x", func(w http.ResponseWriter, req *http.Request) { called++ })
	r.HandleFunc("/x", nil)
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest("GET", "/x", nil))
	if called != 0 {
		t.Fatal("删除后 handler 不应被调用")
	}
	if rw.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", rw.Code)
	}
}

func TestRoute_DefaultErrorHandler(t *testing.T) {
	var r Route
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest("GET", "/none", nil))
	if rw.Code != http.StatusNotFound {
		t.Fatalf("期望 404，得到 %d", rw.Code)
	}
	if rw.Header().Get("Connection") != "close" {
		t.Fatal("默认错误应设置 Connection: close")
	}
}

func TestRoute_ExactBeatsPattern(t *testing.T) {
	var r Route
	var hits []string
	r.HandleFunc("^/x$", func(w http.ResponseWriter, req *http.Request) { hits = append(hits, "regex") })
	r.HandleFunc("/x", func(w http.ResponseWriter, req *http.Request) { hits = append(hits, "exact") })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if len(hits) == 0 || hits[0] != "exact" {
		t.Fatalf("精确匹配应优先，实际=%v", hits)
	}
}

func TestRoute_PlaceholderOverwrite(t *testing.T) {
	var r Route
	r.HandleFunc("/a/{x}/b/{x}/c", func(w http.ResponseWriter, req *http.Request) {
		params := req.Context().Value("url-params").(map[string]string)
		w.Write([]byte(params["x"]))
	})
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest("GET", "/a/1/b/2/c", nil))
	if rw.Body.String() != "2" {
		t.Fatalf("重叠占位符应后者覆盖，得到 %q", rw.Body.String())
	}
}

func TestRoute_InvalidPlaceholderPanics(t *testing.T) {
	cases := []string{"/a/{}/b", "/a/{a}{b}/c", "/a/{a/b", "/a/a}/b"}
	for _, p := range cases {
		func(p string) {
			defer func() {
				if recover() == nil {
					t.Errorf("非法占位符 %q 应 panic", p)
				}
			}()
			var r Route
			r.HandleFunc(p, func(w http.ResponseWriter, req *http.Request) {})
		}(p)
	}
}

func TestRoute_DirMismatch(t *testing.T) {
	var r Route
	r.HandleFunc("/a/", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("dir")) })
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest("GET", "/a", nil))
	if rw.Code != http.StatusNotFound {
		t.Fatalf("目录路由不应匹配无尾斜杠路径，得到 %d", rw.Code)
	}
}

func TestRoute_ContextContainsRouter(t *testing.T) {
	var r Route
	r.HandleFunc("/x", func(w http.ResponseWriter, req *http.Request) {
		if req.Context().Value(RouterContextKey) == nil {
			t.Fatal("上下文应包含 RouterContextKey")
		}
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
}

func TestRoute_WildcardDoesNotCrossSlash(t *testing.T) {
	var r Route
	r.HandleFunc("/wild/*", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("w")) })

	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest("GET", "/wild/foo", nil))
	if rw.Body.String() != "w" {
		t.Fatalf("单段通配应匹配，得到 %q", rw.Body.String())
	}

	rw2 := httptest.NewRecorder()
	r.ServeHTTP(rw2, httptest.NewRequest("GET", "/wild/foo/bar", nil))
	if rw2.Body.String() == "w" {
		t.Fatalf("path.Match 的 * 不跨 /，多段路径不应匹配。若期望跨段请调整实现")
	}
}

func TestRoute_SetSiteMan_Nil(t *testing.T) {
	var r Route
	r.SetSiteMan(nil)
	r.HandleFunc("/x", func(w http.ResponseWriter, req *http.Request) {})
	rw := httptest.NewRecorder()
	r.ServeHTTP(rw, httptest.NewRequest("GET", "/x", nil))
	if rw.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", rw.Code)
	}
}

// TestRoute_Boundary 覆盖精确/正则优先、重叠占位符、非法占位符、目录严格匹配。
func TestRoute_Boundary(t *testing.T) {
	t.Run("精确优先于正则", func(t *testing.T) {
		var r Route
		var hit []string
		r.HandleFunc("^/x$", func(http.ResponseWriter, *http.Request) { hit = append(hit, "regex") })
		r.HandleFunc("/x", func(http.ResponseWriter, *http.Request) { hit = append(hit, "exact") })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
		if len(hit) == 0 || hit[0] != "exact" {
			t.Fatalf("精确应优先，实际=%v", hit)
		}
	})

	t.Run("通配 * 不跨 /", func(t *testing.T) {
		var r Route
		r.HandleFunc("/wild/*", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("w")) })
		rw := httptest.NewRecorder()
		r.ServeHTTP(rw, httptest.NewRequest("GET", "/wild/foo/bar", nil))
		if rw.Body.String() == "w" {
			t.Fatal("path.Match 的 * 不应跨 /")
		}
	})
}
