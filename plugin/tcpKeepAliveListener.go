package plugin
import (
	"net"
	"time"
	"crypto/tls"
)
//tcpKeepAliveListener tcp连接保持
type tcpKeepAliveListener struct {
    *net.TCPListener                // TCP监听
    tlsconf	*tls.Config
}

//Accept 接受
//  返：
//      c net.Conn    tcp连接
//      err error     错误
func (T *tcpKeepAliveListener) Accept() (c net.Conn, err error) {
    tc, err := T.TCPListener.AcceptTCP()
    if err != nil {
        return
    }
    tc.SetKeepAlive(true)
    tc.SetKeepAlivePeriod(3 * time.Minute)
    if T.tlsconf != nil {
    	return tls.Server(tc, T.tlsconf), nil
    }
    return tc, nil
}
