package vweb

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
	if rw.Code != http.StatusOK {
		t.Fatalf("非法 Range 应 200，得到 %d", rw.Code)
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
		{"非法 Range 前缀→200", map[string]string{"Range": "items=0-9"}, 200, nil},
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

func writeTestFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// rangeHeaderN 生成 n 段互不相邻的单字节区间：bytes=0-0,2-2,...
func rangeHeaderN(n int) string {
	var b strings.Builder
	b.WriteString("bytes=")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%d-%d", i*2, i*2)
	}
	return b.String()
}

// TestRangeCountLimit 验证 Range 段数边界：99/100 段 multipart，101 段忽略 Range。
func TestRangeCountLimit(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 251)
	}
	writeTestFile(t, dir, "f.bin", data)
	h := &ServerHandlerStatic{RootPath: dir}

	cases := []struct {
		name          string
		n             int
		wantStatus    int
		wantMultipart bool
		wantBodyLen   int
	}{
		{"99_segments", 99, http.StatusPartialContent, true, 0},
		{"100_segments", 100, http.StatusPartialContent, true, 0},
		{"101_segments_ignored", 101, http.StatusOK, false, len(data)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/f.bin", nil)
			req.Header.Set("Range", rangeHeaderN(tc.n))
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			ct := rec.Header().Get("Content-Type")
			isMultipart := strings.HasPrefix(ct, "multipart/byteranges")
			if isMultipart != tc.wantMultipart {
				t.Fatalf("multipart = %v, want %v (Content-Type=%q)", isMultipart, tc.wantMultipart, ct)
			}
			if tc.wantStatus == http.StatusOK && rec.Body.Len() != tc.wantBodyLen {
				t.Fatalf("body len = %d, want %d", rec.Body.Len(), tc.wantBodyLen)
			}
		})
	}
}

// TestCacheControlNotOverridden 验证 PageExpired=0 时不再用 no-cache 覆盖站点已设置的 Cache-Control。
func TestCacheControlNotOverridden(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", []byte("hello"))
	h := &ServerHandlerStatic{RootPath: dir, PageExpired: 0}

	rec := httptest.NewRecorder()
	rec.Header().Set("Cache-Control", "public, max-age=3600")
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/a.txt", nil))

	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want public, max-age=3600", got)
	}
}

// TestIfRange 验证 If-Range 不匹配（弱 ETag / 过期时间）时回退到 200 全量。
func TestIfRange(t *testing.T) {
	dir := t.TempDir()
	const body = "0123456789"
	writeTestFile(t, dir, "a.txt", []byte(body))
	h := &ServerHandlerStatic{RootPath: dir}

	// 先取一次 ETag
	rec0 := httptest.NewRecorder()
	h.ServeHTTP(rec0, httptest.NewRequest("GET", "/a.txt", nil))
	etag := rec0.Header().Get("ETag")
	if !strings.HasPrefix(etag, `W/`) {
		t.Fatalf("etag = %q, want weak prefix W/", etag)
	}

	cases := []struct {
		name    string
		ifRange string
	}{
		{"weak_etag_never_matches", etag},
		{"time_mismatch", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/a.txt", nil)
			req.Header.Set("Range", "bytes=0-4")
			req.Header.Set("If-Range", tc.ifRange)
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if rec.Body.Len() != len(body) {
				t.Fatalf("body len = %d, want %d", rec.Body.Len(), len(body))
			}
		})
	}
}

// TestNotModified 验证 304 路径（阶段 A 命中，不 Open）。
func TestNotModified(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", []byte("hello"))
	h := &ServerHandlerStatic{RootPath: dir}

	rec0 := httptest.NewRecorder()
	h.ServeHTTP(rec0, httptest.NewRequest("GET", "/a.txt", nil))
	etag := rec0.Header().Get("ETag")
	lastMod := rec0.Header().Get("Last-Modified")

	cases := []struct {
		name   string
		header string
		value  string
	}{
		{"If-None-Match", "If-None-Match", etag},
		{"If-Modified-Since", "If-Modified-Since", lastMod},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/a.txt", nil)
			req.Header.Set(tc.header, tc.value)
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotModified {
				t.Fatalf("status = %d, want 304", rec.Code)
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("body len = %d, want 0", rec.Body.Len())
			}
		})
	}
}

// TestEmptyFile 验证空文件返回 200 + Content-Length: 0。
func TestEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "empty.txt", nil)
	h := &ServerHandlerStatic{RootPath: dir}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/empty.txt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "0" {
		t.Fatalf("Content-Length = %q, want 0", cl)
	}
}

// TestContentLengthMatchesBody 验证响应头 Content-Length 与正文长度、文件大小一致（S1 契约）。
func TestContentLengthMatchesBody(t *testing.T) {
	dir := t.TempDir()
	data := []byte("0123456789abcdef")
	writeTestFile(t, dir, "a.txt", data)
	h := &ServerHandlerStatic{RootPath: dir}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/a.txt", nil))

	if cl := rec.Header().Get("Content-Length"); cl != fmt.Sprint(len(data)) {
		t.Fatalf("Content-Length = %q, want %d", cl, len(data))
	}
	if rec.Body.Len() != len(data) {
		t.Fatalf("body len = %d, want %d", rec.Body.Len(), len(data))
	}
}

// TestHead 验证 HEAD 返回头、无正文。
func TestHead(t *testing.T) {
	dir := t.TempDir()
	data := []byte("0123456789")
	writeTestFile(t, dir, "a.txt", data)
	h := &ServerHandlerStatic{RootPath: dir}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("HEAD", "/a.txt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cl := rec.Header().Get("Content-Length"); cl != fmt.Sprint(len(data)) {
		t.Fatalf("Content-Length = %q, want %d", cl, len(data))
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body len = %d, want 0", rec.Body.Len())
	}
}

// TestSingleRange 验证单 Range 返回 206 与正确 Content-Range。
func TestSingleRange(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", []byte("0123456789"))
	h := &ServerHandlerStatic{RootPath: dir}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/a.txt", nil)
	req.Header.Set("Range", "bytes=2-5")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q, want bytes 2-5/10", cr)
	}
	if rec.Body.String() != "2345" {
		t.Fatalf("body = %q, want 2345", rec.Body.String())
	}
}

// TestRangeNotSatisfiable 验证越界 Range 返回 416。
func TestRangeNotSatisfiable(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", []byte("0123456789"))
	h := &ServerHandlerStatic{RootPath: dir}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/a.txt", nil)
	req.Header.Set("Range", "bytes=100-200")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("status = %d, want 416", rec.Code)
	}
}

// TestPathTraversal404 验证 URL 路径中的 ../ 被 path.Clean 消除后 404。
func TestPathTraversal404(t *testing.T) {
	dir := t.TempDir()
	h := &ServerHandlerStatic{RootPath: dir}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/../etc/passwd", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// TestSymlinkEscapeForbidden 验证 root 内符号链接指向 root 外时返回 403。
func TestSymlinkEscapeForbidden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink requires privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "secret.txt", []byte("secret"))
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	h := &ServerHandlerStatic{RootPath: root}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/link.txt", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestSameFile 单元测试：size/mtime 相同视为同一版本，变化则不同。
func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	p := writeTestFile(t, dir, "a.txt", []byte("x"))

	fi1, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	fi2, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !sameFile(fi1, fi2) {
		t.Fatal("sameFile(fi1, fi2) = false, want true")
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(p, []byte("yy"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi3, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if sameFile(fi1, fi3) {
		t.Fatal("sameFile(fi1, fi3) = true, want false after modification")
	}
}
