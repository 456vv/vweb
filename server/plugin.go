package server

import (
	"fmt"
	"sync"

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
	inf, ok := T.rpc.Load(name)
	if !ok {
		return nil, fmt.Errorf("rpc plugin %s not found", name)
	}
	c, ok := inf.(*vweb.PluginRPCClient)
	if !ok || c == nil {
		return nil, fmt.Errorf("rpc plugin %s is invalid", name)
	}
	return c.Connection()
}

// HTTP 按名称获取 HTTP 插件连接。不存在时返回带追踪信息的错误。
func (T *plugin) HTTP(name string) (vweb.PluginHTTP, error) {
	inf, ok := T.http.Load(name)
	if !ok {
		return nil, fmt.Errorf("http plugin %s not found", name)
	}
	c, ok := inf.(*vweb.PluginHTTPClient)
	if !ok || c == nil {
		return nil, fmt.Errorf("http plugin %s is invalid", name)
	}
	return c.Connection()
}
