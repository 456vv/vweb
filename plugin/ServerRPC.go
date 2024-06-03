package plugin

import (
	"crypto/tls"
	"encoding/gob"
	"net"
	"net/http"
	"net/rpc"
	"strconv"

	"github.com/456vv/vweb/v2"
	"golang.org/x/crypto/acme/autocert"
)

// ServerRPC 服务器，这个一个RPC服务器，客户端可以调用绑定的方法。
type ServerRPC struct {
	*rpc.Server                      // RPC
	Addr        string               // 地址
	AutoCert    *autocert.Manager    // 自动申请证书
	l           tcpKeepAliveListener // 监听器
	handled     bool                 // 使用路径
}

// NewServerRPC 服务器监听
func NewServerRPC() *ServerRPC {
	return &ServerRPC{Server: rpc.NewServer()}
}

// Register 注册解析类型
//
//	value any     注册类型
func (T *ServerRPC) Register(value any) {
	gob.Register(value)
}

// RegisterName 注册一个struct，让客户端进行访问。
//
//	name string       包名
//	rcvr any  结构对象
//	error             错误
func (T *ServerRPC) RegisterName(name string, rcvr any) error {
	return T.Server.RegisterName(name, rcvr)
}

// HandleHTTP 设置 RPC地址 和 调试地址
//
//	rpcPath, debugPath string       访问地址和调试地址
func (T *ServerRPC) HandleHTTP(rpcPath, debugPath string) {
	T.Server.HandleHTTP(rpcPath, debugPath)
	T.handled = true
}

// LoadTLS 加载证书文件
//
//	certFile     证书公钥
//	keyFile      证书私钥
func (T *ServerRPC) LoadTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	T.l.tlsconfig = new(tls.Config)
	T.l.tlsconfig.Certificates = append(T.l.tlsconfig.Certificates, cert)
	return nil
}

// ListenAndServe 监听并启动
//
//	error 错误
func (T *ServerRPC) ListenAndServe() error {
	if T.Addr == "" {
		T.Addr = ":http"
	}
	l, err := net.Listen("tcp", T.Addr)
	if err != nil {
		return err
	}
	return T.Serve(l)
}

// Serve 监听客户端连接
//
//	error 错误
func (T *ServerRPC) Serve(l net.Listener) error {
	addr := l.Addr().(*net.TCPAddr)
	ip := addr.IP.To4()
	if ip == nil {
		ip = addr.IP.To16()
	}
	T.Addr = net.JoinHostPort(ip.String(), strconv.Itoa(addr.Port))
	T.l.TCPListener = l.(*net.TCPListener)

	if T.handled {
		hanlde := vweb.AutoCert(T.AutoCert, T.l.tlsconfig, http.Handler(http.HandlerFunc((func(w http.ResponseWriter, r *http.Request) {
			http.DefaultServeMux.ServeHTTP(w, r)
		}))))
		return http.Serve(&T.l, hanlde)
	}
	T.Server.Accept(&T.l)
	return nil
}

// Close 判断监听的连接
//
//	error 错误
func (T *ServerRPC) Close() error {
	if T.l.TCPListener != nil {
		return T.l.Close()
	}
	return nil
}
