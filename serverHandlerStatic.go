package vweb

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// shshRange 定义了 HTTP Range 请求中的字节范围信息
type shshRange struct {
	seek, length int64
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

// etag 基于文件修改时间和大小生成强/弱校验符
// 返回: 示例 "W/\"65a1234-1024\""
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
func (T *serverHandlerStaticHeader) ranges(ranges string) (r []shshRange, n int64, err error) {
	size := T.fileInfo.Size()
	if !strings.HasPrefix(ranges, "bytes=") {
		return nil, 0, fmt.Errorf("invalid range format")
	}
	rdata := strings.Split(ranges[6:], ",")
	for _, v := range rdata {
		rv := strings.Split(strings.TrimSpace(v), "-")
		if len(rv) != 2 {
			continue
		}
		var start, end int64
		// 处理后缀范围: "-500" 表示最后500字节
		if rv[0] == "" {
			val, err := strconv.ParseInt(rv[1], 10, 64)
			if err != nil {
				continue
			}
			start = size - val
			if start < 0 {
				start = 0
			}
			end = size - 1
		} else { // 处理常规范围: "0-499" 或 "500-"
			var err error
			start, err = strconv.ParseInt(rv[0], 10, 64)
			if err != nil {
				continue
			}
			if rv[1] == "" { // "500-" 意为到文件末尾
				end = size - 1
			} else {
				end, err = strconv.ParseInt(rv[1], 10, 64)
				if err != nil {
					continue
				}
			}
		}
		// 越界保护
		if start < 0 || start >= size || start > end {
			continue
		}
		if end >= size {
			end = size - 1
		}
		length := end - start + 1
		r = append(r, shshRange{start, length})
		n += length
	}
	return r, n, nil
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
	// 仅允许 GET 和 HEAD 请求，HEAD 用于预检文件信息
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		rw.Header().Set("Allow", "GET, HEAD")
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	pathPart := T.PagePath
	if pathPart == "" {
		pathPart = filepath.Clean("/" + req.URL.Path)
	}

	filePath := filepath.Join(T.RootPath, pathPart)
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
func (T *ServerHandlerStatic) header(rw http.ResponseWriter, req *http.Request, fi os.FileInfo) (*serverHandlerStaticHeader, []shshRange, error) {
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

	// 缓存协商: ETag 比对
	if req.Header.Get("If-None-Match") != "" {
		if req.Header.Get("If-None-Match") == shsh.etag() {
			rw.WriteHeader(http.StatusNotModified)
			return nil, nil, fmt.Errorf("cached")
		}
	} else if ims := req.Header.Get("If-Modified-Since"); ims != "" {
		// 缓存协商: 最后修改时间比对
		if t, err := time.Parse(http.TimeFormat, ims); err == nil && fi.ModTime().Unix() <= t.Unix() {
			rw.WriteHeader(http.StatusNotModified)
			return nil, nil, fmt.Errorf("cached")
		}
	}

	// 处理 Range 请求
	ranges := req.Header.Get("Range")
	if ranges == "" {
		shsh.setContentLength(fi.Size())
		return shsh, nil, nil
	}

	block, _, err := shsh.ranges(ranges)
	if err != nil || len(block) == 0 {
		rw.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fi.Size()))
		rw.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil, nil, err
	}
	return shsh, block, nil
}

// body 将文件内容写入 HTTP 响应体，支持分段传输 (Range)
func (T *ServerHandlerStatic) body(rw http.ResponseWriter, req *http.Request, fi os.FileInfo, path string) {
	_, blocks, err := T.header(rw, req, fi)
	if err != nil {
		return
	}

	// 若为 HEAD 请求，仅返回头信息即可
	if req.Method == http.MethodHead {
		rw.WriteHeader(http.StatusOK)
		return
	}

	file, err := os.Open(path)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// 1. 无 Range 参数：全量传输
	if len(blocks) == 0 {
		rw.WriteHeader(http.StatusOK)
		io.Copy(rw, file)
		return
	}

	// 2. 单 Range 请求：分段读取部分内容
	if len(blocks) == 1 {
		b := blocks[0]
		rw.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.seek, b.seek+b.length-1, fi.Size()))
		rw.Header().Set("Content-Length", strconv.FormatInt(b.length, 10))
		rw.WriteHeader(http.StatusPartialContent)

		file.Seek(b.seek, io.SeekStart)
		io.CopyN(rw, file, b.length)
		return
	}

	// 3. 多 Range 请求：使用 multipart 协议传输
	mw := multipart.NewWriter(rw)
	rw.Header().Set("Content-Type", "multipart/byteranges; boundary="+mw.Boundary())
	rw.WriteHeader(http.StatusPartialContent)

	for _, b := range blocks {
		partH := make(textproto.MIMEHeader)
		partH.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", b.seek, b.seek+b.length-1, fi.Size()))
		partH.Set("Content-Type", rw.Header().Get("Content-Type"))

		w, err := mw.CreatePart(partH)
		if err != nil {
			break
		}

		file.Seek(b.seek, io.SeekStart)
		io.CopyN(w, file, b.length)
	}
	mw.Close()
}
