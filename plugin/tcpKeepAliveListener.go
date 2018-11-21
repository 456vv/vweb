package plugin
import (
	"net"
	"time"
)
//tcpKeepAliveListener tcp连接保持
type tcpKeepAliveListener struct {
    *net.TCPListener                // TCP监听
}

//Accept 接受
//  返：
//      c net.Conn    tcp连接
//      err error     错误
func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
    tc, err := ln.AcceptTCP()
    if err != nil {
        return
    }
    tc.SetKeepAlive(true)
    tc.SetKeepAlivePeriod(3 * time.Minute)
    return tc, nil
}
