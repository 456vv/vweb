package vweb

// 上下文中使用的key
var (
	SiteContextKey     = &contextKey{"web-site"}
	RouterContextKey   = &contextKey{"web-router"}
	ListenerContextKey = &contextKey{"web-listener"}
	ConnContextKey     = &contextKey{"web-conn"}
	PluginContextKey   = &contextKey{"web-plugin"}
)
