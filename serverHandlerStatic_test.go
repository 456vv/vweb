package vweb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// setupTestEnvironment 创建用于测试的绝对路径目录和文件
// 满足严格使用绝对路径的规范
func setupTestEnvironment(t *testing.T) (rootPath string, cleanup func()) {
	// 获取系统临时目录的绝对路径
	tmpDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		t.Fatalf("Failed to get absolute temp dir: %v", err)
	}

	rootPath = filepath.Join(tmpDir, "vweb_static_test_dir")
	err = os.MkdirAll(rootPath, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// 写入测试文件 (36 bytes: 0123456789abcdefghijklmnopqrstuvwxyz)
	filePath := filepath.Join(rootPath, "testfile.txt")
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	err = os.WriteFile(filePath, content, 0o644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// 创建一个子目录用于测试 403 权限问题
	subDir := filepath.Join(rootPath, "subdir")
	os.MkdirAll(subDir, 0o755)

	cleanup = func() {
		os.RemoveAll(rootPath)
	}
	return rootPath, cleanup
}

// TestServerHandlerStatic_ServeHTTP 详细的单元测试，模拟不同参数输入
func TestServerHandlerStatic_ServeHTTP(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	handler := &ServerHandlerStatic{
		RootPath:    rootPath,
		PageExpired: 3600,
	}

	// 获取文件的真实 ETag 和 Last-Modified 用于缓存测试
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
		{
			name:           "Normal GET",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			expectedStatus: http.StatusOK,
			expectedBody:   "0123456789abcdefghijklmnopqrstuvwxyz",
		},
		{
			name:           "HEAD Request",
			method:         http.MethodHead,
			path:           "/testfile.txt",
			expectedStatus: http.StatusOK,
			expectedBody:   "", // HEAD 应该没有 body
		},
		{
			name:           "Method Not Allowed",
			method:         http.MethodPost,
			path:           "/testfile.txt",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "File Not Found",
			method:         http.MethodGet,
			path:           "/missing.txt",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Directory Forbidden",
			method:         http.MethodGet,
			path:           "/subdir",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "Single Range",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			headers:        map[string]string{"Range": "bytes=0-9"},
			expectedStatus: http.StatusPartialContent,
			expectedBody:   "0123456789",
		},
		{
			name:           "Suffix Range",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			headers:        map[string]string{"Range": "bytes=-5"},
			expectedStatus: http.StatusPartialContent,
			expectedBody:   "vwxyz",
		},
		{
			name:           "Multiple Range (Multipart)",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			headers:        map[string]string{"Range": "bytes=0-4, 10-14"},
			expectedStatus: http.StatusPartialContent,
			checkHeader: func(h http.Header) bool {
				return strings.HasPrefix(h.Get("Content-Type"), "multipart/byteranges")
			},
		},
		{
			name:           "Invalid Range",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			headers:        map[string]string{"Range": "bytes=100-200"},
			expectedStatus: http.StatusRequestedRangeNotSatisfiable,
		},
		{
			name:           "Cache Hit 304 ETag",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			headers:        map[string]string{"If-None-Match": validETag},
			expectedStatus: http.StatusNotModified,
		},
		{
			name:           "Cache Hit 304 Last-Modified",
			method:         http.MethodGet,
			path:           "/testfile.txt",
			headers:        map[string]string{"If-Modified-Since": validModTime},
			expectedStatus: http.StatusNotModified,
		},
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
				// 针对 Multipart 进一步简单校验内容
				if strings.Contains(resp.Header.Get("Content-Type"), "multipart") {
					if !strings.Contains(bodyStr, "01234") || !strings.Contains(bodyStr, "abcde") {
						t.Errorf("Multipart body missing expected chunks, got: %s", bodyStr)
					}
				}
			}
		})
	}
}

// TestServerHandlerStatic_Concurrency 多线程并发交叉调用测试
// 用于检测共享状态 (如 handler 配置、文件 IO 描述符) 是否存在竞态条件
func TestServerHandlerStatic_Concurrency(t *testing.T) {
	rootPath, cleanup := setupTestEnvironment(t)
	defer cleanup()

	handler := &ServerHandlerStatic{
		RootPath:    rootPath,
		PageExpired: 60,
	}

	var wg sync.WaitGroup
	workers := 100   // 100 个并发 Goroutine
	iterations := 50 // 每个 Goroutine 执行 50 次不同的请求

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 混合生成各种参数
				method := http.MethodGet
				if (workerID+j)%4 == 0 {
					method = http.MethodHead
				}

				req := httptest.NewRequest(method, "http://localhost/testfile.txt", nil)

				// 交错植入缓存和 Range Header
				if j%3 == 0 {
					req.Header.Set("Range", "bytes=2-8")
				} else if j%5 == 0 {
					req.Header.Set("Range", "bytes=0-1,3-5,7-9")
				}

				if j%7 == 0 {
					// 随机无效 Etag
					req.Header.Set("If-None-Match", `W/"invalid-etag"`)
				}

				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				// 仅仅读取确保过程不 Panic，检查基础状态
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

// BenchmarkServerHandlerStatic 串行基准测试
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

// BenchmarkServerHandlerStatic_Parallel 并行基准测试 (模拟真实 Web 流量)
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
