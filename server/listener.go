package server
import (
	"net"
    "time"
    "crypto/tls"
)
//listener tcp连接保持
type listener struct {
    *net.TCPListener                	// TCP监听
    cc		*ConfigConn					// 连接配置
    tlsconf	*tls.Config
    closed	bool
}
//Accept 接受
//	c net.Conn    tcp连接
//	err error     错误
func (T *listener) Accept() (c net.Conn, err error) {
    tc, err := T.TCPListener.AcceptTCP()
	
	if err != nil {
		if ne, ok := err.(net.Error); ok {
			T.closed = !ne.Temporary()
		}
		return
	}
	
    if T.cc != nil  {
	    if d := T.cc.Deadline; d != 0 {
	        tc.SetDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
	    }
	    if d := T.cc.WriteDeadline; d != 0 {
	        tc.SetWriteDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
	    }
	    if d := T.cc.ReadDeadline; d != 0 {
	        tc.SetReadDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
	    }
	    if d := T.cc.KeepAlivePeriod; d != 0 {
	        tc.SetKeepAlivePeriod(time.Duration(d) * time.Millisecond)
	    }
	    if d := T.cc.ReadBuffer; d != 0 {
	    	tc.SetReadBuffer(d)
	    }
	    if d := T.cc.WriteBuffer; d != 0 {
	    	tc.SetWriteBuffer(d)
	    }
	    if d := T.cc.Linger; d != 0 {
	    	tc.SetLinger(T.cc.Linger)
	    }
	    tc.SetKeepAlive(T.cc.KeepAlive)
	    tc.SetNoDelay(T.cc.NoDelay)
    }
    
    if T.tlsconf != nil {
    	return tls.Server(tc, T.tlsconf), nil
    }
    return tc, nil
    
}
