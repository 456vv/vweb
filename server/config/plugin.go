package config
import (
	"github.com/456vv/vweb/v2"
)

type Pluginer interface{
	RPC(name string) (vweb.PluginRPC, error)
	HTTP(name string) (vweb.PluginHTTP, error)
}