package vweb

import (
	"bufio"
	"encoding/gob"
	"errors"
	"io"
	"net"
	"net/http"
	"net/rpc"

	"github.com/456vv/vconnpool/v3"
)

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
// ConnPool 负责连接复用；Addr 为 TCP 地址；Path 为 HTTP CONNECT 路径。
type PluginRPCClient struct {
	ConnPool *vconnpool.Pool // 连接池（不可为空）
	Addr     string          // 如 "127.0.0.1:9000"
	Path     string          // 如 "/_goRPC_"
}

// Connection 从连接池拨号并建立 Go net/rpc 客户端。
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
	// RPC客户端连接
	conn, err := T.ConnPool.Dial("tcp", T.Addr)
	if err != nil {
		return nil, err
	}

	// RPC客户端准备
	var client *rpc.Client
	if conn.IsReuseConn() {
		// 重复连接不需要做连接前准备
		client = rpc.NewClient(conn)
	} else {
		client, err = connectRPCClient(conn, T.Path)
		if err != nil {
			// 连接失败时确保底层连接被关闭，避免池中残留半开连接
			conn.Close()
			return nil, err
		}
	}
	return &pluginRPC{Client: client, conn: conn}, nil
}

// connectRPCClient 建立 HTTP CONNECT 后切换到 Go RPC。
// 发送 "CONNECT <path> HTTP/1.0"，期望 "200 Connected to Go RPC"。
func connectRPCClient(conn net.Conn, p string) (*rpc.Client, error) {
	if _, err := io.WriteString(conn, "CONNECT "+p+" HTTP/1.0\r\n\r\n"); err != nil {
		return nil, err
	}

	// 需要成功的 HTTP 响应，再切换到 RPC 协议
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == "200 Connected to Go RPC" {
		return rpc.NewClient(conn), nil
	}
	if err == nil {
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

// pluginRPC 插件连接 RPC 的具体实现。
type pluginRPC struct {
	*rpc.Client // 内嵌标准 RPC 客户端
	conn        net.Conn
}

// Type 返回插件类型 PluginTypeRPC。
func (T *pluginRPC) Type() PluginType {
	return PluginTypeRPC
}

// Register RPC 注册类型，仅用于 RPC 客户端。默认 gob 编码。
// 示例：rpc.Register(MyStruct{})
func (T *pluginRPC) Register(value any) {
	gob.Register(value)
}

// Call 发起远程过程调用。
//
// 参数：
//   - name: "Service.Method"
//   - arg: 入参
//   - reply: 出参指针
//
// 返回：网络错误、服务端错误等
func (T *pluginRPC) Call(name string, arg any, reply any) error {
	return T.Client.Call(name, arg, reply)
}

// Close 关闭 RPC 连接（通常归还连接池）。
func (T *pluginRPC) Close() error {
	return T.Client.Close()
}

// Discard 废弃本连接，不再回收到池中（例如连接状态异常时）。
func (T *pluginRPC) Discard() error {
	if conn, ok := T.conn.(vconnpool.Conn); ok {
		conn.Discard()
	}
	return nil
}
