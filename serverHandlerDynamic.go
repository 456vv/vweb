package vweb

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d}
	utf8BOM   = []byte{0xef, 0xbb, 0xbf}

	// 缓存复用 bytes.Buffer，减少高并发下的 GC 压力
	bufferPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
)

// 限制放回 pool 的 buffer 大小，防止个别特大页面占用过多常驻内存
func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() > 1024*1024 { // 超过 1MB 则丢弃
		return
	}
	buf.Reset()
	bufferPool.Put(buf)
}

type DynamicTemplater interface {
	SetPath(root string, page string)
	Parse(r io.Reader) (err error)                        // 解析
	Execute(name string, out io.Writer, dot ...any) error // 执行
	Close() error
}

type DynamicTemplateFunc func(*ServerHandlerDynamic) DynamicTemplater

// 引用计数包装器，用以解决生命周期并发安全问题
type refCountedTemplater struct {
	DynamicTemplater
	mu        sync.Mutex
	refs      int
	destroyed bool
}

func (r *refCountedTemplater) Close() error {
	r.mu.Lock()
	if r.destroyed {
		r.mu.Unlock()
		return nil
	}
	r.destroyed = true
	r.refs-- // 扣除缓存在 T.exec 的引用
	shouldClose := r.refs <= 0
	r.mu.Unlock()

	if shouldClose {
		return r.DynamicTemplater.Close()
	}
	return nil
}

func (r *refCountedTemplater) acquire() bool {
	r.mu.Lock()
	if r.destroyed {
		r.mu.Unlock()
		return false
	}
	r.refs++
	r.mu.Unlock()
	return true
}

func (r *refCountedTemplater) release() {
	r.mu.Lock()
	r.refs--
	shouldClose := r.refs <= 0 && r.destroyed
	r.mu.Unlock()

	if shouldClose {
		r.DynamicTemplater.Close()
	}
}

// web错误调用
func webError(rw http.ResponseWriter, v ...any) {
	// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
	http.Error(rw, fmt.Sprint(v...), http.StatusInternalServerError)
}

// ServerHandlerDynamic 处理动态页面文件
type ServerHandlerDynamic struct {
	// 必须的
	RootPath, PagePath string

	// 可选的
	Site   *Site                                       // 网站配置
	Module func(name string) (DynamicTemplater, error) // 支持更动态文件类型

	ReadFile func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error) // 读取文件。仅在 .ServeHTTP 方法中使用
	mu       sync.RWMutex                                                        // 保护 exec / modeTime 的并发安全
	exec     *refCountedTemplater
	modeTime time.Time
}

// ServeHTTP 服务HTTP
//
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if T == nil {
		webError(rw, "vweb: ServerHandlerDynamic is nil")
		return
	}

	// 获取或创建模板执行器
	tplExec, err := T.getOrCreateExecutor(req)
	if err != nil {
		webError(rw, err.Error())
		return
	}
	// 执行完毕后，释放计数引用
	defer tplExec.release()

	// 准备执行上下文
	dock := T.prepareDot(rw, req)
	defer dock.Close()

	// 从 Pool 获取缓冲区
	body := bufferPool.Get().(*bytes.Buffer)
	defer putBuffer(body)

	callName := entryname(req.URL.Path)
	if stack, err := T.executeWith(tplExec, callName, body, dock); err != nil {
		log.Printf("执行模板错误 %s: \n%v\n", req.URL.Path, err)
		if stack != nil {
			log.Println(string(stack))
		}
		if !dock.isWrited() {
			webError(rw, err.Error())
			return
		}
		// 已经开始写入响应，只能尽量把错误信息追加写出
		io.WriteString(rw, err.Error())
		return
	}

	if !dock.isWrited() && body.Len() > 0 {
		// 写入到浏览器页面中去
		body.WriteTo(rw)
	}
}

// getOrCreateExecutor 获取或创建模板执行器
func (T *ServerHandlerDynamic) getOrCreateExecutor(req *http.Request) (*refCountedTemplater, error) {
	// 确定页面路径（URL 路径风格）
	pagePath := T.PagePath
	if pagePath == "" {
		pagePath = path.Clean(req.URL.Path)
	}
	// 规范化并去除可能的前导斜杠，后续用 filepath 拼接
	pagePath = strings.TrimPrefix(pagePath, "/")
	if pagePath == "" || pagePath == "." {
		pagePath = "index"
	}

	// 跨平台路径拼接
	filePath := filepath.Join(T.RootPath, filepath.FromSlash(pagePath))

	// 严格防止路径穿越：最终路径必须位于 RootPath 之下
	rootAbs, err := filepath.Abs(T.RootPath)
	if err != nil {
		return nil, fmt.Errorf("vweb: invalid RootPath: %w", err)
	}
	fileAbs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("vweb: invalid file path: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, fileAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.HasPrefix(rel, "../") {
		return nil, errors.New("vweb: path traversal detected")
	}

	// ---------- 快速路径：先检查缓存是否仍然有效（不打开文件内容） ----------
	var modeTime time.Time
	if T.ReadFile == nil {
		// 本地文件系统：仅 Stat，避免不必要的 Open
		fi, statErr := os.Stat(filePath)
		if statErr != nil {
			// 文件被删除或损坏时，自动清理陈旧缓存
			T.mu.Lock()
			if T.exec != nil {
				T.exec.Close()
				T.exec = nil
				T.modeTime = time.Time{}
			}
			T.mu.Unlock()
			return nil, fmt.Errorf("vweb: Failed to stat file! Error: %s", rel)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("vweb: path is a directory: %s", rel)
		}
		modeTime = fi.ModTime()
	} else {
		// 自定义 ReadFile：必须调用一次才能拿到时间戳。
		r, mt, rfErr := T.ReadFile(filePath, req.URL)
		if rfErr != nil {
			T.mu.Lock()
			if T.exec != nil {
				T.exec.Close()
				T.exec = nil
				T.modeTime = time.Time{}
			}
			T.mu.Unlock()
			return nil, fmt.Errorf("vweb: Failed to read the ReadFile! Error: %s", rel)
		}
		defer r.Close()

		modeTime = mt
		// 先检查缓存，命中则安全关闭 reader 并复用
		T.mu.RLock()
		cachedExec := T.exec
		cachedTime := T.modeTime
		if cachedExec != nil && !cachedTime.IsZero() && cachedTime.Equal(modeTime) {
			if cachedExec.acquire() {
				T.mu.RUnlock()
				return cachedExec, nil
			}
		}
		T.mu.RUnlock()

		return T.parseAndCache(pagePath, r, modeTime)
	}

	// 本地文件：检查缓存
	T.mu.RLock()
	cachedExec := T.exec
	cachedTime := T.modeTime
	if cachedExec != nil && !cachedTime.IsZero() && cachedTime.Equal(modeTime) {
		if cachedExec.acquire() {
			T.mu.RUnlock()
			return cachedExec, nil
		}
	}
	T.mu.RUnlock()

	// 需要重新解析：打开文件
	osFile, openErr := os.OpenFile(filePath, os.O_RDONLY, 0)
	if openErr != nil {
		return nil, fmt.Errorf("vweb: Failed to open file! Error: %s", rel)
	}
	defer osFile.Close()

	return T.parseAndCache(pagePath, osFile, modeTime)
}

// parseAndCache 安全解析与替换缓存
func (T *ServerHandlerDynamic) parseAndCache(pagePath string, r io.Reader, modeTime time.Time) (*refCountedTemplater, error) {
	T.mu.Lock()
	defer T.mu.Unlock()

	// 双重检查防止并发重复解析
	if T.exec != nil && T.modeTime.Equal(modeTime) {
		if T.exec.acquire() {
			return T.exec, nil
		}
	}

	newExec, parseErr := T.parseTemplate(pagePath, r)
	if parseErr != nil {
		return nil, parseErr
	}

	wrapped := &refCountedTemplater{
		DynamicTemplater: newExec,
		refs:             1, // 1 表示当前被 T.exec 缓存所持有
	}

	if T.exec != nil {
		T.exec.Close()
	}
	T.exec = wrapped
	T.modeTime = modeTime

	// 增加当前执行请求的引用计数
	wrapped.acquire()
	return wrapped, nil
}

// parseTemplate 解析模板
func (T *ServerHandlerDynamic) parseTemplate(pagePath string, r io.Reader) (DynamicTemplater, error) {
	// 文件第一行，确认动态文件类型
	if T.Module == nil {
		return nil, errors.New("vweb: the file type of the first line of the file is not recognized")
	}

	buf := bufio.NewReaderSize(r, 64*1024) // 64KB 缓冲提升大模板解析性能
	dynmicType, err := fileFirstLine(buf)
	if err != nil {
		return nil, err
	}

	exec, err := T.Module(string(dynmicType))
	if err != nil {
		return nil, err
	}

	exec.SetPath(T.RootPath, pagePath)
	if err = exec.Parse(buf); err != nil {
		exec.Close()
		return nil, fmt.Errorf("vweb: Template parsing failed: %w", err)
	}

	return exec, nil
}

// prepareDot 准备执行上下文
func (T *ServerHandlerDynamic) prepareDot(rw http.ResponseWriter, req *http.Request) *Dot {
	dock := &Dot{
		R:    req,
		W:    rw,
		Site: T.Site,
	}

	// 从上下文中获取Site配置（如果存在）
	ctx := req.Context()
	if site, ok := ctx.Value(SiteContextKey).(*Site); ok && site != nil {
		dock.Site = site
	}
	dock.WithContext(ctx)

	return dock
}

func entryname(name string) string {
	if name == "" {
		return "Main"
	}

	// 使用path.Base而非filepath.Base以支持URL路径
	base := path.Base(name)
	if base == "." || base == "/" {
		return "Main"
	}

	// 去掉扩展名，使用 IndexByte 提升性能并减少开销
	if pos := strings.IndexByte(base, '.'); pos != -1 {
		base = base[:pos]
	}

	if base == "" || base == "index" {
		return "Main"
	}

	// 直接迭代底层字节校验合法标识符，比 UTF-8 rune 解码性能更高
	for i := 0; i < len(base); i++ {
		v := base[i]
		if !((v >= '0' && v <= '9') || (v >= 'A' && v <= 'Z') || (v >= 'a' && v <= 'z')) {
			return "Main"
		}
	}

	// 首字母大写
	if base[0] >= 'a' && base[0] <= 'z' {
		return string(base[0]-'a'+'A') + base[1:]
	}
	return base
}

func fileFirstLine(buf *bufio.Reader) (dynamicType []byte, err error) {
	h, peekErr := buf.Peek(8)
	if peekErr != nil && peekErr != io.EOF {
		return nil, fmt.Errorf("vweb: Failed to peek file header: %w", peekErr)
	}

	// 支持 UTF-8 BOM
	if len(h) >= 3 && bytes.Equal(h[:3], utf8BOM) {
		if _, err = buf.Discard(3); err != nil {
			return nil, fmt.Errorf("vweb: Failed to discard BOM: %w", err)
		}
		h, peekErr = buf.Peek(8)
		if peekErr != nil && peekErr != io.EOF {
			return nil, fmt.Errorf("vweb: Failed to peek file header after BOM: %w", peekErr)
		}
	}

	// WASM 文件：00 61 73 6D 01 00 00 00
	// 可能是wasm文件，直接返回wasm
	//| 检查项					| 值（十六进制） 	  | 值（ASCII/解释） | 说明
	//| 魔数 (Magic Number)		| `00 61 73 6D` 	| `\0 a s m` 	  | **唯一标识WASM二进制格式**
	//| 版本 (Version) 			| `01 00 00 00` 	| 小端序的数字 `1`  | 当前核心标准版本为1
	if len(h) >= 4 && bytes.Equal(h[:4], wasmMagic) {
		if len(h) >= 8 {
			// wasm-01000000
			return []byte(fmt.Sprintf("wasm-%0x", h[4:8])), nil
		}
		return []byte("wasm"), nil
	}

	// 读取第一行
	dynamicType, err = buf.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("vweb: Failed to read first line: %w", err)
	}

	if len(dynamicType) == 0 {
		return nil, errors.New("vweb: The file content is empty")
	}

	// 不是注释行退出
	trimmed := bytes.TrimSpace(dynamicType)
	if len(trimmed) < 2 || trimmed[0] != '/' || trimmed[1] != '/' {
		return nil, errors.New("vweb: The first line of the file needs to confirm the dynamic type")
	}

	// 提取类型信息
	comment := trimmed[2:]
	// 处理CRLF或LF
	comment = bytes.TrimRight(comment, "\r\n")
	comment = bytes.TrimSpace(comment)

	if len(comment) == 0 {
		return nil, errors.New("vweb: The first line of comments in the file is empty")
	}

	return comment, nil
}

// executeWith 内部执行
func (T *ServerHandlerDynamic) executeWith(exec DynamicTemplater, name string, bufw io.Writer, dock any) (stack []byte, err error) {
	if exec == nil {
		return nil, errors.New("vweb: Parse the template content first and then call the Execute")
	}
	defer func() {
		if e := recover(); e != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			stack = buf[:runtime.Stack(buf, false)]
			err = fmt.Errorf("vweb: Dynamic code execute error. %v", e)
		}
	}()

	return nil, exec.Execute(name, bufw, dock)
}

func (T *ServerHandlerDynamic) Close() error {
	T.mu.Lock()
	defer T.mu.Unlock()
	if T.exec != nil {
		err := T.exec.Close() // 递减引用计数，若无活动请求将在此触发真实的底层 Close()
		T.exec = nil
		T.modeTime = time.Time{}
		return err
	}
	return nil
}
