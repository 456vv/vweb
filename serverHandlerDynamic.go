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
	"sync/atomic"
	"time"
)

var (
	wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d}
	utf8BOM   = []byte{0xef, 0xbb, 0xbf}

	// bufferPool 缓存复用 bytes.Buffer，减少高并发下的 GC 压力
	bufferPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
)

// putBuffer 限制放回 pool 的 buffer 大小，防止个别特大页面占用过多常驻内存
func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() > 1024*1024 { // 超过 1MB 则丢弃
		return
	}
	buf.Reset()
	bufferPool.Put(buf)
}

// DynamicTemplater 动态模板接口规范
type DynamicTemplater interface {
	SetPath(root string, page string)
	Parse(r io.Reader) (err error)                        // 解析模板内容
	Execute(name string, out io.Writer, dot ...any) error // 执行模板并输出
	Close() error                                         // 释放相关资源
}

// DynamicTemplateFunc 声明动态模板获取函数类型
type DynamicTemplateFunc func(*ServerHandlerDynamic) DynamicTemplater

// refCountedTemplater 引用计数包装器，用以解决生命周期并发安全问题
type refCountedTemplater struct {
	DynamicTemplater
	mu        sync.Mutex
	refs      int
	destroyed bool
}

// Close 释放并关闭引用（减少缓存占用的1个计数，若归0则真实关闭）
func (r *refCountedTemplater) Close() error {
	r.mu.Lock()
	if r.destroyed {
		r.mu.Unlock()
		return nil
	}
	r.destroyed = true
	r.refs-- // 扣除缓存持有的引用
	shouldClose := r.refs <= 0
	r.mu.Unlock()

	if shouldClose {
		return r.DynamicTemplater.Close()
	}
	return nil
}

// acquire 增加一个并发操作引用
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

// release 释放当前操作的计数，归0且销毁时真实关闭
func (r *refCountedTemplater) release() {
	r.mu.Lock()
	r.refs--
	shouldClose := r.refs <= 0 && r.destroyed
	r.mu.Unlock()

	if shouldClose {
		r.DynamicTemplater.Close()
	}
}

// pageCacheEntry 缓存页面条目记录结构
type pageCacheEntry struct {
	exec     *refCountedTemplater
	modeTime time.Time
}

// webError 处理 web 请求时的统一错误响应
func webError(rw http.ResponseWriter, v ...any) {
	// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
	http.Error(rw, fmt.Sprint(v...), http.StatusInternalServerError)
}

// ServerHandlerDynamic 处理动态页面文件并作缓存管理
type ServerHandlerDynamic struct {
	// RootPath 网站根目录；PagePath 固定使用的页面路径（可选）
	RootPath, PagePath string

	// 可选的配置
	Site   *Site                                       // 网站配置
	Module func(name string) (DynamicTemplater, error) // 根据解析的首行识别动态文件类型对应的解释器

	// 读取文件。仅在 .ServeHTTP 方法中使用，可外接特殊存储介质
	ReadFile func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error)

	mu        sync.RWMutex // 保护 pageCache / pathLocks 的并发安全
	pageCache map[string]*pageCacheEntry
	pathLocks map[string]*sync.Mutex // 同页缓存填充串行锁，不同页互不阻塞
	closed    atomic.Bool
}

// getOrCreatePathLockLocked 获取同页路径的串行解析锁（必须在持有 T.mu 写锁时调用）
func (T *ServerHandlerDynamic) getOrCreatePathLockLocked(pagePath string) *sync.Mutex {
	if T.pathLocks == nil {
		T.pathLocks = make(map[string]*sync.Mutex)
	}
	if l := T.pathLocks[pagePath]; l != nil {
		return l
	}
	l := &sync.Mutex{}
	T.pathLocks[pagePath] = l
	return l
}

// ServeHTTP 服务 HTTP 请求
//
//	rw http.ResponseWriter    响应写入流
//	req *http.Request         当前客户端请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if T == nil {
		webError(rw, "vweb: ServerHandlerDynamic is nil")
		return
	}
	if T.closed.Load() {
		webError(rw, "vweb: ServerHandlerDynamic is closed")
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

// getOrCreateExecutor 获取或创建指定路径的模板执行器
func (T *ServerHandlerDynamic) getOrCreateExecutor(req *http.Request) (*refCountedTemplater, error) {
	// 确定页面路径（URL 路径风格）
	pagePath := T.PagePath
	if pagePath == "" {
		pagePath = req.URL.Path
	}
	pagePath = path.Clean(pagePath)
	// 规范化并去除可能的前导斜杠，后续用 filepath 拼接
	pagePath = strings.TrimPrefix(pagePath, "/")
	if pagePath == "" || pagePath == "." {
		pagePath = "index.go"
	}

	// 跨平台路径拼接
	filePath := filepath.Join(T.RootPath, filepath.FromSlash(pagePath))

	// 严格防止路径穿越：最终路径必须位于 RootPath 之下
	rootAbs, err := filepath.Abs(T.RootPath)
	if err != nil {
		return nil, fmt.Errorf("vweb: invalid RootPath")
	}
	fileAbs, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("vweb: invalid file path")
	}
	rel, err := filepath.Rel(rootAbs, fileAbs)
	if err != nil || !isLocalPath(rel) {
		return nil, errors.New("vweb: path traversal detected")
	}

	// ---------- 快速路径：先检查缓存是否仍然有效（不打开文件内容） ----------
	var modeTime time.Time
	var reader io.ReadCloser // only set when we must parse
	if T.ReadFile == nil {
		// 本地文件系统：仅 Stat，避免不必要的 Open
		fi, statErr := os.Stat(filePath)
		if statErr != nil {
			// 文件被删除或损坏时，自动清理陈旧缓存
			T.cleanupCacheEntry(pagePath)
			return nil, fmt.Errorf("vweb: Failed to stat file! Error: %s", rel)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("vweb: path is a directory: %s", rel)
		}
		modeTime = fi.ModTime()
	} else {
		// 自定义 ReadFile：必须调用一次才能拿到时间戳。
		reader, modeTime, err = T.ReadFile(filePath, req.URL)
		if err != nil {
			T.cleanupCacheEntry(pagePath)
			return nil, fmt.Errorf("vweb: Failed to read the ReadFile! Error: %s", rel)
		}
		defer reader.Close()
	}

	// 查缓存（读锁）
	T.mu.RLock()
	if T.pageCache != nil {
		if e, ok := T.pageCache[pagePath]; ok && e.modeTime.Equal(modeTime) {
			if e.exec.acquire() {
				T.mu.RUnlock()
				return e.exec, nil
			}
		}
	}
	T.mu.RUnlock()

	// 本地文件系统：延迟打开文件，并获取最新的 mtime，避免 TOCTOU 竞态
	if reader == nil {
		// 需要重新解析：打开文件
		reader, err = os.OpenFile(filePath, os.O_RDONLY, 0)
		if err != nil {
			return nil, fmt.Errorf("vweb: Failed to open file! Error: %s", rel)
		}
		defer reader.Close()
	}

	return T.parseAndCache(pagePath, reader, modeTime)
}

// cleanupCacheEntry 安全清理指定缓存条目（内部辅助，不导出）
func (T *ServerHandlerDynamic) cleanupCacheEntry(pagePath string) {
	T.mu.Lock()
	defer T.mu.Unlock()
	if T.pageCache != nil {
		if e, ok := T.pageCache[pagePath]; ok {
			e.exec.Close()
			delete(T.pageCache, pagePath)
		}
	}
	if T.pathLocks != nil {
		delete(T.pathLocks, pagePath)
	}
}

// parseAndCache 解析内容到执行器中并将其加入到安全缓存中
func (T *ServerHandlerDynamic) parseAndCache(pagePath string, r io.Reader, modeTime time.Time) (*refCountedTemplater, error) {
	// 同页串行解析，不同页之间不互相阻塞
	T.mu.Lock()
	if T.closed.Load() {
		T.mu.Unlock()
		return nil, errors.New("vweb: ServerHandlerDynamic is closed")
	}
	lock := T.getOrCreatePathLockLocked(pagePath)
	T.mu.Unlock()

	lock.Lock()
	defer lock.Unlock()

	// 获取到同页锁后再次检查，避免并发排队时重复解析
	T.mu.RLock()
	closed := T.closed.Load()
	if !closed && T.pageCache != nil {
		if e, ok := T.pageCache[pagePath]; ok && e.modeTime.Equal(modeTime) {
			if e.exec.acquire() {
				T.mu.RUnlock()
				return e.exec, nil
			}
		}
	}
	T.mu.RUnlock()
	if closed {
		return nil, errors.New("vweb: ServerHandlerDynamic is closed")
	}

	newExec, parseErr := T.parseTemplate(pagePath, r)
	if parseErr != nil {
		return nil, parseErr
	}

	T.mu.Lock()
	defer T.mu.Unlock()

	if T.closed.Load() {
		newExec.Close()
		return nil, errors.New("vweb: ServerHandlerDynamic is closed")
	}

	wrapped := &refCountedTemplater{
		DynamicTemplater: newExec,
		refs:             1, // 1 表示当前被 pageCache 缓存所持有
	}

	// 准备存入多页缓存
	if T.pageCache == nil {
		T.pageCache = make(map[string]*pageCacheEntry)
	}
	if old, ok := T.pageCache[pagePath]; ok {
		old.exec.Close()
	}
	T.pageCache[pagePath] = &pageCacheEntry{
		exec:     wrapped,
		modeTime: modeTime,
	}

	// 增加当前执行请求的引用计数返回
	wrapped.acquire()
	return wrapped, nil
}

// parseTemplate 解析模板内容并绑定首行确定的执行器
func (T *ServerHandlerDynamic) parseTemplate(pagePath string, r io.Reader) (DynamicTemplater, error) {
	// 文件第一行，确认动态文件类型
	if T.Module == nil {
		return nil, errors.New("vweb: Module is nil; cannot determine dynamic file type")
	}

	buf := bufio.NewReaderSize(r, 64*1024) // 64KB 缓冲提升大模板解析性能
	dynamicType, err := fileFirstLine(buf)
	if err != nil {
		return nil, err
	}

	exec, err := T.Module(string(dynamicType))
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

// prepareDot 准备模板渲染使用的顶级上下文变量 Dot
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

// entryname 从路径中提取首字母大写的执行主入口函数名
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

// fileFirstLine 读取并判定首行格式作为解析引擎标志（去除 BOM 及处理 WASM 字节码特例）
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

	// 必须为注释行作为标志
	trimmed := bytes.TrimSpace(dynamicType)
	if len(trimmed) < 2 || trimmed[0] != '/' || trimmed[1] != '/' {
		return nil, errors.New("vweb: The first line of the file needs to confirm the dynamic type")
	}

	// 提取类型信息
	comment := trimmed[2:]
	// 处理 CRLF 或 LF
	comment = bytes.TrimRight(comment, "\r\n")
	comment = bytes.TrimSpace(comment)

	if len(comment) == 0 {
		return nil, errors.New("vweb: The first line of comments in the file is empty")
	}

	return comment, nil
}

// executeWith 安全模式内部执行动态文件代码
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

// Close 关闭整个处理器缓存释放相关持有资源
func (T *ServerHandlerDynamic) Close() error {
	T.mu.Lock()
	if T.closed.Swap(true) {
		T.mu.Unlock()
		return nil
	}
	entries := T.pageCache
	T.pageCache = nil
	T.pathLocks = nil
	T.mu.Unlock()

	var firstErr error
	for _, e := range entries {
		if err := e.exec.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// isLocalPath 判断相对路径是否安全（无越界）
// 兼容 Windows 与 Unix，等价于 Go 1.20+ 的 filepath.IsLocal 逻辑
func isLocalPath1(rel string) bool {
	if rel == "" || rel == "." {
		return true
	}
	// 拒绝绝对路径、盘符路径或 UNC 路径
	if filepath.IsAbs(rel) {
		return false
	}
	if filepath.VolumeName(rel) != "" {
		return false
	}
	if strings.HasPrefix(rel, `\\`) || strings.HasPrefix(rel, "//") {
		return false
	}

	// 检查中间是否存在 ".." 组件（同时兼容 / 与系统分隔符）
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if part == ".." {
			return false
		}
	}
	return true
}
