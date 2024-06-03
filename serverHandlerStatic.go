package vweb

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// shshRange Range-标头-处理静态页面文件
type shshRange struct {
	seek, length int64 // 偏移，长度
}

// serverHandlerStaticHeader 标头-处理静态页面文件
type serverHandlerStaticHeader struct {
	fileInfo os.FileInfo // 文件信息
	wh       http.Header // 响应HTTP头
}

// setETag 设置内容不变标识
func (T *serverHandlerStaticHeader) setETag() {
	T.wh.Set("ETag", T.etag())
}

// etag 内容不变标识，需要设置状态码304。格式："文件最后修改时间:文件字节" == "13a8a1e1232237d0:379b"
//
//	string    标识符
func (T *serverHandlerStaticHeader) etag() string {
	modifiedTime := T.fileInfo.ModTime().UnixNano()
	fileSize := T.fileInfo.Size()
	return fmt.Sprintf("\"%x:%x\"", modifiedTime, fileSize)
}

// ranges 格式化Range，并过滤无效的
//
//	ranges string    请求标头Range
//	r []shshRange
//	n int64
//	err error
func (T *serverHandlerStaticHeader) ranges(ranges string) (r []shshRange, n int64, err error) {
	size := T.fileInfo.Size() - 1
	ri := strings.Index(ranges, "bytes=")
	if ri != 0 || len(ranges) <= 6 {
		return nil, 0, errors.New("vweb: 附带的Range内容不支持，格式应该是(Range: bytes=0-1024)")
	}
	rdata := strings.Split(ranges[6:], ",")
	for _, v := range rdata {
		rv := strings.Split(v, "-")
		if len(rv) == 1 || len(rv) > 2 || (rv[0] == "" && rv[1] == "0") {
			return nil, 0, fmt.Errorf("vweb: 这不是有效的格式。Error(%s)", v)
		}
		start, serr := strconv.ParseInt(rv[0], 10, 64)
		end, lerr := strconv.ParseInt(rv[1], 10, 64)
		switch {
		case rv[0] == "" && lerr == nil:
			// 格式: Range: bytes=-123
			// 表示后面的123个字节
			if end > size {
				start = 0
				end = size
			} else {
				start = (size - end) + 1
				end = size
			}
		case serr == nil && rv[1] == "":
			// 格式: Range: bytes=123-
			// 表示从123个字节开始，到最后面
			end = size
		case serr == nil && lerr == nil:
			// 格式: Range: bytes=123-567
			// 正确的格式，不做任何处理
			if end > size {
				end = size
			}
		default:
			// 错误的格式
			return nil, 0, fmt.Errorf("vweb: Range数值不正确，Error(%s)", v)
		}
		// 开始大于结束，无效的，跳过。
		if start > end {
			continue
		}
		length := (end - start) + 1
		r = append(r, shshRange{start, length})
		n = n + length
	}
	return r, n, nil
}

// setLastModified 设置文件最后修改时间
func (T *serverHandlerStaticHeader) setLastModified() {
	T.wh.Set("Last-Modified", T.lastModified())
}

// lastModified 文件最后修改时间
//
//	string    日期时间
func (T *serverHandlerStaticHeader) lastModified() string {
	// 时间格式是：Fri, 01 Aug 2014 11:57:57 GMT
	return T.fileInfo.ModTime().Format(http.TimeFormat)
}

// setDate 设置日期时间
func (T *serverHandlerStaticHeader) setDate() {
	// 时间格式是：Fri, 01 Aug 2014 11:57:57 GMT
	T.wh.Set("Date", time.Now().Format(http.TimeFormat))
}

// setContentLength 设置文件大小字节
func (T *serverHandlerStaticHeader) setContentLength() {
	T.wh.Set("Content-Length", T.contentLength())
}

// contentLength 文件大小字节
//
//	string    文件大小
func (T *serverHandlerStaticHeader) contentLength() string {
	i64 := T.fileInfo.Size()
	return fmt.Sprint(i64)
}

// setAcceptRanges 设置Range支持的类型
func (T *serverHandlerStaticHeader) setAcceptRanges() {
	T.wh.Set("Accept-Ranges", "bytes")
}

// setPageExpired 设置页面固定过期时间
//
//	pageExpired int64	页面过期
func (T *serverHandlerStaticHeader) setPageExpired(pageExpired int64) {
	T.wh.Set("Cache-Control", fmt.Sprintf("must-revalidate,max-age=%d", pageExpired))
	T.wh.Set("Expires", time.Now().Add(time.Duration(pageExpired)*time.Second).Format(http.TimeFormat))
}

// ServerHandlerStatic 处理静态页面文件
type ServerHandlerStatic struct {
	RootPath, PagePath string      // 根目录, 页路径
	PageExpired        int64       // 页面过期时间（秒为单位）
	BuffSize           int         // 缓冲块大小
	fileInfo           os.FileInfo // 文件基本信息
}

// serveHTTP 服务HTTP
//
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerStatic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// 打开文件
	filePath := filepath.Join(T.RootPath, T.PagePath)
	file, err := os.Open(filePath)
	if err != nil {
		// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	T.fileInfo = fileInfo

	// 处理静态文件的 Header 报头
	rangeBlock, err := T.header(rw, req)
	if err != nil {
		return
	}

	// 处理静态文件的 body 数据
	T.body(rw, rangeBlock)
}

// Header 处理静态文件的Header 报头
//
//	rw http.ResponseWriter      响应
//	req *http.Request           请求
//	[]shshRange				    数据块，如何为nil，则读取所有数据。否则，接数据块读取。
//	error						如果出错，说明Range配置出错。或内容没有发生变化，客户端使用缓存。
func (T *ServerHandlerStatic) header(rw http.ResponseWriter, req *http.Request) ([]shshRange, error) {
	rh := req.Header
	wh := rw.Header()

	// 设置响应标头
	shsh := &serverHandlerStaticHeader{
		fileInfo: T.fileInfo,
		wh:       wh,
	}
	// 设置静态文件的必带文件标头
	shsh.setDate()
	shsh.setLastModified()
	shsh.setETag()
	shsh.setAcceptRanges()
	if T.PageExpired != 0 {
		shsh.setPageExpired(T.PageExpired)
	}

	var (
		block   []shshRange
		dataLen int64
		err     error
		ranges  string = rh.Get("Range")
	)

	if ranges == "" {
		// 如果 Range 头域为空，可以使用ETag来缓存
		if shsh.etag() == rh.Get("If-None-Match") {
			rw.WriteHeader(304)
			return nil, fmt.Errorf("vweb: 服务端文件没有变化")
		}
		shsh.setContentLength()
	} else {
		// 解析range
		block, dataLen, err = shsh.ranges(ranges)
		if err != nil {
			rw.WriteHeader(416)
			return nil, err
		}

		// 如果请求长度大于文件长度，则应该直接下载整个文件。忽略Range
		if dataLen > T.fileInfo.Size() || dataLen == 0 {
			shsh.setContentLength()
			block = nil
		}
	}
	return block, nil
}

// body 处理静态文件的 body 数据
//
//	rw http.ResponseWriter    	响应
//	rangeBlock []shshRange		数据块，如何为nil，则读取所有数据。否则，接数据块读取。
func (T *ServerHandlerStatic) body(rw http.ResponseWriter, rangeBlock []shshRange) {
	// 打开文件
	filePath := filepath.Join(T.RootPath, T.PagePath)
	file, err := os.Open(filePath)
	if err != nil {
		// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	buffsize := T.BuffSize
	if buffsize == 0 {
		buffsize = defaultDataBufioSize
	}

	// 处理静态文件的 body 数据
	var (
		wh    = rw.Header()
		p     = make([]byte, buffsize)
		flush = rw.(http.Flusher)
	)
	switch len(rangeBlock) {
	case 0:
		rw.WriteHeader(200)
		// 正常读出文件
		for {
			nr, er := file.Read(p)
			if nr > 0 {
				nw, ew := rw.Write(p[:nr])
				if ew != nil || nr != nw {
					// 日志
					return
				}
				flush.Flush()
			}
			if er != nil {
				// 日志
				return
			}
		}
	case 1:
		// 只有一个块
		block := rangeBlock[0]
		wh.Set("Content-Length", fmt.Sprint(block.length))
		wh.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", block.seek, (block.seek+block.length)-1, T.fileInfo.Size()))
		// 标头写入完成
		rw.WriteHeader(206)
		file.Seek(block.seek, 0)
		io.Copy(rw, io.LimitReader(file, block.length))
		// rw.(io.ReaderFrom).ReadFrom(io.LimitReader(file, block.length))
	default:
		// 多个块
		bytesBuffer := bytes.NewBuffer(nil)
		multipartWriter := multipart.NewWriter(bytesBuffer)
		defer multipartWriter.Close()

		bytesBuffer.WriteString("\r\n") // 首行是空行
		for _, block := range rangeBlock {
			textprotoMIMEHeader := make(textproto.MIMEHeader)
			textprotoMIMEHeader.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", block.seek, block.seek+(block.length-1), T.fileInfo.Size()))
			textprotoMIMEHeader.Set("Content-Type", wh.Get("Content-Type"))
			ioWriter, err := multipartWriter.CreatePart(textprotoMIMEHeader)
			if err != nil {
				break
			}

			file.Seek(block.seek, 0)
			io.CopyN(ioWriter, file, block.length)
		}
		bytesBuffer.WriteString(fmt.Sprintf("\r\n--%s\r\n", multipartWriter.Boundary())) // 结尾分行符
		// 设置数据长度和内容类型
		wh.Set("Content-Length", fmt.Sprint(bytesBuffer.Len()))
		wh.Set("Content-Type", fmt.Sprintf("multipart/byteranges; boundary=%s", multipartWriter.Boundary()))
		rw.WriteHeader(206)
		bytesBuffer.WriteTo(rw)
	}
}
