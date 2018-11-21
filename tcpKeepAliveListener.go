package vweb
import (
	"net"
    "time"

)
//tcpKeepAliveListener tcp连接保持
type tcpKeepAliveListener struct {
    *net.TCPListener                	// TCP监听
    cc	*ConfigConn						// 连接配置
}
//Accept 接受
//	c net.Conn    tcp连接
//	err error     错误
func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
    tc, err := ln.AcceptTCP()
    if err != nil {
        return
    }
    if d := ln.cc.Deadline; d != 0 {
        tc.SetDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
    }
    if d := ln.cc.WriteDeadline; d != 0 {
        tc.SetWriteDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
    }
    if d := ln.cc.ReadDeadline; d != 0 {
        tc.SetReadDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
    }
    if d := ln.cc.KeepAlivePeriod; d != 0 {
        tc.SetKeepAlivePeriod(time.Duration(d) * time.Millisecond)
    }

    tc.SetKeepAlive(ln.cc.KeepAlive)
    tc.SetLinger(ln.cc.Linger)
    tc.SetNoDelay(ln.cc.NoDelay)
    tc.SetReadBuffer(ln.cc.ReadBuffer)
    tc.SetWriteBuffer(ln.cc.WriteBuffer)
    return tc, nil
}

//Close关闭监听，会调用服务集的关闭函数
//	error	错误
func (ln tcpKeepAliveListener) Close() error {
	return ln.TCPListener.Close()
}


