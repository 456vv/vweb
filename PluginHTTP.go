package vweb
import (
	//"fmt"
    "net"
    "net/http"
    //"net/url"
    //"crypto/x509"
    "crypto/tls"
    //"io"
    "time"
    //"io/ioutil"
    "context"
    "github.com/456vv/verror"
)

//http插件接口
type PluginHTTP interface{
	Type() PluginType
    ServeHTTP(w http.ResponseWriter, r *http.Request)								// 服务HTTP
    RoundTrip(r *http.Request) (resp *http.Response, err error)						// 代理
    CancelRequest(req *http.Request)												// 取消HTTP请求
    CloseIdleConnections()															// 关闭空闲连接
    RegisterProtocol(scheme string, rt http.RoundTripper)							// 注册新协议
}
//插件HTTP客户端
type PluginHTTPClient struct{
    Tr			*http.Transport		// 客户端
   	Addr		string				// 地址
   	Scheme		string				// 协议（用于默认填充）
    Host		string				// 请求Host（用于默认填充）
    Dialer 		*net.Dialer
}
//快速连接HTTP
//	PluginHTTP	插件HTTP
//	error		错误
func (T *PluginHTTPClient) Connection() (PluginHTTP, error) {
	if T.Tr == nil {
		return nil, verror.TrackError("vweb: Tr字段不可以为空！")
	}
	if T.Dialer == nil {
		T.Dialer = &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}
	}
	if T.Addr != "" {
		if T.Tr.DialContext == nil {
	        T.Tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error){
	            return T.Dialer.DialContext(ctx, network, T.Addr)
	        }
		}
        if T.Tr.TLSClientConfig != nil {
	        T.Tr.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error){
	            return tls.DialWithDialer(T.Dialer, network, T.Addr, T.Tr.TLSClientConfig)
	        }
        }
	}
	return &pluginHTTP{tr:T.Tr, host:T.Host, scheme:T.Scheme}, nil
}

//pluginHTTP 连接HTTP
type pluginHTTP struct{
	tr		*http.Transport
	host	string
	scheme	string
}

//插件类型
//	PluginType 插件类型
func (T *pluginHTTP) Type() PluginType {
	return PluginTypeHTTP
}

//ServeHTTP 服务器处理
//	w http.ResponseWriter    响应
//	r *http.Request          请求
func (T *pluginHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    resp, err := T.RoundTrip(r)
    if err != nil {
        //504错误是（网关超时） 服务器作为网关或代理，但是没有及时从上游服务器收到请求。
        http.Error(w, err.Error(), http.StatusGatewayTimeout)
        return
    }
    //写入header标头
    rh := resp.Header
    wh := w.Header()
	for key, values := range rh {
		for _, value := range values {
			wh.Add(key, value)
		}
	}
    //写入状态码
    w.WriteHeader(resp.StatusCode)
    //写入body数据
    body    := resp.Body
    defer body.Close()
    //io.Copy(rw, body)
    p       := make([]byte, defaultDataBufioSize)
    flush   := w.(http.Flusher)
    for {
        nr, er := body.Read(p)
        if nr > 0 {
	        nw, ew := w.Write(p[:nr])
	        if ew != nil || nr != nw {
	        	//日志
	        	break
	        }
	        flush.Flush()
        }
        if er != nil {
	        //日志
            break
        }
	}
}

//RoundTrip 单一的HTTP请求
//	r *http.Request            请求
//	resp *http.Response        响应
//	err error                  错误
func (T *pluginHTTP) RoundTrip(r *http.Request) (resp *http.Response, err error) {
	//由于req中的URL不完整，需要补充
	T.fillCompleteURL(r)
    return T.tr.RoundTrip(r)
}

//fillCompleteURL 补充完整URL
//	r *http.Request          请求
func (T *pluginHTTP) fillCompleteURL(r *http.Request) {
    if r.Host == "" {
        r.Host = T.host
        r.Header.Set("Host", r.Host)
    }
    r.URL.Host = r.Host
    if r.URL.Scheme == "" {
    	r.URL.Scheme = T.scheme
    }
}

//CancelRequest 取消HTTP请求
//	r *http.Request            请求
func (T *pluginHTTP) CancelRequest(r *http.Request) {
  	T.tr.CancelRequest(r)
}

//CloseIdleConnections 关闭空闲连接
func (T *pluginHTTP) CloseIdleConnections() {
    T.tr.CloseIdleConnections()
}

//RegisterProtocol 注册新协议
//	scheme string					协议
//	rt http.RoundTripper            新代理
func (T *pluginHTTP) RegisterProtocol(scheme string, rt http.RoundTripper) {
    T.tr.RegisterProtocol(scheme, rt)
}
