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
