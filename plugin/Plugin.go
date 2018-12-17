package plugin
import (
    "fmt"
    "encoding/json"
)

//PluginType 插件类型
type PluginType int
const(
    PluginTypeHTTP   PluginType = iota                                      // HTTP
    PluginTypeRPC                                                           // RPC
)
//PluginExtra 插件额外信息
type PluginExtra map[string]string

//Plugin 插件，扩展 birdswo 服务器的插件
type Plugin struct{
    Type        PluginType                                                  // 插件类型
    Version     string                                                      // 插件版本
    Name, Addr  string                                                      // 名称，IP地址
    Error		error														// 发生错误的信息
    Extra       PluginExtra                                                 // 额外信息
}

//String 标准输出，输出配置。客户端解析。
func (p *Plugin) String() string {
    b, _ := json.Marshal(p)
    return fmt.Sprintf("%s", b)
}
