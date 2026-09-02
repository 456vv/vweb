package server

import (
	"sync"

	"github.com/456vv/verror"
	"github.com/456vv/vweb/v3"
)

// plugin 管理 RPC 与 HTTP 插件客户端。
// 使用 sync.Map 保证多 goroutine 并发安全。
type plugin struct {
	rpc  sync.Map // map[string]*vweb.PluginRPCClient
	http sync.Map // map[string]*vweb.PluginHTTPClient
}

// RPC 按名称获取 RPC 插件连接。不存在时返回带追踪信息的错误。
func (T *plugin) RPC(name string) (vweb.PluginRPC, error) {
	if inf, ok := T.rpc.Load(name); ok {
		return inf.(*vweb.PluginRPCClient).Connection()
	}
	return nil, verror.TrackErrorf("rpc plugin %s not found", name)
}

// HTTP 按名称获取 HTTP 插件连接。不存在时返回带追踪信息的错误。
func (T *plugin) HTTP(name string) (vweb.PluginHTTP, error) {
	if inf, ok := T.http.Load(name); ok {
		return inf.(*vweb.PluginHTTPClient).Connection()
	}
	return nil, verror.TrackErrorf("http plugin %s not found", name)
}
