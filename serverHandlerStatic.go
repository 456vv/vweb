package vweb

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

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
//
// 参数: ranges - 字符串示例 "bytes=0-499, 1000-1499"
// 返回:
//
//	r - 解析后的切片数组 [{start:0, length:500}, {start:1000, length:500}]
//	n - 合并后的总有效字节数
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
		T.header.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", pageExpired))
		T.header.Set("Expires", time.Now().Add(time.Duration(pageExpired)*time.Second).UTC().Format(http.TimeFormat))
	} else {
		T.header.Set("Cache-Control", "no-cache")
	}
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

// ServeHTTP 实现 http.Handler 接口，处理文件读取与响应
// 输入: rw - 响应流, req - HTTP请求
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

	// 确定页面路径（URL 路径风格）
	pagePath := T.PagePath
	if pagePath == "" {
		pagePath = req.URL.Path
	}
	// 使用 path.Clean 规范化 URL 风格路径，并去掉前导斜杠
	pagePath = path.Clean("/" + pagePath)
	pagePath = strings.TrimPrefix(pagePath, "/")
	if pagePath == "" || pagePath == "." {
		pagePath = "index.html"
	}

	// 跨平台路径拼接（FromSlash 保证 Windows 上正确）
	filePath := filepath.Join(T.RootPath, filepath.FromSlash(pagePath))

	// 严格防止路径穿越：最终路径必须位于 RootPath 之下
	rootAbs, err := filepath.Abs(T.RootPath)
	if err != nil {
		http.Error(rw, "vweb: invalid RootPath", http.StatusInternalServerError)
		return
	}
	// Clean + Abs 消除符号链接之外的 ".." 与多余分隔符
	fileAbs, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		http.Error(rw, "vweb: invalid file path", http.StatusNotFound)
		return
	}
	rel, err := filepath.Rel(rootAbs, fileAbs)
	if err != nil || !isLocalPath(rel) {
		http.Error(rw, "vweb: path traversal detected", http.StatusNotFound)
		return
	}

	fileInfo, err := os.Stat(fileAbs) // 使用绝对路径进行 stat 和后续操作
	if err != nil {
		http.Error(rw, "File not found", http.StatusNotFound)
		return
	}
	if fileInfo.IsDir() {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}
	// 仅提供普通文件，避免设备文件、命名管道等非普通文件造成阻塞或安全问题
	if !fileInfo.Mode().IsRegular() {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}

	blocks, ok := T.setHeader(rw, req, fileInfo, fileAbs)
	if !ok {
		return
	}

	T.body(rw, blocks, fileInfo, fileAbs)
}

// setHeader 初始化响应头，并处理 304 Not Modified 协商逻辑
// 返回: 处理器对象, 若为 Range 请求则返回对应的字节区间列表
func (T *ServerHandlerStatic) setHeader(rw http.ResponseWriter, req *http.Request, fi os.FileInfo, filePath string) ([]blockRange, bool) {
	sh := &serverHandlerStaticHeader{
		fileInfo: fi,
		header:   rw.Header(),
		blocks:   make([]blockRange, 0, 1),
	}

	sh.setContentType(filePath)
	sh.setDate()
	sh.setLastModified()
	sh.setETag()
	sh.setAcceptRanges()
	sh.setPageExpired(T.PageExpired)

	size := fi.Size()
	etag := sh.etag()
	modTime := fi.ModTime()

	// 缓存协商: 优先 ETag（If-None-Match）——弱比较
	if inm := req.Header.Get("If-None-Match"); inm != "" {
		if etagMatch(inm, etag) {
			rw.Header().Del("Content-Type")
			rw.Header().Del("Content-Length")
			rw.WriteHeader(http.StatusNotModified)
			return nil, false
		}
	} else if ims := req.Header.Get("If-Modified-Since"); ims != "" {
		if t, err := http.ParseTime(ims); err == nil {
			// 秒级精度：文件修改时间不晚于客户端时间 → 304
			if !modTime.After(t.Add(time.Second - 1)) {
				rw.Header().Del("Content-Type")
				rw.Header().Del("Content-Length")
				rw.WriteHeader(http.StatusNotModified)
				return nil, false
			}
		}
	}

	// 空文件在缓存协商完成后直接返回，避免丢失 304 行为
	if size <= 0 {
		sh.setContentLength(0)
		rw.WriteHeader(http.StatusOK)
		return nil, false
	}

	rangeHeader := req.Header.Get("Range")

	// 若同时存在 If-Range，需校验后才应用 Range（RFC 7233）
	if ir := req.Header.Get("If-Range"); ir != "" && rangeHeader != "" {
		useRange := false
		if strings.HasPrefix(ir, `W/"`) || strings.HasPrefix(ir, `"`) {
			// 强比较：必须完全一致（含 W/ 前缀）
			if ir == etag {
				useRange = true
			}
		} else if t, err := http.ParseTime(ir); err == nil {
			// 秒级比较（强校验）
			if modTime.Unix() == t.Unix() {
				useRange = true
			}
		}
		if !useRange {
			// If-Range 不匹配，回退到全量响应
			sh.setContentLength(size)
			if req.Method == http.MethodHead {
				rw.WriteHeader(http.StatusOK)
				return nil, false
			}
			return nil, true
		}
	}

	// 无 Range 或空文件已处理
	if rangeHeader == "" {
		sh.setContentLength(size)
		if req.Method == http.MethodHead {
			rw.WriteHeader(http.StatusOK)
			return nil, false
		}
		return nil, true
	}
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		sh.setContentRange(fmt.Sprintf("bytes */%d", size))
		sh.setContentLength(0)
		rw.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil, false
	}

	blocks, total := sh.ranges(rangeHeader)
	if len(blocks) == 0 {
		sh.setContentRange(fmt.Sprintf("bytes */%d", size))
		sh.setContentLength(0)
		rw.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil, false
	}

	// RFC 7233 / stdlib: 若请求的总字节数大于文件本身，忽略 Range
	if total > size {
		sh.setContentLength(size)
		if req.Method == http.MethodHead {
			rw.WriteHeader(http.StatusOK)
			return nil, false
		}
		return nil, true
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

// body 将文件内容写入 HTTP 响应体，支持分段传输 (Range)
func (T *ServerHandlerStatic) body(rw http.ResponseWriter, blocks []blockRange, fi os.FileInfo, filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(rw, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	size := fi.Size()
	bufp := copyBufPool.Get().(*[]byte)
	buf := *bufp
	defer copyBufPool.Put(bufp)

	// 1. 无 Range 参数：全量传输
	if len(blocks) == 0 {
		rw.WriteHeader(http.StatusOK)
		io.CopyBuffer(rw, file, buf)
		return
	}

	// 2. 单 Range 请求：分段读取部分内容
	if len(blocks) == 1 {
		b := blocks[0]
		rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, size))
		rw.Header().Set("Content-Length", strconv.FormatInt(b.length, 10))
		rw.WriteHeader(http.StatusPartialContent)

		if _, err := file.Seek(b.start, io.SeekStart); err != nil {
			return
		}
		io.CopyBuffer(rw, io.LimitReader(file, b.length), buf)
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

	for _, b := range blocks {
		partH := make(textproto.MIMEHeader)
		partH.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, size))
		partH.Set("Content-Type", ctype)

		w, err := mw.CreatePart(partH)
		if err != nil {
			break
		}
		if _, err := file.Seek(b.start, io.SeekStart); err != nil {
			break
		}
		if _, err := io.CopyBuffer(w, io.LimitReader(file, b.length), buf); err != nil {
			break
		}
	}
	mw.Close()
}

// isLocalPath 检查相对路径是否安全（无 ".."、非绝对路径等）
// 保留原有可导出/包内函数名称与签名
func isLocalPath(rel string) bool {
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

	// 使用标准库 IsLocal 增强跨平台安全（Go 1.20+）
	if !filepath.IsLocal(rel) {
		return false
	}

	// 检查中间是否存在 ".." 组件（同时兼容 / 与系统分隔符）
	return !slices.Contains(strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == filepath.Separator }), "..")
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
