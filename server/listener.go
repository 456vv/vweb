package server

import (
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/456vv/vweb/v3/server/config"
)

// listener 实现 net.Listener，支持 TCP 选项配置与可选 TLS 包装。
// 可被多个 goroutine 并发调用 Accept/Close。
// closed 使用 atomic.Bool 保证可见性与并发安全。
type listener struct {
	*net.TCPListener              // TCP监听
	cConn            *config.Conn // 用于连接
	tlsconfig        *tls.Config
	closed           atomic.Bool // 使用原子操作保证并发安全
}

// Accept 接受下一个连接。
// 并发安全：先检查 closed，再 AcceptTCP。
// TLS 包装在 TCP 选项应用之后进行。
func (T *listener) Accept() (net.Conn, error) {
	if T.closed.Load() {
		return nil, net.ErrClosed
	}
	if T.TCPListener == nil {
		// 尚未 Serve（例如直接误用），同样按已关闭处理，避免 nil 解引用
		return nil, net.ErrClosed
	}

	tc, err := T.TCPListener.AcceptTCP()
	if err != nil {
		// 仅在明确关闭错误时标记，避免依赖已弃用的 Temporary()
		// 其他错误（超时、临时网络问题）直接返回，由上层决定重试或退出
		if errors.Is(err, net.ErrClosed) {
			T.closed.Store(true)
		}
		return nil, err
	}

	// 应用连接配置（在 TLS 包装前设置 TCP 层选项）
	if T.cConn != nil {
		T.applyConnConfig(tc)
	}

	if T.tlsconfig != nil {
		// tls.Server 返回的 Conn 实现了 net.Conn，可直接交给 http.Server
		return tls.Server(tc, T.tlsconfig), nil
	}
	return tc, nil
}

// applyConnConfig 将 config.Conn 选项应用到 TCP 连接。
// 忽略设置失败（兼容旧 Windows、Plan9、部分嵌入式平台）。
// 时间单位与配置一致（毫秒，Linger 为秒）。
func (T *listener) applyConnConfig(tc *net.TCPConn) {
	cfg := T.cConn

	if d := cfg.Deadline; d != 0 {
		tc.SetDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
	}
	if d := cfg.WriteDeadline; d != 0 {
		tc.SetWriteDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
	}
	if d := cfg.ReadDeadline; d != 0 {
		tc.SetReadDeadline(time.Now().Add(time.Duration(d) * time.Millisecond))
	}
	if d := cfg.KeepAlivePeriod; d != 0 {
		// 注意：Windows 10 1709 之前会重置 KeepAliveInterval 为系统默认值
		// 现代 Windows / Linux / macOS / FreeBSD 均正常支持
		tc.SetKeepAlivePeriod(time.Duration(d) * time.Millisecond)
	}
	if d := cfg.ReadBuffer; d != 0 {
		tc.SetReadBuffer(d)
	}
	if d := cfg.WriteBuffer; d != 0 {
		tc.SetWriteBuffer(d)
	}
	if d := cfg.Linger; d != 0 {
		// SetLinger 参数单位为秒；负值表示立即丢弃，0 表示优雅关闭后立即返回
		tc.SetLinger(d)
	}
	// KeepAlive 与 NoDelay 几乎在所有主流平台可用
	tc.SetKeepAlive(cfg.KeepAlive)
	tc.SetNoDelay(cfg.NoDelay)
}

// Close 关闭监听器（幂等）。
// 先标记 closed，再关闭底层，使阻塞 Accept 立即返回 net.ErrClosed。
// 使用 CAS 保证底层只被关闭一次：重复 Close 返回 nil，避免二次关闭 fd 的平台差异。
func (T *listener) Close() error {
	if !T.closed.CompareAndSwap(false, true) {
		return nil // 已被关闭（幂等）
	}
	if T.TCPListener != nil {
		return T.TCPListener.Close()
	}
	return nil
}

// Addr 返回监听地址。
func (T *listener) Addr() net.Addr {
	if T.TCPListener != nil {
		return T.TCPListener.Addr()
	}
	return nil
}
