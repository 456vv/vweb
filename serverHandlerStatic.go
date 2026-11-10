package vweb

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxRangeCount 单个请求允许的 Range 段数上限。
// "bytes=0-0,2-2,4-4,..." 让每个 1 字节区间都输出一段 multipart 头，
// 响应远大于文件本身，可被用于放大攻击。超限时忽略 Range，返回全量。
const maxRangeCount = 100

// blockRange 定义了 HTTP Range 请求中的字节范围信息
type blockRange struct {
	start, length int64
}

// serverHandlerStaticHeader 负责辅助设置 HTTP 响应头，简化 Header 写入逻辑
type serverHandlerStaticHeader struct {
	fileInfo os.FileInfo
	header   http.Header
	blocks   []blockRange
}

// setETag 设置 ETag 响应头，用于浏览器校验缓存有效性
func (T *serverHandlerStaticHeader) setETag() {
	T.header.Set("ETag", T.etag())
}

// etag 基于文件修改时间和大小生成弱校验符
// 返回: 示例 W/"65a1234-1024"
func (T *serverHandlerStaticHeader) etag() string {
	return fmt.Sprintf(`W/"%x-%x"`, T.fileInfo.ModTime().UnixNano(), T.fileInfo.Size())
}

// ranges 解析 HTTP 请求头中的 "Range" 字段，计算文件的读取位置和长度。
// 与原生实现相比，这里会对结果排序并合并相邻或重叠区间，避免多个重叠 Range
// 的总和大于文件大小时被误判为非法 Range，同时减少不必要的分段读取。
func (T *serverHandlerStaticHeader) ranges(ranges string) (r []blockRange, n int64) {
	size := T.fileInfo.Size()
	if size <= 0 {
		return nil, 0
	}
	if !strings.HasPrefix(ranges, "bytes=") {
		return nil, 0
	}

	var raw []blockRange

	// 去掉 "bytes=" 前缀后按逗号分割，忽略空段
	for _, v := range strings.Split(ranges[6:], ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		rv := strings.SplitN(v, "-", 2)
		if len(rv) != 2 {
			continue
		}
		var start, end int64

		// 处理后缀范围: "-500" 表示最后 500 字节
		if rv[0] == "" {
			if rv[1] == "" {
				continue
			}
			val, e := strconv.ParseInt(rv[1], 10, 64)
			if e != nil || val <= 0 {
				continue
			}
			start = max(size-val, 0)
			end = size - 1
		} else {
			// 处理常规范围: "0-499" 或 "500-"
			var e error
			start, e = strconv.ParseInt(rv[0], 10, 64)
			if e != nil || start < 0 {
				continue
			}

			if rv[1] == "" {
				// "500-" 表示到文件末尾
				end = size - 1
			} else {
				end, e = strconv.ParseInt(rv[1], 10, 64)
				if e != nil {
					continue
				}
			}
		}

		// 越界与合法性保护（符合 RFC 7233）
		if end >= size {
			end = size - 1
		}
		if start >= size || start > end {
			continue
		}

		raw = append(raw, blockRange{start: start, length: end - start + 1})
	}

	if len(raw) == 0 {
		return nil, 0
	}

	// 按起始位置排序，便于合并相邻或重叠区间
	sort.Slice(raw, func(i, j int) bool { return raw[i].start < raw[j].start })

	r = append(r, raw[0])
	for _, b := range raw[1:] {
		last := &r[len(r)-1]
		lastEnd := last.start + last.length

		// 重叠或相邻则合并
		if b.start <= lastEnd {
			bEnd := b.start + b.length
			if bEnd > lastEnd {
				last.length = bEnd - last.start
			}
		} else {
			r = append(r, b)
		}
	}

	for _, b := range r {
		n += b.length
	}
	return r, n
}

// 设置基础 HTTP 响应头
func (T *serverHandlerStaticHeader) setLastModified() {
	T.header.Set("Last-Modified", T.fileInfo.ModTime().UTC().Format(http.TimeFormat))
}

func (T *serverHandlerStaticHeader) setDate() {
	T.header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
}

func (T *serverHandlerStaticHeader) setContentLength(l int64) {
	T.header.Set("Content-Length", strconv.FormatInt(l, 10))
}

func (T *serverHandlerStaticHeader) setAcceptRanges() {
	T.header.Set("Accept-Ranges", "bytes")
}

func (T *serverHandlerStaticHeader) setContentType(filePath string) {
	// 站点 MIME 或上游可能已设置 Content-Type，不能再用扩展名默认值覆盖。
	if T.header.Get("Content-Type") != "" {
		return
	}
	// 自动识别 MIME 类型，防止浏览器直接下载
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath)))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	T.header.Set("Content-Type", ctype)
}

// setPageExpired 设置缓存过期策略 (Cache-Control & Expires)
func (T *serverHandlerStaticHeader) setPageExpired(pageExpired int64) {
	if pageExpired > 0 {
		// pageExpired * time.Second 在极大值下会发生溢出。
		maxSeconds := int64(time.Duration(1<<63-1) / time.Second)
		if pageExpired > maxSeconds {
			pageExpired = maxSeconds
		}
		T.header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", pageExpired))
		T.header.Set("Expires", time.Now().Add(time.Duration(pageExpired)*time.Second).UTC().Format(http.TimeFormat))
		return
	}
	// PageExpired=0 表示未指定过期时间。站点 Header.Static 已写入 Cache-Control 时，
	// 不能用 no-cache 覆盖它。
	if T.header.Get("Cache-Control") != "" {
		return
	}
	T.header.Set("Cache-Control", "no-cache")
}

func (T *serverHandlerStaticHeader) setContentRange(r string) {
	T.header.Set("Content-Range", r)
}

// ServerHandlerStatic 静态文件服务配置
type ServerHandlerStatic struct {
	RootPath, PagePath string
	PageExpired        int64 // 缓存过期时长（单位：秒），0则不缓存
}

// 可选缓冲区池，降低高并发下的分配压力
var copyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

// ServeHTTP 实现 http.Handler 接口，处理文件读取与响应。
//
// 两阶段设计：
//  1. os.Stat + 基础头 + 缓存协商：命中 304 直接返回，不 Open 文件；
//  2. 未命中才 Open，并用 fstat 与第一阶段 Stat 对比；若文件在 Stat 与 Open
//     之间被替换（mtime/size 变化），用新 fileInfo 重做缓存协商与 Range 解析，
//     保证 Content-Length、ETag、Range 与正文读的是同一个文件版本（S1）。
func (T *ServerHandlerStatic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// 仅允许安全方法
	switch req.Method {
	case http.MethodGet, http.MethodHead:
		// ok
	default:
		rw.Header().Set("Allow", "GET, HEAD")
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	_, fileAbs, _, err := resolvePagePath(T.RootPath, T.PagePath, req.URL.Path, "index.html")
	if err != nil {
		switch {
		case errors.Is(err, errInvalidRootPath):
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		case errors.Is(err, errSymlinkFailure):
			http.Error(rw, "Forbidden", http.StatusForbidden) // ← root 不存在也走这里
		default:
			http.Error(rw, err.Error(), http.StatusNotFound)
		}
		return
	}

	// ---------- 阶段 A：os.Stat + 缓存协商（不 Open） ----------
	statInfo, err := os.Stat(fileAbs)
	if err != nil {
		http.Error(rw, "File not found", http.StatusNotFound)
		return
	}
	if statInfo.IsDir() {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}
	// 仅提供普通文件，避免设备文件、命名管道等非普通文件造成阻塞或安全问题
	if !statInfo.Mode().IsRegular() {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}

	sh := T.newHeader(rw, statInfo, fileAbs)
	if T.notModified(rw, req, sh) {
		return // 304，不 Open
	}

	// ---------- 阶段 B：Open + fstat，响应头与正文读同一句柄 ----------
	file, err := os.Open(fileAbs)
	if err != nil {
		switch {
		case os.IsNotExist(err):
			http.Error(rw, "File not found", http.StatusNotFound)
		case os.IsPermission(err):
			http.Error(rw, "Forbidden", http.StatusForbidden)
		default:
			http.Error(rw, "Failed to open file", http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil || !fileInfo.Mode().IsRegular() {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}
	if !sameFile(statInfo, fileInfo) {
		// Stat 与 Open 之间文件被替换：重建头并重做缓存协商。
		sh = T.newHeader(rw, fileInfo, fileAbs)
		if T.notModified(rw, req, sh) {
			return
		}
	}

	blocks, ok := T.serveRange(rw, req, sh)
	if !ok {
		return
	}
	T.body(rw, blocks, sh.fileInfo, file, fileAbs)
}

// newHeader 创建 sh 并设置基础响应头（不含 Content-Length）。
func (T *ServerHandlerStatic) newHeader(rw http.ResponseWriter, fi os.FileInfo, filePath string) *serverHandlerStaticHeader {
	sh := &serverHandlerStaticHeader{
		fileInfo: fi,
		header:   rw.Header(),
	}
	sh.setContentType(filePath)
	sh.setDate()
	sh.setLastModified()
	sh.setETag()
	sh.setAcceptRanges()
	sh.setPageExpired(T.PageExpired)
	return sh
}

// notModified 做缓存协商；命中写 304 并返回 true。
func (T *ServerHandlerStatic) notModified(rw http.ResponseWriter, req *http.Request, sh *serverHandlerStaticHeader) bool {
	etag := sh.etag()
	modTime := sh.fileInfo.ModTime()

	// 优先 ETag（If-None-Match）——弱比较
	if inm := req.Header.Get("If-None-Match"); inm != "" {
		if etagMatch(inm, etag) {
			rw.Header().Del("Content-Type")
			rw.Header().Del("Content-Length")
			rw.WriteHeader(http.StatusNotModified)
			return true
		}
		return false
	}
	if ims := req.Header.Get("If-Modified-Since"); ims != "" {
		if t, err := http.ParseTime(ims); err == nil {
			// 秒级截断比较，与 RFC 7232 一致。
			if !modTime.Truncate(time.Second).After(t) {
				rw.Header().Del("Content-Type")
				rw.Header().Del("Content-Length")
				rw.WriteHeader(http.StatusNotModified)
				return true
			}
		}
	}
	return false
}

// serveRange 处理空文件、Range、If-Range、HEAD 与 416。
// 返回待发送区间；ok=false 表示响应已结束（304 已在 notModified 处理）。
func (T *ServerHandlerStatic) serveRange(rw http.ResponseWriter, req *http.Request, sh *serverHandlerStaticHeader) ([]blockRange, bool) {
	size := sh.fileInfo.Size()
	// 空文件在缓存协商完成后直接返回，避免丢失 304 行为
	if size <= 0 {
		sh.setContentLength(0)
		rw.WriteHeader(http.StatusOK)
		return nil, false
	}

	rangeHeader := req.Header.Get("Range")
	// RFC 9110 规定不认识的 Range 单位应忽略；非法 bytes 区间仍由 ranges 返回 416。
	if rangeHeader != "" && !strings.HasPrefix(rangeHeader, "bytes=") {
		rangeHeader = ""
	}
	// 段数 = 逗号数 + 1。逗号数 >= maxRangeCount ⇒ 段数 > maxRangeCount；
	// 超限时忽略 Range 返回全量，防止 "bytes=0-0,2-2,..." 多段放大响应。
	if strings.Count(rangeHeader, ",") >= maxRangeCount {
		rangeHeader = ""
	}

	// 若同时存在 If-Range，需校验后才应用 Range（RFC 7233）
	if ir := req.Header.Get("If-Range"); ir != "" && rangeHeader != "" {
		if !T.ifRangeMatch(ir, sh) {
			rangeHeader = ""
		}
	}

	// 无 Range：全量响应
	if rangeHeader == "" {
		sh.setContentLength(size)
		if req.Method == http.MethodHead {
			rw.WriteHeader(http.StatusOK)
			return nil, false
		}
		return nil, true
	}

	blocks, _ := sh.ranges(rangeHeader)
	if len(blocks) == 0 {
		sh.setContentRange(fmt.Sprintf("bytes */%d", size))
		sh.setContentLength(0)
		rw.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil, false
	}

	// HEAD 请求仅返回头
	if req.Method == http.MethodHead {
		if len(blocks) == 1 {
			b := blocks[0]
			sh.setContentRange(fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, size))
			sh.setContentLength(b.length)
			rw.WriteHeader(http.StatusPartialContent)
		} else {
			// 多 Range 的 HEAD：设置 multipart 类型，不写 body
			mw := multipart.NewWriter(io.Discard)
			boundary := mw.Boundary()
			rw.Header().Set("Content-Type", "multipart/byteranges; boundary="+boundary)
			rw.Header().Del("Content-Length")
			rw.WriteHeader(http.StatusPartialContent)
		}
		return nil, false
	}

	return blocks, true
}

// etagStrongMatch 实现强实体标签比较（RFC 7232 §2.3.2）：
// 两个都是强 ETag（以 " 开头，无 W/ 前缀）且字面相等。
func etagStrongMatch(a, b string) bool {
	return a != "" && b != "" && a == b && a[0] == '"'
}

// ifRangeMatch 校验 If-Range。弱 ETag 永不匹配；时间按秒级比较（强校验）。
func (T *ServerHandlerStatic) ifRangeMatch(ir string, sh *serverHandlerStaticHeader) bool {
	// 弱 ETag 永不匹配（RFC 7233 §3.2）。此时必须忽略 Range，返回 200 全量。
	if strings.HasPrefix(ir, "W/") {
		return false
	}
	// 强 ETag：两端都必须是强 ETag 且字面相等。
	if strings.HasPrefix(ir, `"`) {
		return etagStrongMatch(ir, sh.etag())
	}
	if t, err := http.ParseTime(ir); err == nil {
		return sh.fileInfo.ModTime().Unix() == t.Unix()
	}
	return false
}

// sameFile 判断两次 Stat 是否为同一版本（大小与 mtime 一致）。
// 用于检测 Stat 与 Open 之间文件是否被替换或原地改写。
func sameFile(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// body 将文件内容写入 HTTP 响应体，支持分段传输 (Range)。
// 接收 ServeHTTP 已打开的文件，不再二次 Open（S1）。
func (T *ServerHandlerStatic) body(rw http.ResponseWriter, blocks []blockRange, fi os.FileInfo, file *os.File, filePath string) {
	size := fi.Size()
	bufp := copyBufPool.Get().(*[]byte)
	buf := *bufp
	defer copyBufPool.Put(bufp)

	// copyAndLog 复制并记录非客户端原因的错误
	copyAndLog := func(dst io.Writer, src io.Reader) {
		if _, err := io.CopyBuffer(dst, src, buf); err != nil && !isClientGone(err) {
			log.Printf("vweb: static file copy error %q: %v", filePath, err)
		}
	}

	// 1. 无 Range 参数：全量传输
	if len(blocks) == 0 {
		rw.WriteHeader(http.StatusOK)
		copyAndLog(rw, file)
		return
	}

	// 2. 单 Range 请求：分段读取部分内容
	if len(blocks) == 1 {
		b := blocks[0]
		// 防御性检查，避免 length<=0 时 Content-Range 非法
		if b.length <= 0 {
			http.Error(rw, "Invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, size))
		rw.Header().Set("Content-Length", strconv.FormatInt(b.length, 10))
		rw.WriteHeader(http.StatusPartialContent)

		if _, err := file.Seek(b.start, io.SeekStart); err != nil {
			log.Printf("vweb: static file seek error %q: %v", filePath, err)
			return
		}
		copyAndLog(rw, io.LimitReader(file, b.length))
		return
	}

	// 3. 多 Range 请求：使用 multipart/byteranges 协议传输
	ctype := rw.Header().Get("Content-Type")
	mw := multipart.NewWriter(rw)
	boundary := mw.Boundary()
	rw.Header().Set("Content-Type", "multipart/byteranges; boundary="+boundary)
	// 多 Range 时不强制设置 Content-Length（由传输编码决定）
	rw.Header().Del("Content-Length")
	rw.WriteHeader(http.StatusPartialContent)
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	for _, b := range blocks {
		partH := make(textproto.MIMEHeader)
		partH.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, size))
		partH.Set("Content-Type", ctype)

		w, err := mw.CreatePart(partH)
		if err != nil {
			log.Printf("vweb: multipart create part error %q: %v", filePath, err)
			break
		}
		if _, err := file.Seek(b.start, io.SeekStart); err != nil {
			log.Printf("vweb: static file seek error %q: %v", filePath, err)
			break
		}
		if _, err := io.CopyBuffer(w, io.LimitReader(file, b.length), buf); err != nil {
			if !isClientGone(err) {
				log.Printf("vweb: static file copy error %q: %v", filePath, err)
			}
			break
		}
	}
	if err := mw.Close(); err != nil && !isClientGone(err) {
		log.Printf("vweb: multipart close error %q: %v", filePath, err)
	}
}

// isClientGone 判断错误是否由客户端主动断开导致（不值得作为服务端错误记录）。
func isClientGone(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "use of closed network connection")
}

// etagMatch 实现弱 ETag 比较（支持多个值与 *）
func etagMatch(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "*" {
		return true
	}
	for _, v := range strings.Split(header, ",") {
		v = strings.TrimSpace(v)
		// 弱比较：忽略 W/ 前缀差异
		if v == etag || strings.TrimPrefix(v, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}
