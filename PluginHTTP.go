package vweb

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

// PluginHTTP http插件接口
type PluginHTTP interface {
	Type() PluginType                                           // 类型
	ServeHTTP(w http.ResponseWriter, r *http.Request)           // 服务HTTP
	RoundTrip(r *http.Request) (resp *http.Response, err error) // 代理
	CancelRequest(req *http.Request)                            // 取消HTTP请求
	CloseIdleConnections()                                      // 关闭空闲连接
	RegisterProtocol(scheme string, rt http.RoundTripper)       // 注册新协议
}

// PluginHTTPClient 插件HTTP客户端
type PluginHTTPClient struct {
	Tr     *http.Transport // 客户端
	Addr   string          // 地址
	Scheme string          // 协议（用于默认填充）
	Host   string          // 请求Host（用于默认填充）
	Dialer *net.Dialer
}

// Connection 快速连接HTTP
//
//	PluginHTTP	插件HTTP
//	error		错误
func (T *PluginHTTPClient) Connection() (PluginHTTP, error) {
	if T.Tr == nil {
		return nil, errors.New("vweb: Tr字段不可以为空！")
	}

	phttp := &pluginHTTP{
		tr:     T.Tr,
		host:   T.Host,
		scheme: T.Scheme,
	}
	if T.Addr != "" && T.Tr.Proxy == nil {
		phttp.rewriteHost = T.Addr
	}
	return phttp, nil
}

// pluginHTTP 连接HTTP
type pluginHTTP struct {
	tr          *http.Transport
	host        string
	scheme      string
	rewriteHost string
}

// 插件类型
//
//	PluginType 插件类型
func (T *pluginHTTP) Type() PluginType {
	return PluginTypeHTTP
}

// ServeHTTP 服务器处理
//
//	w http.ResponseWriter    响应
//	r *http.Request          请求
func (T *pluginHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp, err := T.RoundTrip(r)
	if err != nil {
		// 504 网关超时：作为代理，未及时从上游收到请求
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
		return
	}
	// 写入header标头
	rh := resp.Header
	wh := w.Header()
	// 删除服务器默认增加的标头，避免泄露默认 Server/Content-Type
	wh.Del("Content-Type")
	wh.Del("Server")
	removeHopHeaders(rh) // 响应侧也剥离逐跳标头
	for key, values := range rh {
		for _, value := range values {
			wh.Add(key, value)
		}
	}
	// 写入状态码
	w.WriteHeader(resp.StatusCode)
	// 写入body数据
	body := resp.Body
	defer body.Close()
	// io.Copy(rw, body)
	flush, canFlush := w.(http.Flusher)

	p := make([]byte, defaultDataBufioSize)
	for {
		nr, er := body.Read(p)
		if nr > 0 {
			nw, ew := w.Write(p[:nr])
			if ew != nil || nr != nw {
				// 写失败或短写：客户端很可能已断开，或上游截断
				break
			}
			if canFlush {
				flush.Flush()
			}
		}
		if er != nil {
			// io.EOF 是正常结束；其它错误是上游读取失败
			if er != io.EOF {
				// 保持沉默或按需记录，避免刷屏；此处不做日志以维持原行为。
				_ = er
			}
			break
		}
	}
}

// RoundTrip 单一的HTTP请求
//
//	r *http.Request            请求
//	resp *http.Response        响应
//	err error                  错误
func (T *pluginHTTP) RoundTrip(r *http.Request) (resp *http.Response, err error) {
	outreq := r.Clone(r.Context())
	removeHopHeaders(outreq.Header)
	// 由于req中的URL不完整，需要补充
	T.fillCompleteURL(outreq)
	return T.tr.RoundTrip(outreq)
}

// fillCompleteURL 补充完整URL
//
//	r *http.Request          请求
func (T *pluginHTTP) fillCompleteURL(r *http.Request) {
	if r.Host == "" {
		r.Host = T.host
		r.Header.Set("Host", r.Host)
	}
	if T.rewriteHost != "" {
		r.URL.Host = T.rewriteHost
	} else if r.URL.Host == "" {
		r.URL.Host = r.Host
	}
	if r.URL.Scheme == "" {
		r.URL.Scheme = T.scheme
	}
}

// CancelRequest 取消HTTP请求
//
//	r *http.Request            请求
func (T *pluginHTTP) CancelRequest(r *http.Request) {
	T.tr.CancelRequest(r)
}

// CloseIdleConnections 关闭空闲连接
func (T *pluginHTTP) CloseIdleConnections() {
	T.tr.CloseIdleConnections()
}

// RegisterProtocol 注册新协议
//
//	scheme string					协议
//	rt http.RoundTripper            新代理
func (T *pluginHTTP) RegisterProtocol(scheme string, rt http.RoundTripper) {
	T.tr.RegisterProtocol(scheme, rt)
}

// hopHeaders 是不应在代理间转发的逐跳标头（RFC 7230 §6.1）。
var hopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive",
	"Proxy-Authenticate", "Proxy-Authorization",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func removeHopHeaders(h http.Header) {
	for _, c := range h.Values("Connection") {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				h.Del(f)
			}
		}
	}
	for _, k := range hopHeaders {
		h.Del(k)
	}
}
