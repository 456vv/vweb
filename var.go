package vweb

// 上下文中使用的key
var (
	SiteContextKey     = &contextKey{"web-site"}
	ListenerContextKey = &contextKey{"web-listener"}
	ConnContextKey     = &contextKey{"web-conn"}
	PluginContextKey   = &contextKey{"web-plugin"}
)
