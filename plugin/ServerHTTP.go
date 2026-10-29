package plugin

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/456vv/vweb/v3"
	"github.com/456vv/vweb/v3/server"
	"golang.org/x/crypto/acme/autocert"
)

// ServerHTTP 服务器HTTP
type ServerHTTP struct {
	*http.Server                      // HTTP
	Addr         string               // 监听地址
	Route        *vweb.Route          // 路由表
	AutoCert     *autocert.Manager    // 自动申请证书
	l            tcpKeepAliveListener // 监听器
}

// NewServerHTTP HTTP服务对象
func NewServerHTTP() *ServerHTTP {
	ser := &ServerHTTP{
		Server: new(http.Server),
		Route:  new(vweb.Route),
	}
	ser.Server.BaseContext = func(l net.Listener) context.Context {
		ctx := context.WithValue(context.Background(), server.ServerContextKey, ser)
		return context.WithValue(ctx, vweb.ListenerContextKey, ser.l.TCPListener)
	}

	ser.Server.ConnContext = func(ctx context.Context, rwc net.Conn) context.Context {
		return context.WithValue(ctx, vweb.ConnContextKey, rwc)
	}

	return ser
}

// LoadTLS 加载证书文件
//
//	certFile     证书公钥
//	keyFile      证书私钥
func (T *ServerHTTP) LoadTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	if T.Server.TLSConfig == nil {
		T.Server.TLSConfig = new(tls.Config)
	}
	T.l.tlsconfig = T.Server.TLSConfig
	T.Server.TLSConfig.Certificates = append(T.Server.TLSConfig.Certificates, cert)
	return nil
}

// ListenAndServe 监听并启动
//
//	error 错误
func (T *ServerHTTP) ListenAndServe() error {
	if T.Addr == "" {
		T.Addr = ":http"
	}
	l, err := net.Listen("tcp", T.Addr)
	if err != nil {
		return err
	}
	return T.Serve(l)
}

// Serve 监听
//
//	error 错误
func (T *ServerHTTP) Serve(l net.Listener) error {
	tcpLn, ok := l.(*net.TCPListener)
	if !ok {
		return fmt.Errorf("vweb/plugin: listener must be *net.TCPListener, got %T", l)
	}
	tcpAddr, ok := tcpLn.Addr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("vweb/plugin: listener address must be *net.TCPAddr, got %T", tcpLn.Addr())
	}
	ip := tcpAddr.IP
	if ip == nil {
		ip = net.IPv4zero
	} else if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	T.Addr = net.JoinHostPort(ip.String(), strconv.Itoa(tcpAddr.Port))
	T.l.TCPListener = tcpLn
	T.Server.Handler = http.HandlerFunc(T.Route.ServeHTTP)
	T.Server.Handler = vweb.AutoCert(T.AutoCert, T.Server.TLSConfig, T.Server.Handler)

	return T.Server.Serve(&T.l)
}

// Close 判断监听的连接
//
//	error 错误
func (T *ServerHTTP) Close() error {
	if T.Server != nil {
		return T.Server.Close()
	}
	return nil
}

func strSliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
