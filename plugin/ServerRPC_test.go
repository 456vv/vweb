package plugin

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/rpc"
	"reflect"
	"testing"
	"time"

	"github.com/ugorji/go/codec"
)

type Test struct{}

func (t *Test) OK(arg, result *string) error {
	*result = *arg
	return nil
}

// testHandle 必须与 ServerRPC.getHandle() 的默认配置保持一致。
func testHandle() codec.Handle {
	h := &codec.MsgpackHandle{}
	h.WriterBufferSize = 32 * 1024
	h.ReaderBufferSize = 32 * 1024
	h.MapType = reflect.TypeOf(map[string]interface{}(nil))
	return h
}

type testRW struct {
	r io.Reader
	w io.Writer
	c io.Closer
}

func (x *testRW) Read(p []byte) (int, error)  { return x.r.Read(p) }
func (x *testRW) Write(p []byte) (int, error) { return x.w.Write(p) }
func (x *testRW) Close() error                { return x.c.Close() }

// dialRaw 直接建立使用 ugorji/go/codec 的 RPC 客户端（纯 TCP 模式）。
func dialRaw(t *testing.T, addr string) *rpc.Client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return rpc.NewClientWithCodec(codec.GoRpc.ClientCodec(conn, testHandle()))
}

// dialConnect 通过 HTTP CONNECT 建立使用 ugorji/go/codec 的 RPC 客户端。
func dialConnect(t *testing.T, addr, path string) *rpc.Client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "CONNECT "+path+" HTTP/1.0\n\n"); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if resp.Status != "200 Connected to Go RPC" {
		conn.Close()
		t.Fatalf("unexpected CONNECT status: %s", resp.Status)
	}
	return rpc.NewClientWithCodec(codec.GoRpc.ClientCodec(
		&testRW{r: br, w: conn, c: conn}, testHandle(),
	))
}

// waitTCP 轮询等待目标地址可建立 TCP 连接，最多 3 秒。
func waitTCP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server not reachable: %s", addr)
}

func callOK(t *testing.T, client *rpc.Client) {
	t.Helper()
	arg := "测试"
	result := ""
	if err := client.Call("Test.OK", &arg, &result); err != nil {
		t.Fatalf("远程调用失败：%v", err)
	}
	if arg != result {
		t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
	}
}

// Test_ServerRPC_Serve 使用 Serve 启动（显式 listener），走纯 TCP RPC。
func Test_ServerRPC_Serve(t *testing.T) {
	srpc := NewServerRPC()
	srpc.RegisterName("Test", new(Test))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srpc.Serve(ln)
	defer srpc.Close()

	addr := ln.Addr().String()
	waitTCP(t, addr)

	client := dialRaw(t, addr)
	defer client.Close()
	callOK(t, client)
}

// Test_ServerRPC_HandleHTTP 通过 HTTP CONNECT 隧道连接。
func Test_ServerRPC_HandleHTTP(t *testing.T) {
	srpc := NewServerRPC()
	srpc.Addr = "127.0.0.1:0"
	srpc.HandleHTTP("/_gorpc_", "/_gorpcbug_")
	srpc.RegisterName("Test", new(Test))
	go srpc.ListenAndServe()
	defer srpc.Close()

	deadline := time.Now().Add(3 * time.Second)
	var addr string
	for time.Now().Before(deadline) {
		addr = srpc.Addr
		if addr != "" && addr != "127.0.0.1:0" {
			if c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond); err == nil {
				c.Close()
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if addr == "" || addr == "127.0.0.1:0" {
		t.Fatal("server not ready")
	}

	client := dialConnect(t, addr, "/_gorpc_")
	defer client.Close()
	callOK(t, client)
}

// Test_ServerRPC_HandleHTTP_WrongPath 路径不匹配的 CONNECT 应被拒绝。
// 这验证 rpcPath 参数确实通过 ServeMux 生效，而不是被静默忽略。
func Test_ServerRPC_HandleHTTP_WrongPath(t *testing.T) {
	srpc := NewServerRPC()
	srpc.Addr = "127.0.0.1:0"
	srpc.HandleHTTP("/_gorpc_", "")
	srpc.RegisterName("Test", new(Test))
	go srpc.ListenAndServe()
	defer srpc.Close()

	deadline := time.Now().Add(3 * time.Second)
	var addr string
	for time.Now().Before(deadline) {
		addr = srpc.Addr
		if addr != "" && addr != "127.0.0.1:0" {
			if c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond); err == nil {
				c.Close()
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if addr == "" || addr == "127.0.0.1:0" {
		t.Fatal("server not ready")
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	io.WriteString(conn, "CONNECT /wrong_path HTTP/1.0\n\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "CONNECT"})
	if err != nil {
		t.Fatalf("读响应失败：%v", err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("错误路径 CONNECT 不应返回 200，得到 %s", resp.Status)
	}
}

type rpcNonTCPListener struct{ net.Listener }

func (l *rpcNonTCPListener) Accept() (net.Conn, error) { return l.Listener.Accept() }

func TestServerRPC_Serve_NonTCPListener(t *testing.T) {
	sr := NewServerRPC()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := sr.Serve(&rpcNonTCPListener{Listener: ln}); err == nil {
		t.Fatal("非 TCP listener 应报错")
	}
}

func TestServerRPC_CloseBeforeServe(t *testing.T) {
	sr := NewServerRPC()
	if err := sr.Close(); err != nil {
		t.Fatalf("Close 未 Serve 前应返回 nil，得到 %v", err)
	}
}

func TestServerRPC_RegisterName_Duplicate(t *testing.T) {
	sr := NewServerRPC()
	if err := sr.RegisterName("T", new(Test)); err != nil {
		t.Fatal(err)
	}
	if err := sr.RegisterName("T", new(Test)); err == nil {
		t.Fatal("同名注册应报错")
	}
}

func TestServerRPC_LoadTLS_InvalidFiles(t *testing.T) {
	sr := NewServerRPC()
	if err := sr.LoadTLS("/nonexistent/x.pem", "/nonexistent/y.key"); err == nil {
		t.Fatal("不存在的证书应报错")
	}
}
