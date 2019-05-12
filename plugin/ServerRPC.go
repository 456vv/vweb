package plugin
import(
    "net"
    "net/rpc"
    "encoding/gob"
    "net/http"
)

//ServerRPC 服务器，这个一个RPC服务器，客户端可以调用绑定的方法。
type ServerRPC struct {
    Addr        string                                                      // 地址
    *rpc.Server                                                             // RPC
    L           net.Listener                                                // 监听器
    handled     bool                                                        // 使用路径
}

//NewServerRPC 服务器监听
func NewServerRPC() *ServerRPC {
    return &ServerRPC{Server:rpc.NewServer()}
}

//Register 注册解析类型
//  参：
//      value interface{}     注册类型
func (srpc *ServerRPC) Register(value interface{}) {
    gob.Register(value)
}

//RegisterName 注册一个struct，让客户端进行访问。
//  参：
//      name string       包名
//      rcvr interface{}  结构对象
//  返：
//      error             错误
func (srpc *ServerRPC) RegisterName(name string, rcvr interface{}) error {
    return srpc.Server.RegisterName(name, rcvr)
}


//HandleHTTP 设置 RPC地址 和 调试地址
//  参：
//      rpcPath, debugPath string       访问地址和调试地址
func (srpc *ServerRPC) HandleHTTP(rpcPath, debugPath string) {
    srpc.Server.HandleHTTP(rpcPath, debugPath)
    srpc.handled = true
}

//ListenAndServe 监听并启动
//  返：
//      error 错误
func (srpc *ServerRPC) ListenAndServe() error {
    addr := srpc.Addr
    if addr == "" {
        addr = ":http"
    }
    ln, err := net.Listen("tcp", addr)
    if err != nil {
        return err
    }
    srpc.L = tcpKeepAliveListener{ln.(*net.TCPListener)}
    return srpc.Serve(srpc.L)
}


//Serve 监听客户端连接
//  参：
//      error 错误
func (srpc *ServerRPC) Serve(l net.Listener) error {
    srpc.Addr = l.Addr().String()
    srpc.L = l
    if srpc.handled {
        return http.Serve(l, nil)
    }
    srpc.Server.Accept(l)
    return nil
}

//Close 判断监听的连接
//  参：
//      error 错误
func (srpc *ServerRPC) Close() error {
    if srpc.L != nil {
        return srpc.L.Close()
    }
    return nil
}