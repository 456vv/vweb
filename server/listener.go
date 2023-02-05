package server

import (
	"crypto/tls"
	"net"
	"time"
)

// tcp连接保持
type listener struct {
	*net.TCPListener // TCP监听
	ser              *Server
	tlsconfig        *tls.Config
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

	if T.ser.cConn != nil {
		if d := T.ser.cConn.Deadline; d != 0 {
			tc.SetDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.ser.cConn.WriteDeadline; d != 0 {
			tc.SetWriteDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.ser.cConn.ReadDeadline; d != 0 {
			tc.SetReadDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.ser.cConn.KeepAlivePeriod; d != 0 {
			tc.SetKeepAlivePeriod(time.Duration(d) * time.Millisecond)
		}
		if d := T.ser.cConn.ReadBuffer; d != 0 {
			tc.SetReadBuffer(d)
		}
		if d := T.ser.cConn.WriteBuffer; d != 0 {
			tc.SetWriteBuffer(d)
		}
		if d := T.ser.cConn.Linger; d != 0 {
			tc.SetLinger(T.ser.cConn.Linger)
		}
		tc.SetKeepAlive(T.ser.cConn.KeepAlive)
		tc.SetNoDelay(T.ser.cConn.NoDelay)
	}

	if T.tlsconfig != nil {
		return tls.Server(tc, T.tlsconfig), nil
	}
	return tc, nil
}
