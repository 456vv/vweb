package vweb
import (
    "net/rpc"
    "encoding/gob"
    "net"
    "net/http"
    "bufio"
    "errors"
    "io"
 //   "context"
    "fmt"
    "time"
    "github.com/456vv/vconnpool"
)


//rpc插件接口
type PluginRPC interface{
    Register(value interface{})
    Call(name string, arg interface{}) (interface{}, error)
    Discard() error
    Close() error
}

//插件RPC客户端
type PluginRPCClient struct {
    ConnPool            *vconnpool.ConnPool	// 连接池
    Path 				string				// 路径
    Addr				string            	// 地址
}

//快速连接RPC
//	PluginRPC	插件RPC
//	error		错误
func(T *PluginRPCClient) Connection() (PluginRPC, error) {
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

//配置RPC插件客户端
//	c *PluginRPCClient			客户端插件对象，创建时候可以是nil。
//	config ConfigSitePlugin		配置
//	*PluginRPCClient			返回客户端插件对象
//	error						错误
func ConfigPluginRPCClient(c *PluginRPCClient, config ConfigSitePlugin) (*PluginRPCClient, error) {
	return configRPCClient(c, config)
}

//快速的配置RPC
func configRPCClient(c *PluginRPCClient, config ConfigSitePlugin) (*PluginRPCClient, error) {
    netDialer := &net.Dialer{
        Timeout 	: time.Duration(config.Timeout) * time.Millisecond,
        KeepAlive   : time.Duration(config.KeepAlive) * time.Millisecond,
        FallbackDelay: time.Duration(config.FallbackDelay) * time.Millisecond,
        DualStack	: config.DualStack,
    }

	//设置本地拨号地址
	if config.LocalAddr != "" {
		netTCPAddr, err := net.ResolveTCPAddr("tcp", config.LocalAddr)
		if err == nil {
			netDialer.LocalAddr = netTCPAddr
		}else{
   	    	return nil, fmt.Errorf("vweb: 本地 ConfigSitePlugin.LocalAddr 无法解析这个地址(%s)。格式应该是 111.222.444.555:0 或者 www.xxx.com:0", config.LocalAddr)
		}
	}
	
    //RPC客户端连接池
    if c == nil {
    	c = &PluginRPCClient{
    		ConnPool:&vconnpool.ConnPool{},
    	}
    }
    
    c.ConnPool.Dialer = netDialer
    c.ConnPool.IdeConn = config.IdeConn
    c.ConnPool.MaxConn = config.MaxConn
   	c.Path  = config.Path
    c.Addr	= config.Addr
	
	return c, nil
}


//pluginRPC 插件连接RPC
type pluginRPC struct{
    *rpc.Client			// 配置端
    conn	net.Conn
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

