package vweb
import (
    "net/rpc"
    "encoding/gob"
    "net"
    "net/http"
    "bufio"
    "errors"
    "io"
    "fmt"
    "github.com/456vv/vconnpool/v2"
    "github.com/456vv/verror"
)


//rpc插件接口
type PluginRPC interface{
    Type() PluginType																								// 类型
    Register(value interface{})																						// 注册struct类型
    Call(name string, arg interface{}) (interface{}, error)															// 调用
    Discard() error																									// 废弃连接
    Close() error																									// 关闭
}

//插件RPC客户端
type PluginRPCClient struct {
    ConnPool	*vconnpool.ConnPool	// 连接池
    Addr		string				// 地址
    Path		string				// 路径
}

//快速连接RPC
//	PluginRPC			插件RPC
//	error				错误
func(T *PluginRPCClient) Connection() (PluginRPC, error) {
	if T.ConnPool == nil {
		return nil, verror.TrackError("vweb: ConnPool字段不可以为空！")
	}
    //RPC客户端连接
    conn, err := T.ConnPool.Dial("tcp", T.Addr)
    if err != nil {
    	return nil, err
    }

	//RPC客户端准备
	var client *rpc.Client
	if conn, ok := conn.(vconnpool.Conn); ok && conn.IsReuseConn() {
		//重复连接不需要做连接前准备
		client = rpc.NewClient(conn)
	}else{
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

//pluginRPC 插件连接RPC
type pluginRPC struct{
    *rpc.Client			// 配置端
    conn	net.Conn
}

//插件类型
//	PluginType 插件类型
func (T *pluginRPC) Type() PluginType {
	return PluginTypeRPC
}
//Register RPC注册类型，仅用于RPC客户端。默认gob编码
//	value interface{}     注册类型
func (prpc *pluginRPC) Register(value interface{}){
    gob.Register(value)
}

//Call 调用RPC，连接TCP，等待远程返回数据。
//	name string           远程函数名，格式如：admin.Add 。有关于rpc调用知识，请阅读官方标准库 net/rpc
//	arg interface{}       参数，发送至远程的参数
//	*Map, error           结果，远程返回来的结果
func (prpc *pluginRPC) Call(name string, arg interface{}) (interface{}, error) {
	
    //调用RPC函数
    var result interface{}
    err := prpc.Client.Call(name, arg, result)
    if err != nil {
        return nil, err
    }
    return result, nil
}

//Close 关闭RPC连接
//  error     错误
func (prpc *pluginRPC) Close() error {
    return prpc.Client.Close()
}

//Discard 废弃, RPC这条连接不再回收
//  error     错误
func (prpc *pluginRPC) Discard() error {
	conn, ok := prpc.conn.(vconnpool.Conn)
	if ok {
		return fmt.Errorf("vweb: Discard 方法不存在！")
	}
	return conn.Discard()
}
