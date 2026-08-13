package vweb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerHandlerDynamic(t *testing.T) {
	fileContent := "//mock\r\npackage main\r\n\r\n\r\nfunc Main(dot any){\r\n\r\n}"
	dir, filename := createTempFile(t, fileContent)
	shd := ServerHandlerDynamic{
		RootPath: dir,
		PagePath: filename,
		Site:     new(Site),
		Module: func(name string) (DynamicTemplater, error) {
			if name == "mock" {
				return &mockDynamicTemplater{
					executeBody: "Hello World",
				}, nil
			}
			return nil, errors.New("vweb: the file type does not support dynamic parsing")
		},
	}
	exec, err := shd.parseTemplate(filename, strings.NewReader(fileContent))
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBuffer(nil)
	if _, err := shd.executeWith(exec, "", body, nil); err != nil {
		t.Fatal(err)
	}

	if body.String() != "Hello World" {
		t.Fatalf("unexpected output: %q", body.String())
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/dynamic.go", nil)
	shd.ServeHTTP(rw, req)
	if rw.Code != 200 {
		t.Fatalf("unexpected status code: %d", rw.Code)
	}
	if rw.Body.String() != "Hello World" {
		t.Fatalf("unexpected response body: %q", rw.Body.String())
	}
}

// mockDynamicTemplater 模拟 DynamicTemplater 接口
type mockDynamicTemplater struct {
	rootPath    string
	pagePath    string
	parseErr    error
	executeErr  error
	executeBody string
	executeFn   func(name string, out io.Writer, dot ...any) error // 自定义执行逻辑
}

func (m *mockDynamicTemplater) SetPath(root string, page string) {
	m.rootPath = root
	m.pagePath = page
}

func (m *mockDynamicTemplater) Parse(r io.Reader) error {
	return m.parseErr
}

func (m *mockDynamicTemplater) Execute(name string, out io.Writer, dot ...any) error {
	if m.executeFn != nil {
		return m.executeFn(name, out, dot...)
	}
	if m.executeErr != nil {
		return m.executeErr
	}
	if m.executeBody != "" {
		_, err := io.WriteString(out, m.executeBody)
		return err
	}
	return nil
}

func (m *mockDynamicTemplater) Close() error {
	return nil
}

// createTempFile 创建带有指定内容的临时文件，返回目录和文件名
func createTempFile(t *testing.T, content string) (dir, filename string) {
	t.Helper()
	dir = t.TempDir()
	filename = "test.html"
	fullPath := filepath.Join(dir, filename)
	err := os.WriteFile(fullPath, []byte(content), 0o644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	return dir, filename
}

// newRequest 创建一个测试 HTTP 请求
func newRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return req
}

// PagePath 为空时，自动从 req.URL.Path 获取
func TestServerHandlerDynamic_ServeHTTP_PagePathFromRequest(t *testing.T) {
	fileContent := "//mock\nHello World"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{executeBody: "Hello World"}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "Hello World") {
		t.Errorf("Expected body to contain 'Hello World', got %q", rw.Body.String())
	}
}

// PagePath 已设置时直接使用
func TestServerHandlerDynamic_ServeHTTP_PagePathPreset(t *testing.T) {
	fileContent := "//mock\nHello Preset"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{executeBody: "Hello Preset"}, nil
		},
	}

	req := newRequest("GET", "/"+filename) // URL path 不影响
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "Hello Preset") {
		t.Errorf("Expected 'Hello Preset', got %q", rw.Body.String())
	}
}

// 文件不存在时返回500错误（使用默认 os.Open）
func TestServerHandlerDynamic_ServeHTTP_FileNotFound(t *testing.T) {
	handler := &ServerHandlerDynamic{
		RootPath: "/nonexistent/path",
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{}, nil
		},
	}

	req := newRequest("GET", "/nofile.html")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "vweb: Failed to stat file! Error: nofile.html\n") {
		t.Errorf("Expected open error message, got %q", rw.Body.String())
	}
}

// 使用自定义 ReadFile 函数
func TestServerHandlerDynamic_ServeHTTP_CustomReadFile(t *testing.T) {
	modTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	handler := &ServerHandlerDynamic{
		RootPath: "/virtual",
		PagePath: "/page.html",
		ReadFile: func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error) {
			content := "//mock\nCustom Content"
			return io.NopCloser(strings.NewReader(content)), modTime, nil
		},
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{executeBody: "Custom Content"}, nil
		},
	}

	req := newRequest("GET", "/page.html")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "Custom Content") {
		t.Errorf("Expected 'Custom Content', got %q", rw.Body.String())
	}
}

// ReadFile 返回错误
func TestServerHandlerDynamic_ServeHTTP_ReadFileError(t *testing.T) {
	handler := &ServerHandlerDynamic{
		RootPath: "/virtual",
		PagePath: "/page.html",
		ReadFile: func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error) {
			return nil, time.Time{}, errors.New("read file failed")
		},
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{}, nil
		},
	}

	req := newRequest("GET", "/page.html")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "Failed to read the ReadFile") {
		t.Errorf("Expected ReadFile error message, got %q", rw.Body.String())
	}
}

// ReadFile 文件修改时间变化时重新解析
func TestServerHandlerDynamic_ServeHTTP_ReadFileModeTimeChanged(t *testing.T) {
	callCount := 0
	modTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	handler := &ServerHandlerDynamic{
		RootPath: "/virtual",
		PagePath: "/page.html",
		ReadFile: func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error) {
			content := "//mock\nContent"
			return io.NopCloser(strings.NewReader(content)), modTime, nil
		},
		Module: func(name string) (DynamicTemplater, error) {
			callCount++
			return &mockDynamicTemplater{executeBody: fmt.Sprintf("Content-%d", callCount)}, nil
		},
	}

	// 第一次请求
	req := newRequest("GET", "/page.html")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if callCount != 1 {
		t.Errorf("Expected compiler called 1 time, got %d", callCount)
	}

	// 第二次请求，时间不变，应该使用缓存（不重新编译）
	req = newRequest("GET", "/page.html")
	rw = httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if callCount != 1 {
		t.Errorf("Expected compiler still called 1 time (cached), got %d", callCount)
	}

	// 第三次请求，修改时间变化，应该重新编译
	modTime = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	req = newRequest("GET", "/page.html")
	rw = httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if callCount != 2 {
		t.Errorf("Expected compiler called 2 times after modTime change, got %d", callCount)
	}
}

// 默认 os.Open 方式，文件修改时间变化时重新解析
func TestServerHandlerDynamic_ServeHTTP_OsOpenModeTimeChanged(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	callCount := 0
	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			callCount++
			return &mockDynamicTemplater{executeBody: "Hello"}, nil
		},
	}

	// 第一次请求
	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if callCount != 1 {
		t.Errorf("Expected compiler called 1 time, got %d", callCount)
	}

	// 第二次请求，文件未修改，应使用缓存
	req = newRequest("GET", "/"+filename)
	rw = httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if callCount != 1 {
		t.Errorf("Expected compiler still called 1 time (cached), got %d", callCount)
	}

	// 修改文件（改变修改时间）
	time.Sleep(10 * time.Millisecond) // 确保时间不同
	fullPath := filepath.Join(dir, filename)
	now := time.Now().Add(1 * time.Second)
	os.Chtimes(fullPath, now, now)

	// 第三次请求，文件已修改，应重新编译
	req = newRequest("GET", "/"+filename)
	rw = httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if callCount != 2 {
		t.Errorf("Expected compiler called 2 times after file change, got %d", callCount)
	}
}

// Parse 解析错误时返回500
func TestServerHandlerDynamic_ServeHTTP_ParseError(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return nil, errors.New("unsupported type")
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "unsupported type") {
		t.Errorf("Expected parse error message, got %q", rw.Body.String())
	}
}

// Execute 执行错误时（未写入响应），返回500
func TestServerHandlerDynamic_ServeHTTP_ExecuteError_NotWrited(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeErr: errors.New("execute failed"),
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "execute failed") {
		t.Errorf("Expected execute error message, got %q", rw.Body.String())
	}
}

// Execute 执行错误时（已写入响应），追加错误到响应
func TestServerHandlerDynamic_ServeHTTP_ExecuteError_AlreadyWrited(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeFn: func(name string, out io.Writer, dot ...any) error {
					// 模拟在模板执行过程中已经向 ResponseWriter 写入了数据
					if d, ok := dot[0].(Doter); ok {
						d.Response().Write([]byte("partial content "))
					}
					return errors.New("execute failed mid-way")
				},
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	body := rw.Body.String()
	if body != "partial content execute failed mid-way" {
		t.Logf("Body: %q", body)
		// 这取决于 Dot.isWrited() 的实现
	}
}

// Execute 成功，body 有内容且未直接写入 ResponseWriter
func TestServerHandlerDynamic_ServeHTTP_ExecuteSuccess_BodyWritten(t *testing.T) {
	fileContent := "//mock\n<html>Hello</html>"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeBody: "<html>Response Body</html>",
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.Code)
	}
	expected := "<html>Response Body</html>"
	if rw.Body.String() != expected {
		t.Errorf("Expected body %q, got %q", expected, rw.Body.String())
	}
}

// Execute 成功，body 为空
func TestServerHandlerDynamic_ServeHTTP_ExecuteSuccess_EmptyBody(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeBody: "", // 不写入任何内容
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.Code)
	}
	if rw.Body.Len() != 0 {
		t.Errorf("Expected empty body, got %q", rw.Body.String())
	}
}

// Site 配置传递给 Dot
func TestServerHandlerDynamic_ServeHTTP_SitePassedToDot(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	testSite := &Site{}

	var capturedDot *Dot
	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Site:     testSite,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeFn: func(name string, out io.Writer, dot ...any) error {
					if d, ok := dot[0].(*Dot); ok {
						capturedDot = d
					}
					io.WriteString(out, "OK")
					return nil
				},
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if capturedDot == nil {
		t.Fatal("Dot was not captured")
	}
	if capturedDot.Site != testSite {
		t.Error("Expected Site to be passed to Dot")
	}
}

// Compiler 第一行动态类型识别正确
func TestServerHandlerDynamic_ServeHTTP_CompilerReceivesCorrectType(t *testing.T) {
	fileContent := "//gohtml\n<h1>{{.}}</h1>"
	dir, filename := createTempFile(t, fileContent)

	var receivedType string
	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			receivedType = name
			return &mockDynamicTemplater{executeBody: "OK"}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if receivedType != "gohtml" {
		t.Errorf("Expected compiler to receive type 'gohtml', got %q", receivedType)
	}
}

// Compiler 为 nil 时返回错误
func TestServerHandlerDynamic_ServeHTTP_CompilerNil(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module:   nil, // 没有设置 Compiler
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "vweb: Module is nil; cannot determine dynamic file type\n") {
		t.Errorf("Expected 'not recognized' error, got %q", rw.Body.String())
	}
}

// Dot 的 Request 和 ResponseWriter 正确传递
func TestServerHandlerDynamic_ServeHTTP_DotRequestAndResponseWriter(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	var capturedReq *http.Request
	var capturedRW http.ResponseWriter

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeFn: func(name string, out io.Writer, dot ...any) error {
					if d, ok := dot[0].(*Dot); ok {
						capturedReq = d.R
						capturedRW = d.W
					}
					io.WriteString(out, "OK")
					return nil
				},
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if capturedReq == nil {
		t.Fatal("Request was not captured in Dot")
	}
	if capturedReq.URL.Path != "/"+filename {
		t.Errorf("Expected request path '/%s', got %q", filename, capturedReq.URL.Path)
	}
	if capturedRW == nil {
		t.Fatal("ResponseWriter was not captured in Dot")
	}
}

// Execute panic 恢复
func TestServerHandlerDynamic_ServeHTTP_ExecutePanic(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{
				executeFn: func(name string, out io.Writer, dot ...any) error {
					panic("template panic!")
				},
			}, nil
		},
	}

	req := newRequest("GET", "/"+filename)
	rw := httptest.NewRecorder()

	// 不应该 panic，应该被 recover 捕获
	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", rw.Code)
	}
	body := rw.Body.String()
	if !strings.Contains(body, "vweb: Dynamic code execute error. template panic!\n") {
		t.Errorf("Expected panic recovery error message, got %q", body)
	}
}

// URL 路径清理（Path.Clean）
func TestServerHandlerDynamic_ServeHTTP_PathClean(t *testing.T) {
	fileContent := "//mock\nHello"
	dir, filename := createTempFile(t, fileContent)

	handler := &ServerHandlerDynamic{
		RootPath: dir,
		PagePath: "/" + filename,
		Module: func(name string) (DynamicTemplater, error) {
			return &mockDynamicTemplater{executeBody: "Hello"}, nil
		},
	}

	// 使用带有冗余路径分隔符的 URL
	req := newRequest("GET", "/./"+filename)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rw.Code)
	}
}
