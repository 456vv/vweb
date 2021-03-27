package server
import (
	"sync"
	"github.com/456vv/vweb/v2"
	"github.com/456vv/verror"
)

type Pluginer interface{
	RPC(name string) (vweb.PluginRPC, error)
	HTTP(name string) (vweb.PluginHTTP, error)
}

type plugin struct {
	rpc 	sync.Map
	http	sync.Map
}

func (T *plugin) RPC(name string) (vweb.PluginRPC, error) {
	inf, ok := T.rpc.Load(name)
	if ok {
		client := inf.(*vweb.PluginRPCClient)
		return client.Connection()
	}
	return nil, verror.TrackErrorf("rpc plugin %s not found", name)
}
func (T *plugin) HTTP(name string) (vweb.PluginHTTP, error) {
	inf, ok := T.http.Load(name)
	if ok {
		client := inf.(*vweb.PluginHTTPClient)
		return client.Connection()
	}
	return nil, verror.TrackErrorf("http plugin %s not found", name)
}
