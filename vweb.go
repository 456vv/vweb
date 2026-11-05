package vweb

// Package vweb 是一个轻量级 Web 服务器框架，提供站点池、会话、静态/动态处理、
// 插件（HTTP/RPC）、路由与配置热加载等能力。
//
// 本文件定义框架级常量与请求上下文 Key，便于在中间件与 Handler 间传递共享对象。

// defaultDataBufioSize 默认读写缓冲大小（32KB），用于减少系统调用次数。
const defaultDataBufioSize int = 32 * 1024

// encodeStd 用于生成会话 ID 等伪随机字符串时的可用字符集（Base64 风格变体）。
const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._"

// contextKey 是请求 context 中使用的私有键类型，避免与其他包冲突。
type contextKey struct {
	name string
}

// String 返回调试用描述，例如 "vweb context value web-site"。
func (T *contextKey) String() string { return "vweb context value " + T.name }

// 请求上下文中预定义的 Key，可在 Handler / 模板 / 中间件中通过 req.Context().Value(...) 取出。
//
// 示例：
//
//	if site, ok := req.Context().Value(vweb.SiteContextKey).(*vweb.Site); ok {
//	    // 使用 site
//	}
var (
	SiteContextKey     = &contextKey{"web-site"}     // 当前站点 *Site
	RouterContextKey   = &contextKey{"web-router"}   // 路由相关对象
	ListenerContextKey = &contextKey{"web-listener"} // 监听器
	ConnContextKey     = &contextKey{"web-conn"}     // 底层连接
	PluginContextKey   = &contextKey{"web-plugin"}   // 插件管理器
)
