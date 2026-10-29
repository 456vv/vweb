package vweb

import (
	"errors"
	"net"
	"net/http"
	"net/rpc"
	"sync/atomic"
	"testing"
	"time"

	"github.com/456vv/vconnpool/v3"
)

type echoService struct{}

func (e *echoService) Echo(arg *string, reply *string) error {
	*reply = *arg
	return nil
}
func (e *echoService) Fail(arg *string, reply *string) error { return errors.New("boom") }

func TestPluginRPCClient_NilPool(t *testing.T) {
	c := &PluginRPCClient{}
	if _, err := c.Connection(); err == nil {
		t.Fatal("ConnPool 为 nil 应报错")
	}
}

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
	srv := &http.Server{Handler: rpcSrv}
	go srv.Serve(ln)
	return ln.Addr().String(), func() { srv.Close() }
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

func TestPluginRPC_ConnectionUnexpectedStatus(t *testing.T) {
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
		t.Fatal("非 200 Connected 应报错")
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

// TestPluginRPC_Boundary 非 200 CONNECT 拒绝。
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

func TestPluginRPC_Discard_NilConn(t *testing.T) {
	// conn 为 nil 时，类型断言失败，Discard 返回 nil（不 panic）
	p := &pluginRPC{}
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
