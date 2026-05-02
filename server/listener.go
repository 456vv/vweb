package server

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/456vv/vweb/v2/server/config"
)

// tcp连接保持
type listener struct {
	*net.TCPListener              // TCP监听
	cConn            *config.Conn // 用于连接
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

	if T.cConn != nil {
		if d := T.cConn.Deadline; d != 0 {
			tc.SetDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.cConn.WriteDeadline; d != 0 {
			tc.SetWriteDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.cConn.ReadDeadline; d != 0 {
			tc.SetReadDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
		}
		if d := T.cConn.KeepAlivePeriod; d != 0 {
			tc.SetKeepAlivePeriod(time.Duration(d) * time.Millisecond)
		}
		if d := T.cConn.ReadBuffer; d != 0 {
			tc.SetReadBuffer(d)
		}
		if d := T.cConn.WriteBuffer; d != 0 {
			tc.SetWriteBuffer(d)
		}
		if d := T.cConn.Linger; d != 0 {
			tc.SetLinger(T.cConn.Linger)
		}
		tc.SetKeepAlive(T.cConn.KeepAlive)
		tc.SetNoDelay(T.cConn.NoDelay)
	}

	if T.tlsconfig != nil {
		return tls.Server(tc, T.tlsconfig), nil
	}
	return tc, nil
}
