package plugin
import(
	"testing"
    "net/rpc"
    "net"
    "time"

)
type Test struct {}
func (t *Test) OK(arg, result *string) error {
    *result = *arg
    return nil
}


func Test_ServerRPC_1(t *testing.T){
    //服务器监听
    var srpc = NewServerRPC()
    srpc.RegisterName("Test", new(Test))
    netListener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
    	t.Fatal(err)
    }

    go srpc.Serve(netListener)
    defer srpc.Close()

	time.Sleep(time.Second * 2)

    //客户端
    client, err := rpc.Dial("tcp", srpc.Addr)
    if err != nil {
        t.Fatal("客户端无法建立")
    }
    var arg = "测试"
    var result  = ""
    err = client.Call("Test.OK", &arg, &result)
    if err != nil {
        t.Fatal("客户端无法远程调用函数")
    }
    if arg != result {
        t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
    }
}

func Test_ServerRPC_2(t *testing.T){
    //服务器监听
    var listen, err = net.Listen("tcp", "127.0.0.1:50000")
    if err != nil {
        t.Fatalf("地址监听失败，错误：%v", err)
    }
    var srpc = NewServerRPC()
    srpc.RegisterName("Test", new(Test))
    go srpc.Serve(listen)
    defer srpc.Close()

	time.Sleep(time.Second * 2)

    //客户端
    client, err := rpc.Dial("tcp", srpc.Addr)
    if err != nil {
        t.Fatal("客户端无法建立")
    }
    var arg = "测试"
    var result  = ""
    err = client.Call("Test.OK", &arg, &result)
    if err != nil {
        t.Fatal("客户端无法远程调用函数")
    }
    if arg != result {
        t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
    }
}

func Test_ServerRPC_3(t *testing.T){
    //服务器监听
    var srpc = NewServerRPC()
    srpc.Addr = "127.0.0.1:0"
    srpc.RegisterName("Test", new(Test))
    go srpc.ListenAndServe()
    defer srpc.Close()

	time.Sleep(time.Second * 2)

    //客户端
    client, err := rpc.Dial("tcp", srpc.Addr)
    if err != nil {
        t.Fatal("客户端无法建立")
    }
    var arg = "测试"
    var result  = ""
    err = client.Call("Test.OK", &arg, &result)
    if err != nil {
        t.Fatal("客户端无法远程调用函数")
    }
    if arg != result {
        t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
    }
}

func Test_ServerRPC_4(t *testing.T){
    //服务器监听
    var srpc = NewServerRPC()
    srpc.Addr = "127.0.0.1:0"
    srpc.HandleHTTP("_gorpc_", "_gorpcbug_")
    srpc.RegisterName("Test", new(Test))
    go srpc.ListenAndServe()
    defer srpc.Close()
	time.Sleep(time.Second * 2)

    //客户端
    client, err := rpc.DialHTTPPath("tcp", srpc.Addr, "_gorpc_")
    if err != nil {
        t.Fatal("客户端无法建立")
    }
    var arg = "测试"
    var result  = ""
    err = client.Call("Test.OK", &arg, &result)
    if err != nil {
        t.Fatal("客户端无法远程调用函数")
    }
    if arg != result {
        t.Fatalf("\r\n本地：%v\r\n远程：%v", arg, result)
    }
}

func TestServerRPC_Print(t *testing.T) {
    var p = &Plugin{
        Type: PluginTypeRPC,
        Version: 1.0,
        Name: "bw",
    }
    //服务器监听
    var srpc = NewServerRPC()
    srpc.Addr = "127.0.0.1:0"
    defer srpc.Close()
    srpc.RegisterName("Test", new(Test))
    var addr = p.Addr
    srpc.AutoFill(p)

    if addr == p.Addr {
        t.Fatalf("\r\n前：%v\r\n后：%v", addr, p.Addr)
    }
}














