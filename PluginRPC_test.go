package vweb

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/rpc"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/456vv/vconnpool/v3"
	"github.com/ugorji/go/codec"
)

type echoService struct{}

func (e *echoService) Echo(arg *string, reply *string) error {
	*reply = *arg
	return nil
}
func (e *echoService) Fail(arg *string, reply *string) error { return errors.New("boom") }

// testHandle 必须与 PluginRPCClient.getHandle() 的默认配置保持一致。
func testHandle() codec.Handle {
	h := &codec.MsgpackHandle{}
	h.WriterBufferSize = 32 * 1024
	h.ReaderBufferSize = 32 * 1024
	h.MapType = reflect.TypeOf(map[string]interface{}(nil))
	return h
}

// combinedRW 组合 bufio.Reader 与底层 conn，用于 CONNECT 握手后继续按 RPC 协议读写。
type combinedRW struct {
	r io.Reader
	w io.Writer
	c io.Closer
}

func (x *combinedRW) Read(p []byte) (int, error)  { return x.r.Read(p) }
func (x *combinedRW) Write(p []byte) (int, error) { return x.w.Write(p) }
func (x *combinedRW) Close() error                { return x.c.Close() }

// startRPCService 启动与 plugin.ServerRPC 兼容的最小测试服务端：
// 接受 CONNECT <path> HTTP/1.0，回 200 Connected to Go RPC，
// 然后基于 ugorji/go/codec 切换到 RPC 协议。
func startRPCService(t *testing.T) (string, func()) {
	t.Helper()
	rpcSrv := rpc.NewServer()
	if err := rpcSrv.RegisterName("Echo", new(echoService)); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveTestConn(conn, rpcSrv)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func serveTestConn(c net.Conn, srv *rpc.Server) {
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "CONNECT ") {
		c.Close()
		return
	}
	// 读掉剩余的请求头空行
	for {
		l, err := br.ReadString('\n')
		if err != nil || l == "\n" || l == "\r\n" {
			break
		}
	}
	if _, err := c.Write([]byte("HTTP/1.0 200 Connected to Go RPC\n\n")); err != nil {
		c.Close()
		return
	}
	rwc := &combinedRW{r: br, w: c, c: c}
	srv.ServeCodec(codec.GoRpc.ServerCodec(rwc, testHandle()))
}

func TestPluginRPCClient_NilPool(t *testing.T) {
	c := &PluginRPCClient{}
	if _, err := c.Connection(); err == nil {
		t.Fatal("ConnPool 为 nil 应报错")
	}
}

func TestPluginRPC_EndToEnd(t *testing.T) {
	addr, stop := startRPCService(t)
	defer stop()

	c := &PluginRPCClient{ConnPool: new(vconnpool.Pool), Addr: addr, Path: "/_rpc"}
	prpc, err := c.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer prpc.Close()
	if prpc.Type() != PluginTypeRPC {
		t.Fatalf("Type=%v", prpc.Type())
	}
	var replay string
	err = prpc.Call("Echo.Echo", "world", &replay)
	if err != nil || replay != "world" {
		t.Fatalf("Call=%v err=%v", replay, err)
	}
	if err = prpc.Call("Echo.Fail", "x", &replay); err == nil {
		t.Fatal("应返回远程错误")
	}
}

// TestPluginRPC_ServerErrorKeepsConnReusable 验证业务错误（rpc.ServerError）
// 不会被当作“连接损坏”，后续 Call 仍能继续使用同一条连接。
func TestPluginRPC_ServerErrorKeepsConnReusable(t *testing.T) {
	addr, stop := startRPCService(t)
	defer stop()

	c := &PluginRPCClient{ConnPool: new(vconnpool.Pool), Addr: addr, Path: "/_rpc"}
	prpc, err := c.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer prpc.Close()

	var replay string
	if err = prpc.Call("Echo.Fail", "x", &replay); err == nil {
		t.Fatal("应返回远程错误")
	}
	// 业务错误后仍应能继续调用，且 Close 走正常归还路径。
	if err = prpc.Call("Echo.Echo", "ok", &replay); err != nil || replay != "ok" {
		t.Fatalf("业务错误后连接应可继续使用：err=%v reply=%v", err, replay)
	}
}

func TestPluginRPC_Concurrent(t *testing.T) {
	addr, stop := startRPCService(t)
	defer stop()
	var succ atomic.Int64
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			c := &PluginRPCClient{ConnPool: new(vconnpool.Pool), Addr: addr, Path: "/_rpc"}
			prpc, err := c.Connection()
			if err != nil {
				return
			}
			defer prpc.Close()
			var replay string
			if err := prpc.Call("Echo.Echo", "x", &replay); err == nil {
				succ.Add(1)
			}
		}()
	}
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("超时")
		}
	}
	if succ.Load() == 0 {
		t.Fatal("并发 Call 全部失败")
	}
}

func TestPluginRPC_Boundary(t *testing.T) {
	t.Run("非 200 CONNECT 报错", func(t *testing.T) {
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		defer ln.Close()
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			buf := make([]byte, 512)
			c.Read(buf)
			c.Write([]byte("HTTP/1.0 403 Forbidden\r\n\r\n"))
		}()
		c := &PluginRPCClient{ConnPool: new(vconnpool.Pool), Addr: ln.Addr().String(), Path: "/x"}
		if _, err := c.Connection(); err == nil {
			t.Fatal("非 200 应报错")
		}
	})
}

// TestPluginRPC_Discard_NonPoolConn conn 不是 vconnpool.Conn 时，
// Discard 只关闭 rpc.Client（不调用 conn.Discard），不应 panic。
func TestPluginRPC_Discard_NonPoolConn(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()
	client := rpc.NewClientWithCodec(codec.GoRpc.ClientCodec(c1, testHandle()))
	p := &pluginRPC{Client: client, conn: c1}
	if err := p.Discard(); err != nil {
		t.Fatalf("Discard 应返回 nil，得到 %v", err)
	}
}

func TestPluginRPC_Type(t *testing.T) {
	p := &pluginRPC{}
	if p.Type() != PluginTypeRPC {
		t.Fatalf("Type=%v", p.Type())
	}
}
