package server
import (
	"sync"
	"github.com/456vv/vweb/v2"
	"github.com/456vv/verror"
	"github.com/456vv/vconnpool"
	"net"
	"net/http"
	"time"
	"net/url"
	"crypto/x509"
	"crypto/tls"
	"io/ioutil"
	"path/filepath"
)

type Pluginer interface{
	RPC(name string) (vweb.PluginRPC, error)
	HTTP(name string) (vweb.PluginHTTP, error)
}

type plugin struct {
	rpc 	sync.Map
	http	sync.Map
}

func (T *plugin) RPC(name string) (vweb.PluginRPC, error) {
	inf, ok := T.rpc.Load(name)
	if ok {
		client := inf.(*vweb.PluginRPCClient)
		return client.Connection()
	}
	return nil, verror.TrackErrorf("rpc plugin %s not found", name)
}
func (T *plugin) HTTP(name string) (vweb.PluginHTTP, error) {
	inf, ok := T.http.Load(name)
	if ok {
		client := inf.(*vweb.PluginHTTPClient)
		return client.Connection()
	}
	return nil, verror.TrackErrorf("http plugin %s not found", name)
}

//配置HTTP插件客户端
func configHTTPClient(c *vweb.PluginHTTPClient, config *ConfigSitePlugin) error {
	
	c.Addr						= config.Addr
	c.Host						= config.Host
	c.Scheme					= config.Scheme
	
	if c.Dialer == nil {
		c.Dialer = &net.Dialer{}
	}
   	if config.LocalAddr != "" {
		//设置本地拨号地址
		netTCPAddr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
		if err != nil {
   	    	return verror.TrackErrorf("ConfigSitePlugin.LocalAddr 地址无法解析这个(%s)。格式应该是 111.222.444.555:0 或者 www.xxx.com:0", config.LocalAddr)
		}
		c.Dialer.LocalAddr = netTCPAddr
	}
    c.Dialer.Timeout 		= time.Duration(config.Timeout) * time.Millisecond
    c.Dialer.KeepAlive   	= time.Duration(config.KeepAlive) * time.Millisecond
    c.Dialer.FallbackDelay	= time.Duration(config.FallbackDelay) * time.Millisecond
    c.Dialer.DualStack		= config.DualStack
	
	if c.Tr == nil {
		c.Tr = http.DefaultTransport.(*http.Transport)
	}
	c.Tr.Proxy = http.ProxyFromEnvironment
	if config.ProxyURL != "" {
		u, err := url.Parse(config.ProxyURL)
		if err != nil {
			return verror.TrackErrorf("代理地址不是有效的ConfigSitePlugin.ProxyURL(%s)", config.ProxyURL)
		}
		c.Tr.Proxy = http.ProxyURL(u)
	}
	c.Tr.DisableKeepAlives		= config.DisableKeepAlives
	c.Tr.DisableCompression		= config.DisableCompression
	c.Tr.MaxIdleConns			= config.IdeConn
	c.Tr.MaxIdleConnsPerHost	= config.MaxIdleConnsPerHost
	c.Tr.MaxConnsPerHost		= config.MaxConnsPerHost
	c.Tr.MaxResponseHeaderBytes = config.MaxResponseHeaderBytes
	c.Tr.ReadBufferSize			= config.ReadBufferSize
	c.Tr.ForceAttemptHTTP2		= config.ForceAttemptHTTP2
	c.Tr.WriteBufferSize		= config.WriteBufferSize
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
    if len(config.ProxyConnectHeader) != 0 {
    	c.Tr.ProxyConnectHeader = config.ProxyConnectHeader.Clone()
    }
	var tlsconfig *tls.Config
    if config.TLS != nil && config.TLS.ServerName != "" {
        tlsconfig = &tls.Config{
             ServerName			: config.TLS.ServerName,
             InsecureSkipVerify	: config.TLS.InsecureSkipVerify,
        }
        if len(config.TLS.NextProtos) > 0 {
            copy(tlsconfig.NextProtos, config.TLS.NextProtos)
        }
        if len(config.TLS.CipherSuites) > 0 {
            copy(tlsconfig.CipherSuites, config.TLS.CipherSuites)
        }else{
			//内部判断并使用默认的密码套件
            tlsconfig.CipherSuites = nil
        }
        if config.TLS.ClientSessionCache != 0 {
            tlsconfig.ClientSessionCache = tls.NewLRUClientSessionCache(config.TLS.ClientSessionCache)
        }
        if len(config.TLS.CurvePreferences) != 0 {
            copy(tlsconfig.CurvePreferences, config.TLS.CurvePreferences)
        }
		
		if tlsconfig.RootCAs == nil {
			if certPool, err := x509.SystemCertPool(); err == nil {
				//系统证书
				tlsconfig.RootCAs = certPool
			}else{
				//如果读取系统根证书失败，则创建新的证书
				tlsconfig.RootCAs = x509.NewCertPool()
			}
		}
		
        for _, filename := range config.TLS.RootCAa {
			//打开文件
			caData, err := ioutil.ReadFile(filename)
			if err != nil {
        		return verror.TrackErrorf("%s %s", filename, err.Error()) 
			}
			
			ext := filepath.Ext(filename)
			if ext == ".cert" {
				certificates, err := x509.ParseCertificates(caData)
				if err != nil {
        			return verror.TrackErrorf("%s %s", filename, err.Error()) 
				}
				for _, cert := range certificates {
					tlsconfig.RootCAs.AddCert(cert)
				}
			}else if ext == ".pem" {
				if !tlsconfig.RootCAs.AppendCertsFromPEM(caData) {
	        		return verror.TrackErrorf("%s %s\n", filename, "not is a valid PEM format")
				}
			}
        }
    }
   	c.Tr.TLSClientConfig = tlsconfig
   	
    return nil
}
//快速的配置RPC
func configRPCClient(c *vweb.PluginRPCClient, config *ConfigSitePlugin) error {
    c.Addr	= config.Addr
   	c.Path  = config.Path
    //RPC客户端连接池
    if c.ConnPool == nil {
    	c.ConnPool = &vconnpool.ConnPool{}
    }
    c.ConnPool.IdeConn 				= config.IdeConn
    c.ConnPool.MaxConn 				= config.MaxConn
    
    if c.ConnPool.Dialer  == nil {
	    c.ConnPool.Dialer = &net.Dialer{}
	}
   	if config.LocalAddr != "" {
		//设置本地拨号地址
		netTCPAddr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
		if err != nil {
   	    	return verror.TrackErrorf("ConfigSitePlugin.LocalAddr 地址无法解析这个(%s)。格式应该是 111.222.444.555:0 或者 www.xxx.com:0", config.LocalAddr)
		}
		c.ConnPool.Dialer.LocalAddr = netTCPAddr
	}
    c.ConnPool.Dialer.Timeout 		= time.Duration(config.Timeout) * time.Millisecond
    c.ConnPool.Dialer.KeepAlive   	= time.Duration(config.KeepAlive) * time.Millisecond
    c.ConnPool.Dialer.FallbackDelay	= time.Duration(config.FallbackDelay) * time.Millisecond
    c.ConnPool.Dialer.DualStack		= config.DualStack
	return nil
}
