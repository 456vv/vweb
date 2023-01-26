package server

import (
	"crypto/tls"
	"net"
	"time"
)

// tcp连接保持
type listener struct {
	*net.TCPListener // TCP监听
	server           *Server
	closed           bool
}

// 接受
//
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

	if T.server.cConn != nil {
		if d := T.server.cConn.Deadline; d != 0 {
			tc.SetDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.server.cConn.WriteDeadline; d != 0 {
			tc.SetWriteDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.server.cConn.ReadDeadline; d != 0 {
			tc.SetReadDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.server.cConn.KeepAlivePeriod; d != 0 {
			tc.SetKeepAlivePeriod(time.Duration(d) * time.Millisecond)
		}
		if d := T.server.cConn.ReadBuffer; d != 0 {
			tc.SetReadBuffer(d)
		}
		if d := T.server.cConn.WriteBuffer; d != 0 {
			tc.SetWriteBuffer(d)
		}
		if d := T.server.cConn.Linger; d != 0 {
			tc.SetLinger(T.server.cConn.Linger)
		}
		tc.SetKeepAlive(T.server.cConn.KeepAlive)
		tc.SetNoDelay(T.server.cConn.NoDelay)
	}

	if T.server.TLSConfig != nil {
		return tls.Server(tc, T.server.TLSConfig), nil
	}
	return tc, nil
}
