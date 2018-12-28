package vweb
import (
	"fmt"
    "net"
    "net/http"
    "net/url"
    "crypto/x509"
    "crypto/tls"
    //"io"
    "time"
    "io/ioutil"
    "context"
)

//http插件接口
type PluginHTTP interface{
    ServeHTTP(w http.ResponseWriter, r *http.Request)								// 服务HTTP
    RoundTrip(r *http.Request) (resp *http.Response, err error)						// 代理
    CancelRequest(req *http.Request)												// 取消HTTP请求
    CloseIdleConnections()															// 关闭空闲连接
    RegisterProtocol(scheme string, rt http.RoundTripper)							// 注册新协议
}

//配置HTTP插件客户端
//	c *PluginHTTPClient		客户端插件对象，创建时候可以是nil。
//	config ConfigSitePlugin	配置
//	*PluginHTTPClient		返回客户端插件对象
//	error					错误
func ConfigPluginHTTPClient(c *PluginHTTPClient, config ConfigSitePlugin) (*PluginHTTPClient, error) {
	return configHTTPClient(c, config)
}

func configHTTPClient(c *PluginHTTPClient, config ConfigSitePlugin) (*PluginHTTPClient, error) {
    netDialer := &net.Dialer{
    	DualStack		: config.DualStack,
    	FallbackDelay	: time.Duration(config.FallbackDelay) * time.Millisecond,
    	Timeout			: time.Duration(config.Timeout) * time.Millisecond,
    	KeepAlive		: time.Duration(config.KeepAlive) * time.Millisecond,
    }
   	if config.LocalAddr != "" {
		//设置本地拨号地址
		netTCPAddr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
		if err == nil {
			netDialer.LocalAddr = netTCPAddr
		}else{
   	    	return nil, fmt.Errorf("vweb: 本地 ConfigSitePlugin.LocalAddr 无法解析这个地址(%s)。格式应该是 111.222.444.555:0 或者 www.xxx.com:0", config.LocalAddr)
		}
	}
	
    if c == nil {
		c = &PluginHTTPClient{
			Tr:&http.Transport{},
		}
    }

	c.Host						= config.Host
	c.Scheme					= config.Scheme
	c.Addr						= config.Addr
	c.Tr.DisableKeepAlives		= config.DisableKeepAlives
	c.Tr.DisableCompression		= config.DisableCompression
	c.Tr.MaxIdleConns			= config.IdeConn
	c.Tr.MaxIdleConnsPerHost	= config.MaxIdleConnsPerHost
	c.Tr.MaxConnsPerHost		= config.MaxConnsPerHost
	c.Tr.MaxResponseHeaderBytes = config.MaxResponseHeaderBytes
		
	if config.ProxyURL != "" {
		c.Tr.Proxy = func(r *http.Request) (*url.URL, error){
			return url.Parse(config.ProxyURL)
		}
	}
	if d := config.ResponseHeaderTimeout; d != 0 {
		c.Tr.ResponseHeaderTimeout   = time.Duration(d) * time.Millisecond
	}
	if d := config.ExpectContinueTimeout; d != 0 {
		c.Tr.ExpectContinueTimeout   = time.Duration(d) * time.Millisecond
	}
	if d := config.IdleConnTimeout; d != 0 {
		c.Tr.IdleConnTimeout   = time.Duration(d) * time.Millisecond
	}
	if d := config.TLSHandshakeTimeout; d != 0 {
		c.Tr.TLSHandshakeTimeout   = time.Duration(d) * time.Millisecond
	}
    if config.ProxyConnectHeader != nil {
    	c.Tr.ProxyConnectHeader = config.ProxyConnectHeader
    }

    if config.TLS == nil || config.TLS.ServerName == "" {
        c.Tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error){
            return netDialer.DialContext(ctx, network, config.Addr)
        }
    }else{
        tlsconfig := &tls.Config{
             ServerName			: config.TLS.ServerName,
             InsecureSkipVerify	: config.TLS.InsecureSkipVerify,
        }
    	c.Tr.TLSClientConfig = tlsconfig

        if len(config.TLS.NextProtos) > 0 {
            tlsconfig.NextProtos = config.TLS.NextProtos
        }
        if len(config.TLS.CipherSuites) > 0 {
            tlsconfig.CipherSuites = config.TLS.CipherSuites
        }else{
			//内部判断并使用默认的密码套件
            tlsconfig.CipherSuites = nil
        }
        if config.TLS.ClientSessionCache != 0 {
            tlsconfig.ClientSessionCache = tls.NewLRUClientSessionCache(config.TLS.ClientSessionCache)
        }
        if len(config.TLS.CurvePreferences) != 0 {
            tlsconfig.CurvePreferences = config.TLS.CurvePreferences
        }
        
        
        if tlsconfig.RootCAs == nil {
        	tlsconfig.RootCAs = x509.NewCertPool()
        }
        for _, filename := range config.TLS.File {
        	b, err := ioutil.ReadFile(filename)
            if err != nil {
                //日志
            	continue
            }
            if !tlsconfig.RootCAs.AppendCertsFromPEM(b) {
            	//日志
            	continue
            }
        }
        c.Tr.DialTLS = func(network, addr string) (net.Conn, error){
            return tls.DialWithDialer(netDialer, network, config.Addr, tlsconfig)
        }
    }
    return c, nil
}

//插件HTTP客户端
type PluginHTTPClient struct{
    Tr			*http.Transport		// 客户端
   	Scheme		string				// 协议（默认）
    Host		string				// 请求Host（默认）
    Addr		string				// 地址（默认）
}

//快速连接HTTP
//	PluginHTTP	插件HTTP
//	error		错误
func (T *PluginHTTPClient) Connection() (PluginHTTP, error) {
	
	if T.Tr == nil {
		T.Tr = http.DefaultTransport.(*http.Transport)
		if T.Addr != "" {
	        T.Tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error){
	            return (&net.Dialer{
			        Timeout:   30 * time.Second,
			        KeepAlive: 30 * time.Second,
			        DualStack: true,
			    }).DialContext(ctx, network, T.Addr)
	        }
		}
	}
	
	return &pluginHTTP{client:T}, nil
}

//pluginHTTP 连接HTTP
type pluginHTTP struct{
	client 	*PluginHTTPClient
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
			wh.Set(key, value)
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
        if nr > 0{
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

    return T.client.Tr.RoundTrip(r)
}

//fillCompleteURL 补充完整URL
//	r *http.Request          请求
func (T *pluginHTTP) fillCompleteURL(r *http.Request) {
    if r.Host == "" {
        r.Host = T.client.Host
        r.Header.Set("Host", r.Host)
    }
    r.URL.Host = r.Host
    if r.URL.Scheme == "" {
    	r.URL.Scheme = T.client.Scheme
    }
}

//CancelRequest 取消HTTP请求
//	r *http.Request            请求
func (T *pluginHTTP) CancelRequest(r *http.Request) {
  	T.client.Tr.CancelRequest(r)
}

//CloseIdleConnections 关闭空闲连接
func (T *pluginHTTP) CloseIdleConnections() {
    T.client.Tr.CloseIdleConnections()
}

//RegisterProtocol 注册新协议
//	scheme string					协议
//	rt http.RoundTripper            新代理
func (T *pluginHTTP) RegisterProtocol(scheme string, rt http.RoundTripper) {
    T.client.Tr.RegisterProtocol(scheme, rt)
}

