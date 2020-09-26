package vweb
import (
	"fmt"
    "net/http"
    "io"
    "net"
    "bufio"
)

type Responser interface{
    Write([]byte) (int, error)                                                                  // 写入字节
    WriteString(string) (int, error)                                                            // 写入字符串
    ReadFrom(io.Reader) (int64, error)                                                          // 读取并写入
    Redirect(string, int)                                                                       // 转向
    WriteHeader(int)                                                                            // 状态码
    Error(string, int)                                                                          // 错误
    Flush()                                                                                     // 刷新缓冲
    Push(target string, opts *http.PushOptions) error											// HTTP/2推送
    Hijack() (net.Conn, *bufio.ReadWriter, error)												// 劫持，能双向互相发送信息
}

//response 模本点的响应写入
type response struct {
	buffSize	int64							// 写入的缓冲大小
    r   		*http.Request                 	// 请求
    w   		http.ResponseWriter            	// 响应
    td			*TemplateDot                    // 模板点
}

//Write 写入正文
//	p []byte      数据
//	int           写入的长度
//	error         错误
func (T *response) Write(p []byte) (int, error) {
    T.td.Writed = true
    return T.w.Write(p)
}

//WriteString 写入正文
//	s string      字符串
//	int           写入的长度
//	error         错误
func (T *response) WriteString(s string) (int, error){
    T.td.Writed = true
    return io.WriteString(T.w, s)
}

//ReadFrom 读取写入正文
//	src io.Reader	从src读取
//	int64         写入的长度
//	error         错误
func (T *response) ReadFrom(src io.Reader) (written int64, err error) {
    T.td.Writed = true
    	
	buffsize := T.buffSize
	if buffsize == 0 {
		buffsize = defaultDataBufioSize
	}

	var(
		p		= make([]byte, buffsize)
		flush	= T.w.(http.Flusher)
	)
	//正常读出文件
	for {
        nr, er := src.Read(p)
        if nr > 0{
	        nw, ew := T.w.Write(p[:nr])
	        if nw > 0 {
	    		written += int64(nw)
	        }
	        if ew != nil {
	        	err = ew
	        	break
	        }
  			if nr != nw {
  				err = io.ErrShortWrite
  				break
  			}
	        flush.Flush()
        }
        if er != nil {
  			if er != io.EOF {
  				err = er
  			}
            break
        }
	}
	return written, err
}

//Redirect 重定向
//	urlStr string 网址
//	code int      状态码
func (T *response) Redirect(urlStr string, code int) {
    T.td.Writed = true
    http.Redirect(T.w, T.r, urlStr, code)
}

//WriteHeader 状态码
//	code int      状态码
func (T *response) WriteHeader(code int) {
	T.w.WriteHeader(code)
}

//Error 错误
//  err string    错误字符串
//  code int      状态码
func (T *response) Error(err string, code int) {
    T.td.Writed = true
    http.Error(T.w, err, code)
}

//Flush 刷新缓冲
func (T *response) Flush() {
    T.td.Writed = true
    T.w.(http.Flusher).Flush()
}

//Push HTTP/2推送
//	target string			路径
//	opts *http.PushOptions	选项
//	error					错误
func (T *response) Push(target string, opts *http.PushOptions) error {
	if push, ok := T.w.(http.Pusher); ok {
    	return push.Push(target, opts)
	}
	return fmt.Errorf("vweb: 不支持 http.Pusher !")
}

//Hijack 劫持，能双向互相发送信息
//	net.Conn			连接
//	*bufio.ReadWriter	缓冲写入
//	error				错误
func (T *response) Hijack() (net.Conn, *bufio.ReadWriter, error) {
    if hijack, ok := T.w.(http.Hijacker); ok {
    	T.td.Writed = true
    	return hijack.Hijack()
    }
	return nil, nil, fmt.Errorf("vweb: 不支持 http.Hijacker !")
}