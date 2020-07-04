package plugin
import(
    "net"
    "net/rpc"
    "encoding/gob"
    "net/http"
)

//ServerRPC 服务器，这个一个RPC服务器，客户端可以调用绑定的方法。
type ServerRPC struct {
    *rpc.Server                                                             // RPC
    Addr        string                                                      // 地址
    l           tcpKeepAliveListener                                        // 监听器
    handled     bool                                                        // 使用路径
}

//NewServerRPC 服务器监听
func NewServerRPC() *ServerRPC {
    return &ServerRPC{Server:rpc.NewServer()}
}

//Register 注册解析类型
//	value interface{}     注册类型
func (T *ServerRPC) Register(value interface{}) {
    gob.Register(value)
}

//RegisterName 注册一个struct，让客户端进行访问。
//	name string       包名
//	rcvr interface{}  结构对象
//	error             错误
func (T *ServerRPC) RegisterName(name string, rcvr interface{}) error {
    return T.Server.RegisterName(name, rcvr)
}


//HandleHTTP 设置 RPC地址 和 调试地址
//	rpcPath, debugPath string       访问地址和调试地址
func (T *ServerRPC) HandleHTTP(rpcPath, debugPath string) {
    T.Server.HandleHTTP(rpcPath, debugPath)
    T.handled = true
}

//ListenAndServe 监听并启动
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

//Serve 监听客户端连接
//	error 错误
func (T *ServerRPC) Serve(l net.Listener) error {
	
    T.Addr = l.Addr().String()
    T.l.TCPListener = l.(*net.TCPListener)
    if T.handled {
        return http.Serve(&T.l, nil)
    }
    T.Server.Accept(&T.l)
    return nil
}

//Close 判断监听的连接
//	error 错误
func (T *ServerRPC) Close() error {
    if T.l.TCPListener != nil {
        return T.l.Close()
    }
    return nil
}