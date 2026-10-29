package server

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
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
	mu               sync.RWMutex
}

func (T *listener) upConn(c *config.Conn) {
	T.mu.Lock()
	defer T.mu.Unlock()
	T.cConn = c
}

func (T *listener) upTLS(c *tls.Config) {
	T.mu.Lock()
	defer T.mu.Unlock()
	T.tlsconfig = c
}

// Accept 接受下一个连接。
// 并发安全：先检查 closed，再 AcceptTCP。
func (T *listener) Accept() (net.Conn, error) {
	if T.closed.Load() {
		return nil, net.ErrClosed
	}
	if T.TCPListener == nil {
		return nil, net.ErrClosed
	}

	tc, err := T.TCPListener.AcceptTCP()
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			T.closed.Store(true)
		}
		return nil, err
	}

	// 再次检查，防止 Accept 与 Close 竞态
	if T.closed.Load() {
		tc.Close()
		return nil, net.ErrClosed
	}

	T.mu.RLock()
	cfg := T.cConn
	tlsCfg := T.tlsconfig
	T.mu.RUnlock()

	if cfg != nil {
		T.applyConnConfig(tc, cfg)
	}
	if tlsCfg != nil {
		// tls.Server 返回的 Conn 实现了 net.Conn，可直接交给 http.Server
		return tls.Server(tc, tlsCfg), nil
	}
	return tc, nil
}

// applyConnConfig 将 config.Conn 选项应用到 TCP 连接。
// 忽略设置失败（兼容旧 Windows、Plan9、部分嵌入式平台）。
// 时间单位与配置一致（毫秒，Linger 为秒）。
func (T *listener) applyConnConfig(tc *net.TCPConn, cfg *config.Conn) {
	if d := cfg.Deadline; d != 0 {
		tc.SetDeadline(time.Now().Add(millis(d)))
	}
	if d := cfg.WriteDeadline; d != 0 {
		tc.SetWriteDeadline(time.Now().Add(millis(d)))
	}
	if d := cfg.ReadDeadline; d != 0 {
		tc.SetReadDeadline(time.Now().Add(millis(d)))
	}

	// 先 Enable KeepAlive，再设 Period；KeepAlive=false 时设 Period 在部分平台会反向打开保活。
	tc.SetKeepAlive(cfg.KeepAlive)
	if cfg.KeepAlive && cfg.KeepAlivePeriod != 0 {
		// 注意：Windows 10 1709 之前会重置 KeepAliveInterval 为系统默认值
		tc.SetKeepAlivePeriod(millis(cfg.KeepAlivePeriod))
	}
	if d := cfg.ReadBuffer; d != 0 {
		tc.SetReadBuffer(d)
	}
	if d := cfg.WriteBuffer; d != 0 {
		tc.SetWriteBuffer(d)
	}
	if d := cfg.Linger; d != 0 {
		// SetLinger 参数单位为秒；负值表示立即丢弃，0 表示优雅关闭后立即返回。
		// 配置 0 与“未设置”无法区分，保持跳过 0（与原语义一致）。
		tc.SetLinger(d)
	}
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
