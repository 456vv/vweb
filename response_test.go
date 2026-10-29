package vweb

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponse_Write_WriteString(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)

	resp := &response{w: w, r: r}
	if n, err := resp.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("Write=%d,%v", n, err)
	}
	if !resp.isWrited {
		t.Fatal("isWrited 应为 true")
	}

	resp2 := &response{w: w, r: r}
	if n, err := resp2.WriteString("world"); err != nil || n != 5 {
		t.Fatalf("WriteString=%d,%v", n, err)
	}
	if !resp2.isWrited {
		t.Fatal("isWrited 应为 true")
	}
	if w.Body.String() != "helloworld" {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestResponse_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	resp.WriteHeader(http.StatusCreated)
	if !resp.isWrited {
		t.Fatal("isWrited 应为 true")
	}
	if resp.statusCode != http.StatusCreated {
		t.Fatalf("statusCode=%d", resp.statusCode)
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("w.Code=%d", w.Code)
	}
}

func TestResponse_Header(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	resp.Header().Set("X-Test", "1")
	if w.Header().Get("X-Test") != "1" {
		t.Fatal("Header 未共享")
	}
}

func TestResponse_Error(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	resp.Error("boom", http.StatusBadRequest)
	if !resp.isWrited {
		t.Fatal("isWrited 应为 true")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "boom") {
		t.Fatalf("body=%q", w.Body.String())
	}
}

func TestResponse_Redirect(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/old", nil)
	resp := &response{w: w, r: r}
	resp.Redirect("/new", http.StatusFound)
	if !resp.isWrited {
		t.Fatal("isWrited 应为 true")
	}
	if w.Code != http.StatusFound {
		t.Fatalf("code=%d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/new" {
		t.Fatalf("Location=%q", loc)
	}
}

func TestResponse_ReadFrom(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	n, err := resp.ReadFrom(bytes.NewReader([]byte("hello world")))
	if err != nil || n != 11 {
		t.Fatalf("ReadFrom=%d,%v", n, err)
	}
	if w.Body.String() != "hello world" {
		t.Fatalf("body=%q", w.Body.String())
	}
	if !resp.isWrited {
		t.Fatal("ReadFrom 应置 isWrited")
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("read failed") }

func TestResponse_ReadFrom_Error(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	n, err := resp.ReadFrom(errReader{})
	if err == nil {
		t.Fatal("应报错")
	}
	if n != 0 {
		t.Fatalf("n=%d", n)
	}
}

func TestResponse_Push_Unsupported(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	if err := resp.Push("/x", nil); err == nil {
		t.Fatal("ResponseRecorder 不支持 Push，应报错")
	}
}

func TestResponse_Hijack_Unsupported(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &response{w: w, r: httptest.NewRequest("GET", "/", nil)}
	if _, _, err := resp.Hijack(); err == nil {
		t.Fatal("ResponseRecorder 不支持 Hijack，应报错")
	}
}

func TestResponse_Flush_NotSupported(t *testing.T) {
	resp := &response{w: &noFlushWriter{h: http.Header{}}, r: httptest.NewRequest("GET", "/", nil)}
	// 不 panic 即通过
	resp.Flush()
}

type noFlushWriter struct{ h http.Header }

func (w *noFlushWriter) Header() http.Header         { return w.h }
func (w *noFlushWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *noFlushWriter) WriteHeader(int)             {}
