package vweb
	

type PluginType int
const (
	PluginTypeRPC	PluginType = iota
	PluginTypeHTTP
)


type Pluginer interface{
	RPC(name string) (PluginRPC, error)
	HTTP(name string) (PluginHTTP, error)
}