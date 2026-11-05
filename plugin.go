package vweb
	

// PluginType 插件类型枚举（具体定义可能在其它文件，此处仅作类型别名用途说明）。
// 常见取值：PluginTypeRPC、PluginTypeHTTP 等。
type PluginType int
const (
	PluginTypeRPC	PluginType = iota
	PluginTypeHTTP
)

// 本文件仅作包级占位；实际插件接口见 PluginRPC.go、pluginHTTP.go。
type Pluginer interface{
	RPC(name string) (PluginRPC, error)
	HTTP(name string) (PluginHTTP, error)
}