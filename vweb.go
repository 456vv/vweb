package vweb

// 其它
const defaultDataBufioSize int = 32 * 1024 // 默认数据缓冲32MB

// 随机数的可用字符
const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._"

// 上下文的Key，在请求中可以使用
type contextKey struct {
	name string
}

func (T *contextKey) String() string { return "vweb context value " + T.name }

// 上下文中使用的key
var (
	SiteContextKey     = &contextKey{"web-site"}
	RouterContextKey   = &contextKey{"web-router"}
	ListenerContextKey = &contextKey{"web-listener"}
	ConnContextKey     = &contextKey{"web-conn"}
	PluginContextKey   = &contextKey{"web-plugin"}
)
