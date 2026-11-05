package server

import (
	"fmt"
	"sync"

	"github.com/456vv/vweb/v3"
)

// plugin 管理 RPC 与 HTTP 插件客户端。
// 使用 sync.Map 保证多 goroutine 并发安全。
//
// 热重载策略：rpcConf 记录生成客户端时的配置；配置未变则复用连接池，
// 变了才新建并替换，避免就地改正在使用的 ConnPool。
type plugin struct {
	rpc  sync.Map // map[string]*vweb.PluginRPCClient
	http sync.Map // map[string]*vweb.PluginHTTPClient

	// 记录每个 RPC 客户端生成时使用的配置。热重载时配置未变就保留原客户端
	// 和连接池；变了才新建并替换，不再就地修改正在使用的 ConnPool。
	rpcConf sync.Map // map[string]config.SitePlugin
}

// RPC 按名称获取 RPC 插件连接。
//
// 参数 name：配置中的插件名
// 返回：PluginRPC 句柄，或 "not found" / "invalid" 错误
//
// 示例：
//
//	prpc, err := plugins.RPC("backend")
//	if err != nil { return err }
//	defer prpc.Close()
//	err = prpc.Call("User.Get", id, &user)
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

// HTTP 按名称获取 HTTP 插件连接。
//
// 返回：PluginHTTP（实现见 pluginHTTP.go），用法类似 RPC。
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
