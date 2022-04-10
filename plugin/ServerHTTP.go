package plugin

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strconv"

	"github.com/456vv/vweb/v2"
)

type ServerTLSFile struct {
	CertFile, KeyFile string // 证书，key 文件地址
}

// ServerHTTP 服务器HTTP
type ServerHTTP struct {
	*http.Server                      // HTTP
	Addr         string               // 监听地址
	Route        *vweb.Route          // 路由表
	l            tcpKeepAliveListener // 监听器
}

// NewServerHTTP HTTP服务对象
func NewServerHTTP() *ServerHTTP {
	ser := &ServerHTTP{
		Server: new(http.Server),
		Route:  &vweb.Route{},
	}
	ser.Server.Handler = http.HandlerFunc(ser.Route.ServeHTTP)
	ser.Server.BaseContext = func(l net.Listener) context.Context {
		return context.WithValue(context.Background(), vweb.ListenerContextKey, ser.l.TCPListener)
	}
	ser.Server.ConnContext = func(ctx context.Context, rwc net.Conn) context.Context {
		return context.WithValue(ctx, vweb.ConnContextKey, rwc)
	}

	return ser
}

// LoadTLS 加载证书文件
//	config *tls.Config          证书配置
//	files []ServerTLSFile       证书文件
func (T *ServerHTTP) LoadTLS(config *tls.Config, files []ServerTLSFile) error {
	for _, file := range files {
		cert, err := tls.LoadX509KeyPair(file.CertFile, file.KeyFile)
		if err != nil {
			T.l.tlsconf = nil
			return err
		}
		config.Certificates = append(config.Certificates, cert)
	}
	// 多证书
	config.BuildNameToCertificate()
	T.l.tlsconf = config
	return nil
}

// ListenAndServe 监听并启动
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
//	error 错误
func (T *ServerHTTP) Serve(l net.Listener) error {
	addr := l.Addr().(*net.TCPAddr)
	ip := addr.IP.To4()
	if ip == nil {
		ip = addr.IP.To16()
	}
	T.Addr = net.JoinHostPort(ip.String(), strconv.Itoa(addr.Port))
	T.l.TCPListener = l.(*net.TCPListener)
	return T.Server.Serve(&T.l)
}

// Close 判断监听的连接
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
