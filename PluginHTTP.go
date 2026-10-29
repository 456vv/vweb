package vweb

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
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
	once   sync.Once
}

// Connection 快速连接HTTP
//
//	PluginHTTP	插件HTTP
//	error		错误
func (T *PluginHTTPClient) Connection() (PluginHTTP, error) {
	if T.Tr == nil {
		return nil, errors.New("vweb: Tr字段不可以为空！")
	}
	if T.Dialer == nil {
		T.Dialer = &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}
	}

	// 关键：仅当 Addr 非空时才消耗 once；后续补齐 Addr 再次调用仍能配置。
	if T.Addr != "" {
		T.once.Do(func() {
			addr := T.Addr
			dialer := T.Dialer
			if T.Tr.DialContext == nil {
				T.Tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, network, addr)
				}
			}
			if tlsCfg := T.Tr.TLSClientConfig; tlsCfg != nil && T.Tr.DialTLSContext == nil {
				tlsDialer := &tls.Dialer{NetDialer: dialer, Config: tlsCfg}
				T.Tr.DialTLSContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
					return tlsDialer.DialContext(ctx, network, addr)
				}
			}
		})
	}
	return &pluginHTTP{tr: T.Tr, host: T.Host, scheme: T.Scheme}, nil
}

// pluginHTTP 连接HTTP
type pluginHTTP struct {
	tr     *http.Transport
	host   string
	scheme string
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
	// 由于req中的URL不完整，需要补充
	T.fillCompleteURL(r)
	return T.tr.RoundTrip(r)
}

// fillCompleteURL 补充完整URL
//
//	r *http.Request          请求
func (T *pluginHTTP) fillCompleteURL(r *http.Request) {
	if r.Host == "" {
		r.Host = T.host
		r.Header.Set("Host", r.Host)
	}
	if r.URL.Host == "" {
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
