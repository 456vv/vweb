package plugin

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/rpc"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/456vv/vweb/v3"
	"github.com/ugorji/go/codec"
	"golang.org/x/crypto/acme/autocert"
)

const (
	acceptMaxRetry   = 5
	acceptRetryDelay = 5 * time.Millisecond
)

// ServerRPC 服务器，基于 ugorji/go/codec 的 RPC 服务端。
//
// Handle 为编解码配置，必须与客户端一致。为 nil 时使用共享的 MsgpackHandle。
// 若需支持接口字段，请在 Handle 上配置 MapType / SetExt；客户端需相同配置。
type ServerRPC struct {
	*rpc.Server
	Addr     string
	AutoCert *autocert.Manager
	Handle   codec.Handle

	l         tcpKeepAliveListener
	rpcPath   string
	debugPath string
	handled   bool

	handleOnce    sync.Once
	defaultHandle codec.Handle

	mu       sync.RWMutex
	services map[string]any // 供 debug 页展示
}

// NewServerRPC 服务器监听
func NewServerRPC() *ServerRPC {
	return &ServerRPC{
		Server:   rpc.NewServer(),
		services: make(map[string]any),
	}
}

// getHandle 返回编解码配置，默认 handle 只创建一次并被所有连接共享。
// 注意：返回后调用方不应再修改该 handle。
func (T *ServerRPC) getHandle() codec.Handle {
	if T.Handle != nil {
		return T.Handle
	}
	T.handleOnce.Do(func() {
		h := &codec.MsgpackHandle{}
		h.WriterBufferSize = 32 * 1024
		h.ReaderBufferSize = 32 * 1024
		h.MapType = reflect.TypeOf(map[string]any(nil))
		T.defaultHandle = h
	})
	return T.defaultHandle
}

// Register 空实现：ugorji/go/codec 不需要 gob 式类型注册。
func (T *ServerRPC) Register(value any) {
	if T == nil || T.Server == nil || value == nil {
		return
	}
	T.Server.Register(value)
}

// RegisterName 注册服务对象，并记录用于 debug 页展示。
func (T *ServerRPC) RegisterName(name string, rcvr any) error {
	if err := T.Server.RegisterName(name, rcvr); err != nil {
		return err
	}
	T.mu.Lock()
	if T.services == nil {
		T.services = make(map[string]any)
	}
	T.services[name] = rcvr
	T.mu.Unlock()
	return nil
}

// HandleHTTP 开启 HTTP CONNECT 隧道模式并记录路由。
// 客户端必须发送 CONNECT <rpcPath> 才能切换到 RPC 协议。
// debugPath 非空时提供只读的已注册服务列表。
func (T *ServerRPC) HandleHTTP(rpcPath, debugPath string) {
	if rpcPath == "" {
		rpcPath = "/_goRPC_"
	}
	if !strings.HasPrefix(rpcPath, "/") {
		rpcPath = "/" + rpcPath
	}
	if debugPath != "" && !strings.HasPrefix(debugPath, "/") {
		debugPath = "/" + debugPath
	}
	T.rpcPath = rpcPath
	T.debugPath = debugPath
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
	if T.l.tlsconfig == nil {
		T.l.tlsconfig = new(tls.Config)
	}
	T.l.tlsconfig.Certificates = append(slices.Clone(T.l.tlsconfig.Certificates), cert)
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

// Serve 监听客户端连接。
// 不再使用 T.Server.Accept（内部绑定 gob），而是为每个连接创建独立的 codec。
func (T *ServerRPC) Serve(l net.Listener) error {
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

	if T.handled {
		mux := http.NewServeMux()
		mux.HandleFunc(T.rpcPath, T.serveHTTPTunnel)
		if T.debugPath != "" {
			mux.HandleFunc(T.debugPath, T.serveDebug)
		}
		hanlde := vweb.AutoCert(T.AutoCert, T.l.tlsconfig, mux)
		return http.Serve(&T.l, hanlde)
	}

	var acceptErrCount int
	for {
		conn, err := T.l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			var ne net.Error
			// Temporary() 虽已标记 Deprecated，但对 EMFILE 等 syscall 错误
			// 仍是标准库内部唯一可用的分类手段，与 Timeout() 合并判断。
			if errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()) && acceptErrCount < acceptMaxRetry {
				acceptErrCount++
				time.Sleep(acceptRetryDelay * time.Duration(acceptErrCount))
				continue
			}
			return err
		}
		acceptErrCount = 0
		go T.serveConn(conn)
	}
}

// serveDebug 输出已注册服务与方法名，便于运维排查。
func (T *ServerRPC) serveDebug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "RPC services on %s\n\n", T.rpcPath)

	T.mu.RLock()
	type entry struct {
		name string
		rcvr any
	}
	entries := make([]entry, 0, len(T.services))
	for name, rcvr := range T.services {
		entries = append(entries, entry{name: name, rcvr: rcvr})
	}
	T.mu.RUnlock()

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, e := range entries {
		fmt.Fprintf(w, "%s\n", e.name)
		if e.rcvr == nil {
			continue
		}
		t := reflect.TypeOf(e.rcvr)
		for i := 0; i < t.NumMethod(); i++ {
			fmt.Fprintf(w, "  %s.%s\n", e.name, t.Method(i).Name)
		}
	}
}

// serveHTTPTunnel 处理 HTTP CONNECT 隧道。
func (T *ServerRPC) serveHTTPTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "405 must CONNECT\n")
		return
	}

	// 安全断言：某些中间件包装后 ResponseWriter 可能不实现 Hijacker。
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "RPC hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, "RPC hijacking "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.WriteString(conn, "HTTP/1.0 200 Connected to Go RPC\n\n"); err != nil {
		conn.Close()
		return
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return
	}

	T.serveConn(&hijackedConn{Conn: conn, r: bufrw.Reader})
}

// serveConn 为单个连接创建 codec 并进入 RPC 服务循环。
// defer conn.Close 作为防御性兜底；ServeCodec 正常退出时 codec.Close
// 已经关闭连接，二次 Close 幂等无害。
func (T *ServerRPC) serveConn(conn net.Conn) {
	defer conn.Close()
	rpcCodec := codec.GoRpc.ServerCodec(conn, T.getHandle())
	T.Server.ServeCodec(rpcCodec)
}

// Close 关闭监听器。
func (T *ServerRPC) Close() error {
	if T.l.TCPListener != nil {
		return T.l.Close()
	}
	return nil
}

type hijackedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *hijackedConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}
