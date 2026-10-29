package plugin

import (
	"net"
	"net/rpc"
	"testing"
	"time"
)

type Test struct{}

func (t *Test) OK(arg, result *string) error {
	*result = *arg
	return nil
}

func Test_ServerRPC_1(t *testing.T) {
	srpc := NewServerRPC()
	srpc.RegisterName("Test", new(Test))
	netListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srpc.Serve(netListener)
	defer srpc.Close()

	time.Sleep(time.Second * 2)

	client, err := rpc.Dial("tcp", srpc.Addr)
	if err != nil {
		t.Fatal("客户端无法建立")
	}
	arg := "测试"
	result := ""
	err = client.Call("Test.OK", &arg, &result)
	if err != nil {
		t.Fatal("客户端无法远程调用函数")
	}
	if arg != result {
		t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
	}
}

func Test_ServerRPC_2(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:50000")
	if err != nil {
		t.Fatalf("地址监听失败，错误：%v", err)
	}
	srpc := NewServerRPC()
	srpc.RegisterName("Test", new(Test))
	go srpc.Serve(listen)
	defer srpc.Close()

	time.Sleep(time.Second * 2)

	client, err := rpc.Dial("tcp", srpc.Addr)
	if err != nil {
		t.Fatal("客户端无法建立")
	}
	arg := "测试"
	result := ""
	err = client.Call("Test.OK", &arg, &result)
	if err != nil {
		t.Fatal("客户端无法远程调用函数")
	}
	if arg != result {
		t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
	}
}

func Test_ServerRPC_3(t *testing.T) {
	srpc := NewServerRPC()
	srpc.Addr = "127.0.0.1:0"
	srpc.RegisterName("Test", new(Test))
	go srpc.ListenAndServe()
	defer srpc.Close()

	time.Sleep(time.Second * 2)

	client, err := rpc.Dial("tcp", srpc.Addr)
	if err != nil {
		t.Fatal("客户端无法建立")
	}
	arg := "测试"
	result := ""
	err = client.Call("Test.OK", &arg, &result)
	if err != nil {
		t.Fatal("客户端无法远程调用函数")
	}
	if arg != result {
		t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
	}
}

func Test_ServerRPC_4(t *testing.T) {
	srpc := NewServerRPC()
	srpc.Addr = "127.0.0.1:0"
	srpc.HandleHTTP("/_gorpc_", "/_gorpcbug_")
	srpc.RegisterName("Test", new(Test))
	go srpc.ListenAndServe()
	defer srpc.Close()
	time.Sleep(time.Second * 2)

	client, err := rpc.DialHTTPPath("tcp", srpc.Addr, "/_gorpc_")
	if err != nil {
		t.Fatal("客户端无法建立")
	}
	arg := "测试"
	result := ""
	err = client.Call("Test.OK", &arg, &result)
	if err != nil {
		t.Fatal("客户端无法远程调用函数")
	}
	if arg != result {
		t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
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

// TestPluginServer_NonTCP_RPC 非 TCP listener 拒绝。
func TestPluginServer_NonTCP_RPC(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := NewServerRPC().Serve(&rpcNonTCPListener{Listener: ln}); err == nil {
		t.Fatal("非 TCP 应报错")
	}
}
