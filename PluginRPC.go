package vweb

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/rpc"
	"reflect"
	"strings"
	"sync"

	"github.com/456vv/vconnpool/v3"
	"github.com/ugorji/go/codec"
)

const defaultRPCPath = "/_goRPC_"

// PluginRPC rpc 插件接口。
// 用于与后端 RPC 服务通信：注册类型、发起调用、丢弃或关闭连接。
//
// 典型流程：
//
//	client := &PluginRPCClient{ConnPool: pool, Addr: "127.0.0.1:9000", Path: "/_goRPC_"}
//	rpc, err := client.Connection()
//	rpc.Register(MyType{})
//	err = rpc.Call("Service.Method", arg, &reply)
//	rpc.Close() // 或 Discard() 不回收连接
type PluginRPC interface {
	Type() PluginType                           // 类型，固定返回 PluginTypeRPC
	Register(value any)                         // 注册 struct 类型（gob）
	Call(name string, arg any, reply any) error // 远程调用
	Discard() error                             // 废弃连接（不回池）
	Close() error                               // 关闭并回池（若支持）
}

// PluginRPCClient 插件 RPC 客户端配置。
//
// Handle 为编解码配置；为 nil 时使用共享的 MsgpackHandle（32KB 读写缓冲）。
// 若需支持接口类型字段，请在 Handle 上配置 MapType 或 SetExt；
// 服务端必须使用相同的 Handle 配置。
type PluginRPCClient struct {
	ConnPool *vconnpool.Pool
	Addr     string
	Path     string
	Handle   codec.Handle

	handleOnce    sync.Once
	defaultHandle codec.Handle
}

// getHandle 返回编解码配置。默认 handle 只创建一次并被所有连接共享；
// codec.MsgpackHandle 是只读配置对象，可被多个 Encoder/Decoder 并发共享。
// 注意：一旦返回，调用方不应再修改该 handle，否则会造成数据竞争。
func (T *PluginRPCClient) getHandle() codec.Handle {
	if T.Handle != nil {
		return T.Handle
	}
	T.handleOnce.Do(func() {
		h := &codec.MsgpackHandle{}
		// net.Dial 返回的连接不带缓冲，通过 handle 显式开启缓冲。
		h.WriterBufferSize = 32 * 1024
		h.ReaderBufferSize = 32 * 1024
		h.MapType = reflect.TypeOf(map[string]any(nil))
		T.defaultHandle = h
	})
	return T.defaultHandle
}

// rpcPath 返回可用于 CONNECT 的目标路径（空值补默认、缺前导斜杠补齐）。
func (T *PluginRPCClient) rpcPath() string {
	if T.Path == "" {
		return defaultRPCPath
	}
	if !strings.HasPrefix(T.Path, "/") {
		return "/" + T.Path
	}
	return T.Path
}

// newClient 基于给定连接创建 RPC 客户端。
//
// 复用连接：已经处于 RPC 协议状态，跳过 CONNECT 握手。
// 每次调用 codec.GoRpc.ClientCodec 都会创建独立的 Encoder/Decoder 实例，
// 因此编解码状态（类型缓存等）不会跨会话残留。
// 注意：这是 ugorji/go/codec 的文档约定（Encoder/Decoder 的状态由实例持有，
// 不依赖跨会话重置），与 gob 的连接级类型状态有本质区别。
func (T *PluginRPCClient) newClient(conn net.Conn) (*rpc.Client, error) {
	if c, ok := conn.(vconnpool.Conn); ok && c.IsReuseConn() {
		return rpc.NewClientWithCodec(codec.GoRpc.ClientCodec(conn, T.getHandle())), nil
	}
	return connectRPCClientWithCodec(conn, T.rpcPath(), T.getHandle())
}

// connectRPCClientWithCodec 建立 HTTP CONNECT 后切换到基于 codec 的 Go RPC。
func connectRPCClientWithCodec(conn net.Conn, p string, h codec.Handle) (*rpc.Client, error) {
	if _, err := io.WriteString(conn, "CONNECT "+p+" HTTP/1.0\n\n"); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err == nil {
		if resp.Status == "200 Connected to Go RPC" {
			rc := conn
			if br.Buffered() > 0 { // 有预读就必须把它接到编解码器前面
				rc = &bufferedConn{Conn: conn, r: br}
			}
			return rpc.NewClientWithCodec(codec.GoRpc.ClientCodec(rc, h)), nil
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	addr := conn.RemoteAddr()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  addr.Network() + " " + addr.String(),
		Addr: nil,
		Err:  err,
	}
}

// Connection 从连接池拨号并建立基于 ugorji/go/codec 的 RPC 客户端。
//
// 返回：
//   - PluginRPC: 可用的 RPC 句柄
//   - error: ConnPool 为空、拨号失败、CONNECT 握手失败等
//
// 示例：
//
//	prpc, err := client.Connection()
//	if err != nil { return err }
//	defer prpc.Close()
func (T *PluginRPCClient) Connection() (PluginRPC, error) {
	if T.ConnPool == nil {
		return nil, errors.New("vweb: ConnPool字段不可以为空！")
	}

	conn, err := T.ConnPool.Dial("tcp", T.Addr)
	if err != nil {
		return nil, err
	}

	client, err := T.newClient(conn)
	if err != nil {
		// 先 Discard 标记不回池，再 Close 释放底层 fd
		conn.Discard()
		conn.Close()
		return nil, err
	}

	return &pluginRPC{Client: client, conn: conn}, nil
}

// pluginRPC 插件连接 RPC 的具体实现。
// 一个实例对应一次 Connection 生命周期；rpc.Client 本身并发安全，
// 实例可被多 goroutine 并发 Call，因此 err 标记需要加锁。
type pluginRPC struct {
	*rpc.Client
	conn net.Conn
	mu   sync.Mutex
	err  error // 首次 Call 失败后保留，用于 Close 时决定是否 Discard
}

// Type 返回插件类型 PluginTypeRPC。
func (T *pluginRPC) Type() PluginType {
	return PluginTypeRPC
}

// Register 空实现：ugorji/go/codec 通过反射编解码，不需要 gob 式类型注册。
// 若需支持接口字段，请在 PluginRPCClient.Handle 上配置 MapType / SetExt。
func (T *pluginRPC) Register(value any) {}

// Call 发起远程调用。任一次失败即标记连接不可回池，
// 后续成功调用不会清除该标记，避免把状态可疑的连接归还到池中。
func (T *pluginRPC) Call(name string, arg any, reply any) error {
	err := T.Client.Call(name, arg, reply)
	if err != nil {
		if _, ok := errors.AsType[rpc.ServerError](err); !ok {
			// 非应用层错误：连接可能已损坏，标记后 Close 时 Discard
			T.mu.Lock()
			if T.err == nil {
				T.err = err
			}
			T.mu.Unlock()
		}
	}
	return err
}

// Close 正常归还连接；若曾发生 Call 失败，则改为 Discard。
func (T *pluginRPC) Close() error {
	T.mu.Lock()
	hasErr := T.err != nil
	T.mu.Unlock()
	if hasErr {
		return T.Discard()
	}
	return T.Client.Close()
}

// Discard 废弃连接，不回池。
//
// 顺序说明：先标记 Discard，再 Client.Close。
// - Client.Close 会通过 codec.Close 关闭底层 conn；
// - conn.Close 看到 Discard 标记后执行真正关闭 fd，而不是归还到池；
// - 这样也终止了 rpc.Client 内部阻塞读的读协程，避免 goroutine 泄漏。
func (T *pluginRPC) Discard() error {
	if c, ok := T.conn.(vconnpool.Conn); ok {
		c.Discard()
	}
	return T.Client.Close()
}

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
