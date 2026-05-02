package vweb

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/rpc"

	"github.com/456vv/vconnpool/v2"
)

// PluginRPC rpc插件接口
type PluginRPC interface {
	Type() PluginType                       // 类型
	Register(value any)                     // 注册struct类型
	Call(name string, arg any) (any, error) // 调用
	Discard() error                         // 废弃连接
	Close() error                           // 关闭
}

// PluginRPCClient 插件RPC客户端
type PluginRPCClient struct {
	ConnPool *vconnpool.ConnPool // 连接池
	Addr     string              // 地址
	Path     string              // 路径
}

// Connection 快速连接RPC
//
//	PluginRPC			插件RPC
//	error				错误
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
	if conn, ok := conn.(vconnpool.Conn); ok && conn.IsReuseConn() {
		// 重复连接不需要做连接前准备
		client = rpc.NewClient(conn)
	} else {
		client, err = connentRPCClient(conn, T.Path)
		if err != nil {
			return nil, err
		}
	}
	return &pluginRPC{Client: client, conn: conn}, nil
}

func connentRPCClient(conn net.Conn, p string) (*rpc.Client, error) {
	io.WriteString(conn, "CONNECT "+p+" HTTP/1.0\n\n")

	// 需要成功的HTTP响应
	// 切换到RPC协议之前。
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && resp.Status == "200 Connected to Go RPC" {
		return rpc.NewClient(conn), nil
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	addr := conn.RemoteAddr()
	conn.Close()
	return nil, &net.OpError{
		Op:   "dial-http",
		Net:  addr.Network() + " " + addr.String(),
		Addr: nil,
		Err:  err,
	}
}

// pluginRPC 插件连接RPC
type pluginRPC struct {
	*rpc.Client // 配置端
	conn        net.Conn
}

// 插件类型
//
//	PluginType 插件类型
func (T *pluginRPC) Type() PluginType {
	return PluginTypeRPC
}

// Register RPC注册类型，仅用于RPC客户端。默认gob编码
//
//	value any     注册类型
func (T *pluginRPC) Register(value any) {
	gob.Register(value)
}

// Call 调用RPC，连接TCP，等待远程返回数据。
//
//	name string           远程函数名，格式如：admin.Add 。有关于rpc调用知识，请阅读官方标准库 net/rpc
//	arg any       参数，发送至远程的参数
//	*Map, error           结果，远程返回来的结果
func (T *pluginRPC) Call(name string, arg any) (any, error) {
	// 调用RPC函数
	var result any
	err := T.Client.Call(name, arg, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Close 关闭RPC连接
//
//	error     错误
func (T *pluginRPC) Close() error {
	return T.Client.Close()
}

// Discard 废弃, RPC这条连接不再回收
//
//	error     错误
func (T *pluginRPC) Discard() error {
	conn, ok := T.conn.(vconnpool.Conn)
	if ok {
		return fmt.Errorf("vweb: Discard 方法不存在！")
	}
	return conn.Discard()
}
