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

// refCountedTemplater 引用计数包装器，用以解决生命周期并发安全问题。
//
// 不变式（在 r.mu 保护下）：
//   - refs  := 当前持有者数量（缓存 1 + 每个正在使用/即将使用它的 goroutine）
//   - destroyed := 是否已从缓存移除（Close 首次调用时置 true，且只减一次 refs）
//   - closed := 底层 DynamicTemplater 是否已 Close（保证底层 Close 至多执行一次）
//
// 典型生命周期（parseAndCache 返回时 refs=2）：
//   - 缓存持有 1，调用方持有 1
//   - 调用方 release → refs=1
//   - 缓存移除（Close）→ destroyed=true，refs=0 → 底层 Close
//
// 或：
//   - 缓存移除（Close）→ destroyed=true，refs=1（调用方仍持有）
//   - 调用方 release → refs=0 → 底层 Close
type refCountedTemplater struct {
	DynamicTemplater
	mu        sync.Mutex
	refs      int
	destroyed bool
	// 增加 closed 标志保证底层 Close 最多执行一次。
	closed bool
}

// Close 从缓存移除并释放缓存持有的那 1 个引用。
// 幂等：destroyed 后再次调用直接返回，不会重复 refs--。
func (r *refCountedTemplater) Close() error {
	r.mu.Lock()
	if r.destroyed {
		r.mu.Unlock()
		return nil
	}
	r.destroyed = true
	r.refs--
	shouldClose := r.refs <= 0 && !r.closed
	if shouldClose {
		r.closed = true
	}
	r.mu.Unlock()

	if shouldClose {
		return r.DynamicTemplater.Close()
	}
	return nil
}

// acquire 增加一个并发操作引用。已从缓存移除（destroyed）后返回 false。
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

// release 释放当前操作的计数，归 0 且已 destroyed 时真实关闭。
func (r *refCountedTemplater) release() {
	r.mu.Lock()
	if r.closed {
		// 底层已关闭，不应再改动计数，避免 refs 变成负数。
		r.mu.Unlock()
		return
	}
	r.refs--
	shouldClose := r.refs <= 0 && r.destroyed && !r.closed
	if shouldClose {
		r.closed = true
	}
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

// refLock 引用计数锁。
//
// 语义：refs > 0 表示有 goroutine 正在持有或即将持有该锁。
// 此期间 cleanupCacheEntry 不会从 pathLocks 中删除它，
// 保证"同一 pagePath 始终只有一把串行锁"。
//
// refs 的增减都在 ServerHandlerDynamic.mu 写锁下完成；
// 使用 atomic.Int32 只为让 cleanupCacheEntry 只读检测时无需额外加锁。
type refLock struct {
	mu   sync.Mutex
	refs atomic.Int32
}

// ServerHandlerDynamic 处理动态页面文件并作缓存管理。
type ServerHandlerDynamic struct {
	// RootPath 网站根目录；PagePath 固定使用的页面路径（可选）
	RootPath, PagePath string

	// 可选的配置
	Site   *Site                                       // 网站配置
	Module func(name string) (DynamicTemplater, error) // 根据解析的首行识别动态文件类型对应的解释器

	// 读取文件。仅在 .ServeHTTP 方法中使用，可外接特殊存储介质。
	//   - 非 nil 时：RootPath 视为逻辑前缀，filePath = path.Join(RootPath, pagePath)；
	//     不访问本地文件系统，不做符号链接校验；
	//     仍对请求路径做字符串级 ".." 穿越检查，存储介质内的安全由 ReadFile 实现负责。
	//     返回值中的 reader 必须非 nil（若只需探测 mtime 请仍返回一个可 Close 的空 reader，
	//     或直接返回 io.NopCloser(bytes.NewReader(nil))）。
	//   - 为 nil 时走本地文件系统（os.Stat / os.OpenFile + 符号链接逃逸校验）。
	ReadFile func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error)

	mu        sync.RWMutex // 保护 pageCache / pathLocks / closed 的并发安全
	pageCache map[string]*pageCacheEntry
	pathLocks map[string]*refLock
	closed    atomic.Bool
}

// acquirePathLock 获取 pagePath 的串行解析锁并增加一个引用。
// ok=false 表示 handler 已关闭。
func (T *ServerHandlerDynamic) acquirePathLock(pagePath string) (l *refLock, ok bool) {
	T.mu.Lock()
	defer T.mu.Unlock()
	if T.closed.Load() {
		return nil, false
	}
	if T.pathLocks == nil {
		T.pathLocks = make(map[string]*refLock)
	}
	l = T.pathLocks[pagePath]
	if l == nil {
		l = &refLock{}
		T.pathLocks[pagePath] = l
	}
	l.refs.Add(1)
	return l, true
}

// releasePathLock 释放一个引用；引用归零时从 map 中移除锁对象。
// 若 handler 已被 Close（pathLocks 为 nil）或锁已被替换，静默返回。
//
// 调用约定：调用方应持有 lock.mu（在 parseAndCache 的 defer 中先 release 再 unlock），
// 保证 refs 观测到的锁对象与 map 中一致，不会出现"同页两把锁"。
func (T *ServerHandlerDynamic) releasePathLock(pagePath string, l *refLock) {
	T.mu.Lock()
	defer T.mu.Unlock()
	if T.pathLocks == nil || T.pathLocks[pagePath] != l {
		return
	}
	if l.refs.Add(-1) <= 0 {
		delete(T.pathLocks, pagePath)
	}
}

// ServeHTTP 服务 HTTP 请求
//
//	rw http.ResponseWriter    响应写入流
//	req *http.Request         当前客户端请求
//
// ServeHTTP 服务 HTTP 请求。
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if T == nil {
		webError(rw, "vweb: ServerHandlerDynamic is nil")
		return
	}
	if T.closed.Load() {
		webError(rw, "vweb: ServerHandlerDynamic is closed")
		return
	}

	tplExec, err := T.getOrCreateExecutor(req)
	if err != nil {
		// 与静态处理器共用哨兵错误，按类型映射状态码。
		switch {
		case errors.Is(err, errInvalidRootPath):
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		case errors.Is(err, errFileNotFound):
			http.Error(rw, err.Error(), http.StatusNotFound)
		case errors.Is(err, errPermissionDenied):
			http.Error(rw, "Forbidden", http.StatusForbidden)
		case errors.Is(err, errSymlinkEscape):
			http.Error(rw, "Forbidden", http.StatusForbidden)
		case errors.Is(err, errPathTraversal), errors.Is(err, errInvalidFilePath):
			http.Error(rw, err.Error(), http.StatusNotFound)
		default:
			webError(rw, err.Error())
		}
		return
	}

	defer tplExec.release()

	dock := T.prepareDot(rw, req)
	defer dock.Close()

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
		body.WriteTo(rw)
	}
}

// getOrCreateExecutor 获取或创建指定路径的模板执行器。
func (T *ServerHandlerDynamic) getOrCreateExecutor(req *http.Request) (*refCountedTemplater, error) {
	// 入口二次检查，避免 ServeHTTP 与 Close 之间的窗口。
	if T.closed.Load() {
		return nil, errors.New("vweb: ServerHandlerDynamic is closed")
	}

	// 按存储介质分叉解析：
	//   ReadFile != nil → 虚拟路径，不访问本地文件系统；
	//   ReadFile == nil → 本地文件系统，完整解析 + 符号链接逃逸校验。
	var (
		pagePath, fileAbs, rel string
		err                    error
	)
	if T.ReadFile != nil {
		pagePath, fileAbs, rel, err = resolveVirtualPath(T.RootPath, T.PagePath, req.URL.Path, "index.go")
	} else {
		pagePath, fileAbs, rel, err = resolvePagePath(T.RootPath, T.PagePath, req.URL.Path, "index.go")
	}
	if err != nil {
		return nil, err
	}

	// ---------- 快速路径：先检查缓存是否仍然有效（不打开文件内容） ----------
	var (
		modeTime time.Time
		reader   io.ReadCloser
	)
	if T.ReadFile == nil {
		fi, statErr := os.Stat(fileAbs)
		if statErr != nil {
			T.cleanupCacheEntry(pagePath)
			return nil, classifyFileError(statErr, rel)
		}
		if fi.IsDir() {
			return nil, fmt.Errorf("vweb: path is a directory: %s", rel)
		}
		modeTime = fi.ModTime()
	} else {
		reader, modeTime, err = T.ReadFile(fileAbs, req.URL)
		if err != nil {
			T.cleanupCacheEntry(pagePath)
			return nil, fmt.Errorf("vweb: Failed to read the ReadFile! Error: %s", rel)
		}
		// ReadFile 契约要求 reader 非 nil，否则后续逻辑会误走本地文件系统分支。
		if reader == nil {
			return nil, fmt.Errorf("vweb: ReadFile returned nil reader without error for %s", rel)
		}
		defer reader.Close()
	}

	// 零值 modeTime 视为无法判断缓存有效，走重建。
	// 缓存命中但已 destroyed（acquire 失败）时清理无效条目。
	T.mu.RLock()
	if T.pageCache != nil && !modeTime.IsZero() {
		if e, ok := T.pageCache[pagePath]; ok && e.modeTime.Equal(modeTime) {
			if e.exec.acquire() {
				T.mu.RUnlock()
				return e.exec, nil
			}
		}
	}
	T.mu.RUnlock()

	// 本地文件系统：延迟打开文件，并在打开后重新取 mtime，
	// 避免 Stat→Open 之间文件被替换。
	if reader == nil {
		f, err := os.OpenFile(fileAbs, os.O_RDONLY, 0)
		if err != nil {
			T.cleanupCacheEntry(pagePath)
			return nil, classifyFileError(err, rel)
		}
		if fi, statErr := f.Stat(); statErr == nil {
			modeTime = fi.ModTime()
		}
		reader = f
		defer reader.Close()
	}

	return T.parseAndCache(pagePath, reader, modeTime)
}

// cleanupCacheEntry 安全清理指定缓存条目。
//
// Close 会进入外部 DynamicTemplater.Close()，可能与用户代码争用 T.mu。
// 因此在 T.mu 内只做摘链，T.mu 释放后再执行底层 Close。
func (T *ServerHandlerDynamic) cleanupCacheEntry(pagePath string) {
	var toClose *refCountedTemplater
	T.mu.Lock()
	if T.pageCache != nil {
		if e, ok := T.pageCache[pagePath]; ok {
			toClose = e.exec
			delete(T.pageCache, pagePath)
		}
	}
	// 仅当没有 goroutine 正在持有/等待该页的串行锁时才移除锁对象；
	// 否则会导致"同页出现两把不同锁"的并发解析。
	if T.pathLocks != nil {
		if l, ok := T.pathLocks[pagePath]; ok && l.refs.Load() <= 0 {
			delete(T.pathLocks, pagePath)
		}
	}
	T.mu.Unlock()

	if toClose != nil {
		toClose.Close()
	}
}

// parseAndCache 解析内容到执行器中并将其加入到安全缓存中。
func (T *ServerHandlerDynamic) parseAndCache(pagePath string, r io.Reader, modeTime time.Time) (*refCountedTemplater, error) {
	lock, ok := T.acquirePathLock(pagePath)
	if !ok {
		return nil, errors.New("vweb: ServerHandlerDynamic is closed")
	}
	lock.mu.Lock()
	defer func() {
		// 顺序：先 release 计数（仍在 lock.mu 保护下），再 unlock。
		// 这样 cleanupCacheEntry 观察 refs 时，锁对象仍在 map 中，
		// 不会出现"同页两把锁"的竞态窗口。
		T.releasePathLock(pagePath, lock)
		lock.mu.Unlock()
	}()

	// 获取到同页锁后再次检查，避免并发排队时重复解析。
	T.mu.RLock()
	closed := T.closed.Load()
	if !closed && T.pageCache != nil && !modeTime.IsZero() {
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

	// refs 初始化为 2（缓存 1 + 调用方 1），
	// 不再在 T.mu 外调用 acquire()，消除"publish 后被 Close 抢先"的窗口。
	// 所有底层 Close 都移到 T.mu 释放之后。
	wrapped := &refCountedTemplater{
		DynamicTemplater: newExec,
		refs:             2,
	}

	T.mu.Lock()
	if T.closed.Load() {
		T.mu.Unlock()
		newExec.Close() // T.mu 外关闭外部实现
		return nil, errors.New("vweb: ServerHandlerDynamic is closed")
	}
	if T.pageCache == nil {
		T.pageCache = make(map[string]*pageCacheEntry)
	}
	var oldExec *refCountedTemplater
	if old, ok := T.pageCache[pagePath]; ok {
		oldExec = old.exec
	}
	T.pageCache[pagePath] = &pageCacheEntry{
		exec:     wrapped,
		modeTime: modeTime,
	}
	T.mu.Unlock()

	if oldExec != nil {
		oldExec.Close() // T.mu 外关闭外部实现
	}
	return wrapped, nil
}

// parseTemplate 解析模板内容并绑定首行确定的执行器。
func (T *ServerHandlerDynamic) parseTemplate(pagePath string, r io.Reader) (DynamicTemplater, error) {
	if T.Module == nil {
		return nil, errors.New("vweb: Module is nil; cannot determine dynamic file type")
	}

	buf := bufio.NewReaderSize(r, 64*1024)
	dynamicType, err := fileFirstLine(buf)
	if err != nil {
		return nil, err
	}

	exec, err := T.Module(string(dynamicType))
	if err != nil {
		return nil, err
	}
	// Module 返回 (nil, nil) 时避免 SetPath 空指针 panic。
	if exec == nil {
		return nil, errors.New("vweb: Module returned nil templater without error")
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

// fileFirstLine 读取并判定首行格式作为解析引擎标志（去除 BOM 及处理 WASM 字节码特例）。
func fileFirstLine(buf *bufio.Reader) (dynamicType []byte, err error) {
	h, peekErr := buf.Peek(8)
	if peekErr != nil && peekErr != io.EOF && peekErr != bufio.ErrBufferFull {
		return nil, fmt.Errorf("vweb: Failed to peek file header: %w", peekErr)
	}

	// 支持 UTF-8 BOM
	if len(h) >= 3 && bytes.Equal(h[:3], utf8BOM) {
		if _, err = buf.Discard(3); err != nil {
			return nil, fmt.Errorf("vweb: Failed to discard BOM: %w", err)
		}
		h, peekErr = buf.Peek(8)
		if peekErr != nil && peekErr != io.EOF && peekErr != bufio.ErrBufferFull {
			return nil, fmt.Errorf("vweb: Failed to peek file header after BOM: %w", peekErr)
		}
	}

	// WASM 文件：00 61 73 6D 01 00 00 00
	// h 长度不足 4 字节时不做切片比较，避免越界 panic。
	if len(h) >= 4 && bytes.Equal(h[:4], wasmMagic) {
		if len(h) >= 8 {
			return fmt.Appendf(nil, "wasm-%0x", h[4:8]), nil
		}
		return []byte("wasm"), nil
	}

	dynamicType, err = buf.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("vweb: Failed to read first line: %w", err)
	}

	if len(dynamicType) == 0 {
		return nil, errors.New("vweb: The file content is empty")
	}

	trimmed := bytes.TrimSpace(dynamicType)
	if len(trimmed) < 2 || trimmed[0] != '/' || trimmed[1] != '/' {
		return nil, errors.New("vweb: The first line of the file needs to confirm the dynamic type")
	}

	comment := trimmed[2:]
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

// Close 关闭整个处理器缓存释放相关持有资源。
func (T *ServerHandlerDynamic) Close() error {
	T.mu.Lock()
	if T.closed.Swap(true) {
		T.mu.Unlock()
		return nil
	}
	entries := T.pageCache
	T.pageCache = nil
	// 正在持有 pathLock 的 goroutine 已通过指针引用锁对象；
	// 清空 map 不影响它们；releasePathLock 检测到 map 变更后直接返回，
	// 锁对象在持有者释放后由 GC 回收。
	T.pathLocks = nil
	T.mu.Unlock()

	var firstErr error
	for _, e := range entries {
		if e == nil || e.exec == nil {
			continue
		}
		if err := e.exec.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
