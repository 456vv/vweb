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
	"strconv"
	"strings"
	"time"
)

// shshRange 定义了 HTTP Range 请求中的字节范围信息
type shshRange struct {
	start, length int64
}

// serverHandlerStaticHeader 负责辅助设置 HTTP 响应头，简化 Header 写入逻辑
type serverHandlerStaticHeader struct {
	fileInfo os.FileInfo
	wh       http.Header
}

// setETag 设置 ETag 响应头，用于浏览器校验缓存有效性
func (T *serverHandlerStaticHeader) setETag() {
	T.wh.Set("ETag", T.etag())
}

// etag 基于文件修改时间和大小生成弱校验符
// 返回: 示例 W/"65a1234-1024"
func (T *serverHandlerStaticHeader) etag() string {
	return fmt.Sprintf(`W/"%x-%x"`, T.fileInfo.ModTime().UnixNano(), T.fileInfo.Size())
}

// ranges 解析 HTTP 请求头中的 "Range" 字段，计算文件的读取位置和长度
// 参数: ranges - 字符串示例 "bytes=0-499, 1000-1499"
// 返回:
//
//	r - 解析后的切片数组 [{seek:0, length:500}, {seek:1000, length:500}]
//	n - 总长度 1000
//	err - 若格式错误返回 error
func (T *serverHandlerStaticHeader) ranges(ranges string) (r []shshRange, n int64) {
	size := T.fileInfo.Size()
	// 去掉 "bytes=" 前缀后按逗号分割，忽略空段
	rdata := strings.SplitSeq(ranges[6:], ",")
	for v := range rdata {
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
				// "500-" 意为到文件末尾
				end = size - 1
			} else {
				end, e = strconv.ParseInt(rv[1], 10, 64)
				if e != nil {
					continue
				}
			}
		}

		if end >= size {
			end = size - 1
		}
		// 越界与合法性保护（符合 RFC 7233）
		if start >= size || start > end {
			continue
		}

		length := end - start + 1
		r = append(r, shshRange{start, length})
		n += length
	}
	return r, n
}

// 设置基础 HTTP 响应头
func (T *serverHandlerStaticHeader) setLastModified() {
	T.wh.Set("Last-Modified", T.fileInfo.ModTime().UTC().Format(http.TimeFormat))
}

func (T *serverHandlerStaticHeader) setDate() {
	T.wh.Set("Date", time.Now().UTC().Format(http.TimeFormat))
}

func (T *serverHandlerStaticHeader) setContentLength(l int64) {
	T.wh.Set("Content-Length", strconv.FormatInt(l, 10))
}

func (T *serverHandlerStaticHeader) setAcceptRanges() {
	T.wh.Set("Accept-Ranges", "bytes")
}

// setPageExpired 设置缓存过期策略 (Cache-Control & Expires)
func (T *serverHandlerStaticHeader) setPageExpired(pageExpired int64) {
	T.wh.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", pageExpired))
	T.wh.Set("Expires", time.Now().Add(time.Duration(pageExpired)*time.Second).UTC().Format(http.TimeFormat))
}

// ServerHandlerStatic 静态文件服务配置
type ServerHandlerStatic struct {
	RootPath, PagePath string
	PageExpired        int64 // 缓存过期时长（单位：秒），0则不缓存
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

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		http.Error(rw, "File not found", http.StatusNotFound)
		return
	}
	if fileInfo.IsDir() {
		http.Error(rw, "Forbidden", http.StatusForbidden)
		return
	}

	// 自动识别 MIME 类型，防止浏览器直接下载
	ctype := mime.TypeByExtension(filepath.Ext(filePath))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	rw.Header().Set("Content-Type", ctype)

	T.body(rw, req, fileInfo, filePath)
}

// header 初始化响应头，并处理 304 Not Modified 协商逻辑
// 返回: 处理器对象, 若为 Range 请求则返回对应的字节区间列表
func (T *ServerHandlerStatic) header(rw http.ResponseWriter, req *http.Request, fi os.FileInfo) ([]shshRange, bool) {
	if fi.Size() <= 0 {
		return nil, false
	}

	shsh := &serverHandlerStaticHeader{fileInfo: fi, wh: rw.Header()}
	shsh.setDate()
	shsh.setLastModified()
	shsh.setETag()
	shsh.setAcceptRanges()

	if T.PageExpired != 0 {
		shsh.setPageExpired(T.PageExpired)
	} else {
		rw.Header().Set("Cache-Control", "no-cache")
	}

	etag := shsh.etag()

	// 缓存协商: 优先 ETag（If-None-Match）
	if inm := req.Header.Get("If-None-Match"); inm != "" {
		// 支持多个 ETag 以及弱比较（简单实现：直接字符串匹配或 *）
		if inm == "*" || strings.Contains(inm, etag) {
			rw.WriteHeader(http.StatusNotModified)
			return nil, false
		}
	} else if ims := req.Header.Get("If-Modified-Since"); ims != "" {
		// 缓存协商: 最后修改时间比对（秒级精度即可）
		if t, err := http.ParseTime(ims); err == nil {
			// 文件修改时间不晚于客户端时间则 304
			if !fi.ModTime().After(t) {
				rw.WriteHeader(http.StatusNotModified)
				return nil, false
			}
		}
	}

	// 若同时存在 If-Range，需校验后才应用 Range
	if ir := req.Header.Get("If-Range"); ir != "" {
		// If-Range 可以是 ETag 或 HTTP-date
		useRange := false
		if strings.HasPrefix(ir, `W/"`) || strings.HasPrefix(ir, `"`) {
			if ir == etag {
				useRange = true
			}
		} else if t, err := http.ParseTime(ir); err == nil {
			if !fi.ModTime().After(t) {
				useRange = true
			}
		}
		if !useRange {
			// If-Range 不匹配，回退到全量响应
			shsh.setContentLength(fi.Size())
			return nil, true
		}
	}

	// 处理 Range 请求（仅在 GET 时有意义，但 HEAD 也需正确设置头）
	ranges := req.Header.Get("Range")
	if ranges == "" {
		shsh.setContentLength(fi.Size())
		return nil, true
	}
	if !strings.HasPrefix(ranges, "bytes=") {
		rw.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil, false
	}

	block, _ := shsh.ranges(ranges)
	return block, true
}

// body 将文件内容写入 HTTP 响应体，支持分段传输 (Range)
func (T *ServerHandlerStatic) body(rw http.ResponseWriter, req *http.Request, fi os.FileInfo, path string) {
	blocks, ok := T.header(rw, req, fi)
	if !ok {
		return
	}

	// 若为 HEAD 请求，仅返回头信息即可（Content-Length 等已在 header 中设置）
	if req.Method == http.MethodHead {
		// 对于无 Range 的 HEAD，状态码 200；有 Range 时也应返回 206 的头，但不写 body
		if len(blocks) == 0 {
			rw.WriteHeader(http.StatusOK)
		} else if len(blocks) == 1 {
			b := blocks[0]
			rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, fi.Size()))
			rw.Header().Set("Content-Length", strconv.FormatInt(b.length, 10))
			rw.WriteHeader(http.StatusPartialContent)
		} else {
			// 多 Range 的 HEAD 较复杂，简化为 200 并清除可能不准确的 Content-Length
			rw.Header().Del("Content-Length")
			rw.WriteHeader(http.StatusOK)
		}
		return
	}

	file, err := os.Open(path)
	if err != nil {
		http.Error(rw, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// 1. 无 Range 参数：全量传输
	if len(blocks) == 0 {
		rw.WriteHeader(http.StatusOK)
		// 使用有限缓冲区的 Copy 提升性能并降低内存峰值
		io.Copy(rw, file)
		return
	}

	// 2. 单 Range 请求：分段读取部分内容
	if len(blocks) == 1 {
		b := blocks[0]
		rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, fi.Size()))
		rw.Header().Set("Content-Length", strconv.FormatInt(b.length, 10))
		rw.WriteHeader(http.StatusPartialContent)

		if _, err := file.Seek(b.start, io.SeekStart); err != nil {
			return
		}
		io.CopyN(rw, file, b.length)
		return
	}

	// 3. 多 Range 请求：使用 multipart/byteranges 协议传输
	// 注意：必须在 WriteHeader 之前设置 Content-Type
	mw := multipart.NewWriter(rw)
	ctype := rw.Header().Get("Content-Type")
	rw.Header().Set("Content-Type", "multipart/byteranges; boundary="+mw.Boundary())
	// 多 Range 时不设置 Content-Length（由传输编码决定）
	rw.Header().Del("Content-Length")
	rw.WriteHeader(http.StatusPartialContent)

	for _, b := range blocks {
		partH := make(textproto.MIMEHeader)
		partH.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.start, b.start+b.length-1, fi.Size()))
		partH.Set("Content-Type", ctype)

		w, err := mw.CreatePart(partH)
		if err != nil {
			break
		}
		if _, err := file.Seek(b.start, io.SeekStart); err != nil {
			break
		}
		if _, err := io.CopyN(w, file, b.length); err != nil {
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

	// 检查中间是否存在 ".." 组件（同时兼容 / 与系统分隔符）
	return !slices.Contains(strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == filepath.Separator }), "..")
}
