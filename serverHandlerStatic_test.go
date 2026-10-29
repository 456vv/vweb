package vweb

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupTestEnvironment(t *testing.T) (rootPath string, cleanup func()) {
	tmpDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		t.Fatalf("Failed to get absolute temp dir: %v", err)
	}
	rootPath = filepath.Join(tmpDir, "vweb_static_test_dir")
	err = os.MkdirAll(rootPath, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	filePath := filepath.Join(rootPath, "testfile.txt")
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	err = os.WriteFile(filePath, content, 0o644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	subDir := filepath.Join(rootPath, "subdir")
	os.MkdirAll(subDir, 0o755)
	cleanup = func() { os.RemoveAll(rootPath) }
	return rootPath, cleanup
}

func TestServerHandlerStatic_ServeHTTP(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	handler := &ServerHandlerStatic{RootPath: rootPath, PageExpired: 3600}
	fi, _ := os.Stat(filepath.Join(rootPath, "testfile.txt"))
	shsh := &serverHandlerStaticHeader{fileInfo: fi}
	validETag := shsh.etag()
	validModTime := fi.ModTime().UTC().Format(http.TimeFormat)

	tests := []struct {
		name           string
		method         string
		path           string
		headers        map[string]string
		expectedStatus int
		expectedBody   string
		checkHeader    func(http.Header) bool
	}{
		{"Normal GET", http.MethodGet, "/testfile.txt", nil, http.StatusOK, "0123456789abcdefghijklmnopqrstuvwxyz", nil},
		{"HEAD Request", http.MethodHead, "/testfile.txt", nil, http.StatusOK, "", nil},
		{"Method Not Allowed", http.MethodPost, "/testfile.txt", nil, http.StatusMethodNotAllowed, "", nil},
		{"File Not Found", http.MethodGet, "/missing.txt", nil, http.StatusNotFound, "", nil},
		{"Directory Forbidden", http.MethodGet, "/subdir", nil, http.StatusForbidden, "", nil},
		{"Single Range", http.MethodGet, "/testfile.txt", map[string]string{"Range": "bytes=0-9"}, http.StatusPartialContent, "0123456789", nil},
		{"Suffix Range", http.MethodGet, "/testfile.txt", map[string]string{"Range": "bytes=-5"}, http.StatusPartialContent, "vwxyz", nil},
		{"Multiple Range (Multipart)", http.MethodGet, "/testfile.txt", map[string]string{"Range": "bytes=0-4, 10-14"}, http.StatusPartialContent, "", func(h http.Header) bool {
			return strings.HasPrefix(h.Get("Content-Type"), "multipart/byteranges")
		}},
		{"Invalid Range", http.MethodGet, "/testfile.txt", map[string]string{"Range": "bytes=100-200"}, http.StatusRequestedRangeNotSatisfiable, "", nil},
		{"Cache Hit 304 ETag", http.MethodGet, "/testfile.txt", map[string]string{"If-None-Match": validETag}, http.StatusNotModified, "", nil},
		{"Cache Hit 304 Last-Modified", http.MethodGet, "/testfile.txt", map[string]string{"If-Modified-Since": validModTime}, http.StatusNotModified, "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://localhost"+tt.path, nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			resp := w.Result()
			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
			bodyBytes, _ := io.ReadAll(resp.Body)
			bodyStr := string(bodyBytes)
			resp.Body.Close()
			if tt.expectedBody != "" && bodyStr != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, bodyStr)
			}
			if tt.checkHeader != nil {
				if !tt.checkHeader(resp.Header) {
					t.Errorf("Header check failed for Content-Type: %s", resp.Header.Get("Content-Type"))
				}
				if strings.Contains(resp.Header.Get("Content-Type"), "multipart") {
					if !strings.Contains(bodyStr, "01234") || !strings.Contains(bodyStr, "abcde") {
						t.Errorf("Multipart body missing expected chunks, got: %s", bodyStr)
					}
				}
			}
		})
	}
}

func TestServerHandlerStatic_Concurrency(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	handler := &ServerHandlerStatic{RootPath: rootPath, PageExpired: 60}

	var wg sync.WaitGroup
	workers := 100
	iterations := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				method := http.MethodGet
				if (workerID+j)%4 == 0 {
					method = http.MethodHead
				}
				req := httptest.NewRequest(method, "http://localhost/testfile.txt", nil)
				if j%3 == 0 {
					req.Header.Set("Range", "bytes=2-8")
				} else if j%5 == 0 {
					req.Header.Set("Range", "bytes=0-1,3-5,7-9")
				}
				if j%7 == 0 {
					req.Header.Set("If-None-Match", `W/"invalid-etag"`)
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)
				resp := w.Result()
				_, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode == http.StatusInternalServerError {
					t.Errorf("Unexpected 500 error in concurrent execution")
				}
			}
		}(i)
	}
	wg.Wait()
}

func BenchmarkServerHandlerStatic(b *testing.B) {
	rootPath, cleanup := setupTestEnvironment(&testing.T{})
	defer cleanup()
	handler := &ServerHandlerStatic{RootPath: rootPath}
	req := httptest.NewRequest(http.MethodGet, "http://localhost/testfile.txt", nil)
	for b.Loop() {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkServerHandlerStatic_Parallel(b *testing.B) {
	rootPath, cleanup := setupTestEnvironment(&testing.T{})
	defer cleanup()
	handler := &ServerHandlerStatic{RootPath: rootPath}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/testfile.txt", nil)
		for pb.Next() {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		}
	})
}

func TestServerHandlerStatic_PathTraversal(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()
	h := &ServerHandlerStatic{RootPath: rootPath}
	for _, p := range []string{"/../x", "/a/../../x"} {
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, httptest.NewRequest("GET", p, nil))
		if rw.Code == http.StatusOK {
			t.Errorf("路径 %q 不应成功", p)
		}
	}
}

func TestServerHandlerStatic_RangeOverlapMerged(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()
	h := &ServerHandlerStatic{RootPath: rootPath}
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/testfile.txt", nil)
	req.Header.Set("Range", "bytes=0-9,5-14")
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusPartialContent {
		t.Fatalf("应 206，得到 %d", rw.Code)
	}
	if got := rw.Header().Get("Content-Range"); got != "bytes 0-14/36" {
		t.Fatalf("Content-Range=%q", got)
	}
}

func TestServerHandlerStatic_IfRangeWeakETagMismatch(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()
	h := &ServerHandlerStatic{RootPath: rootPath}
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/testfile.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	req.Header.Set("If-Range", `W/"something"`)
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("弱 ETag 应回退全量，得到 %d", rw.Code)
	}
}

func TestServerHandlerStatic_IfRangeMatchTime(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()
	h := &ServerHandlerStatic{RootPath: rootPath}
	fi, _ := os.Stat(filepath.Join(rootPath, "testfile.txt"))
	lastMod := fi.ModTime().UTC().Format(http.TimeFormat)
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/testfile.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	req.Header.Set("If-Range", lastMod)
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusPartialContent {
		t.Fatalf("If-Range 时间匹配应 206，得到 %d", rw.Code)
	}
}

func TestETagMatch(t *testing.T) {
	cases := []struct {
		header, etag string
		want         bool
	}{
		{`*`, `W/"x"`, true},
		{`W/"x"`, `W/"x"`, true},
		{`"x"`, `W/"x"`, true},
		{`W/"x"`, `"x"`, true},
		{`"y"`, `W/"x"`, false},
		{`"a", "x"`, `W/"x"`, true},
	}
	for _, c := range cases {
		if got := etagMatch(c.header, c.etag); got != c.want {
			t.Errorf("etagMatch(%q,%q)=%v 期望 %v", c.header, c.etag, got, c.want)
		}
	}
}

func TestServerHandlerStatic_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0o644)
	h := &ServerHandlerStatic{RootPath: dir}
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, httptest.NewRequest("GET", "/empty.txt", nil))
	if rw.Code != http.StatusOK || rw.Body.Len() != 0 {
		t.Fatalf("code=%d len=%d", rw.Code, rw.Body.Len())
	}
	if rw.Header().Get("Content-Length") != "0" {
		t.Fatalf("Content-Length=%q", rw.Header().Get("Content-Length"))
	}
}

func TestServerHandlerStatic_NotModifiedWithRangeHeader(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()
	h := &ServerHandlerStatic{RootPath: rootPath}
	fi, _ := os.Stat(filepath.Join(rootPath, "testfile.txt"))
	lastMod := fi.ModTime().UTC().Format(http.TimeFormat)
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/testfile.txt", nil)
	req.Header.Set("If-Modified-Since", lastMod)
	req.Header.Set("Range", "bytes=0-9")
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusNotModified {
		t.Fatalf("If-Modified-Since 命中应 304，得到 %d", rw.Code)
	}
}

func TestServerHandlerStatic_InvalidRangePrefix(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()
	h := &ServerHandlerStatic{RootPath: rootPath}
	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/testfile.txt", nil)
	req.Header.Set("Range", "items=0-9")
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("非法 Range 应 416，得到 %d", rw.Code)
	}
}

// TestStatic_Range_Boundary 汇总缓存协商与 Range 交叉边界。
func TestStatic_Range_Boundary(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "f.txt"), []byte("0123456789abcdefghijklmnopqrstuvwxyz"), 0o644)
	h := &ServerHandlerStatic{RootPath: root}
	fi, _ := os.Stat(filepath.Join(root, "f.txt"))
	lastMod := fi.ModTime().UTC().Format(http.TimeFormat)
	weakTag := fmt.Sprintf(`W/"%x-%x"`, fi.ModTime().UnixNano(), fi.Size())

	cases := []struct {
		name    string
		hdr     map[string]string
		want    int
		wantHdr func(http.Header) bool
	}{
		{"If-None-Match 命中 304", map[string]string{"If-None-Match": weakTag}, 304, nil},
		{"If-Modified-Since 命中 304", map[string]string{"If-Modified-Since": lastMod}, 304, nil},
		{"If-Range 弱 ETag 不匹配→全量 200", map[string]string{"Range": "bytes=0-9", "If-Range": weakTag}, 200, nil},
		{"If-Range 时间匹配→206", map[string]string{"Range": "bytes=0-9", "If-Range": lastMod}, 206, nil},
		{"重叠 Range 合并", map[string]string{"Range": "bytes=0-9,5-14"}, 206, func(h http.Header) bool {
			return h.Get("Content-Range") == "bytes 0-14/36"
		}},
		{"非法 Range 前缀→416", map[string]string{"Range": "items=0-9"}, 416, nil},
		{"越界 Range→416", map[string]string{"Range": "bytes=100-200"}, 416, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rw := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/f.txt", nil)
			for k, v := range c.hdr {
				req.Header.Set(k, v)
			}
			h.ServeHTTP(rw, req)
			if rw.Code != c.want {
				t.Fatalf("status=%d want=%d", rw.Code, c.want)
			}
			if c.wantHdr != nil && !c.wantHdr(rw.Header()) {
				t.Fatalf("header 校验失败：%v", rw.Header())
			}
		})
	}
}

type fakeFileInfo struct{ size int64 }

func (f *fakeFileInfo) Name() string       { return "fake" }
func (f *fakeFileInfo) Size() int64        { return f.size }
func (f *fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f *fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f *fakeFileInfo) IsDir() bool        { return false }
func (f *fakeFileInfo) Sys() any           { return nil }

func TestServerHandlerStatic_Ranges_Boundary(t *testing.T) {
	h := &serverHandlerStaticHeader{fileInfo: &fakeFileInfo{size: 100}}

	cases := []struct {
		in    string
		wantN int64
		wantK int // 期望合并后的块数
	}{
		{"bytes=0-9", 10, 1},
		{"bytes=0-9,10-19", 20, 1}, // 相邻合并
		{"bytes=0-9,5-14", 15, 1},  // 重叠合并
		{"bytes=0-9,20-29", 20, 2}, // 不连续
		{"bytes=-10", 10, 1},       // 后缀范围
		{"bytes=50-", 50, 1},       // 到文件末尾
		{"bytes=200-300", 0, 0},    // 全越界
		{"bytes=0-99", 100, 1},
		{"bytes=0-200", 100, 1}, // 截断
		{"bytes=,", 0, 0},
		{"invalid", 0, 0},
		{"items=0-9", 0, 0},
		{"bytes=0-0", 1, 1},
		{"bytes=-1", 1, 1},
		{"bytes=99-99", 1, 1},
	}
	for _, c := range cases {
		r, n := h.ranges(c.in)
		if n != c.wantN {
			t.Errorf("ranges(%q) n=%d，期望 %d", c.in, n, c.wantN)
		}
		if len(r) != c.wantK {
			t.Errorf("ranges(%q) 块数=%d，期望 %d", c.in, len(r), c.wantK)
		}
	}
}

func TestServerHandlerStatic_Ranges_EmptyFile(t *testing.T) {
	h := &serverHandlerStaticHeader{fileInfo: &fakeFileInfo{size: 0}}
	if r, n := h.ranges("bytes=0-9"); n != 0 || r != nil {
		t.Fatalf("空文件应返回 (nil,0)，得到 %v,%d", r, n)
	}
}
