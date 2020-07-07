package server

import (
    "os"
    "encoding/json"
    "crypto/tls"
    "io"
    "net/http"
    "strings"
    "path/filepath"
    "reflect"
    "github.com/456vv/vweb/v2"
    //"fmt"
)

func configMerge(handle func(name string, dsc, src reflect.Value) bool) func(name string, dsc, src reflect.Value) bool {
	return func(name string, dsc, src reflect.Value) bool {
		if handle != nil && handle(name, dsc, src) {
			return true
		}
		return src.IsZero()
	}
}

//ConfigSiteForwards 配置-转发-配置
type ConfigSiteForwards struct {
	Status		bool																		// 启用或禁止
	Path		[]string																	// 多种路径匹配
	ExcludePath []string																	// 排除多种路径匹配
	RePath		string																		// 重写路径
    RedirectCode int                                                         				// 重定向状态码，默认不转向
	End			bool																		// 不进行二次
}
func (T *ConfigSiteForwards) Rewrite(upath string) (rpath string, rewrited bool, err error) {
	forward := vweb.Forward{
		Path:			T.Path,
		ExcludePath:	T.ExcludePath,
		RePath:			T.RePath,
	}
	return forward.Rewrite(upath)
}

type ConfigSiteForward struct {
    //引用公共配置后，该以结构中的Header如果也有设置，将会使用优先使用。
	PublicName				string															// 引用公共配置的名字
	
	List		[]ConfigSiteForwards
}

//ConfigSiteForwardingC 配置-插件-配置-TLS
type ConfigSitePluginTLS struct {
    ServerName          string                                                              // 服务器名称
    InsecureSkipVerify  bool                                                                // 跳过证书验证
    NextProtos          []string                                                            // TCP 协议，如：http/1.1
    CipherSuites        []uint16                                                            // 密码套件的列表。
    ClientSessionCache  int                                                                 // 是TLS会话恢复 ClientSessionState 条目的缓存。(Client端使用)
    CurvePreferences    []tls.CurveID                                                       // 在ECDHE握手中使用(Client端使用)
    RootCAs             []string                                                            // 根证书文件
}


//ConfigSitePlugin 配置-插件
type ConfigSitePlugin struct {
    //引用公共配置后，该以结构中的Header如果也有设置，将会使用优先使用。
	PublicName				string															// 引用公共配置的名字
    Status              	bool                                                			// 状态，是否启用

	//公共
    Addr        			string                                                          // 地址
    LocalAddr				string															// 本地拨号IP
    Timeout                 int64                                                           // 拨号超时（毫秒单位）
    KeepAlive               int64                                                           // 保持连接超时（毫秒单位）
    FallbackDelay			int64															// 后退延时，等待双协议栈延时，（毫秒单位，默认300ms）。
    DualStack				bool															// 尝试建立多个IPv4和IPv6的连接
    IdeConn     			int                                                             // 空闲连接数

	//RPC
    Path        			string                                                          // 路径
    MaxConn					int																// 最大连接数

	//HTTP
	ProxyURL				string															// 验证用户密码或是否使用socks5
	Host        			string                                                          // Host
	Scheme					string															// 协议
    TLS                     *ConfigSitePluginTLS                                         	// TLS
    TLSHandshakeTimeout 	int64                                                           // 握手超时（毫秒单位）
    DisableKeepAlives       bool                                                            // 禁止长连接
    DisableCompression      bool                                                            // 禁止压缩
	MaxIdleConnsPerHost		int																// 最大空闲连接每个主机
    MaxConnsPerHost 		int																// 最大连接的每个主机
    IdleConnTimeout 		int64                                                           // 设置空闲连接超时（毫秒单位）
    ResponseHeaderTimeout   int64                                                           // 请求Header超时（毫秒单位）
    ExpectContinueTimeout   int64                                                           // 发送Expect: 100-continue标头的PUT请求超时
    ProxyConnectHeader		http.Header 													// CONNECT代理请求中 增加标头 map[string][]string
    MaxResponseHeaderBytes	int64															// 最大的响应标头限制（字节）
    ReadBufferSize			int																// 读取缓冲大小
    WriteBufferSize			int																// 写入缓冲大小
    ForceAttemptHTTP2		bool															// 支持HTTP2
}

func (T *ConfigSitePlugin) ConfigPluginHTTPClient(c *vweb.PluginHTTPClient) error {
	return configHTTPClient(c , T)
}
func (T *ConfigSitePlugin) ConfigPluginRPCClient(c *vweb.PluginRPCClient) error {
	return configRPCClient(c, T)
}

//ConfigSitePlugins 配置-插件
type ConfigSitePlugins struct {
	RPC				map[string]ConfigSitePlugin
	HTTP			map[string]ConfigSitePlugin
}
func (T *ConfigSitePlugins) ConfigSitePluginRPC(origin *ConfigSitePlugin, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.RPC[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin=c
		return true
	}
	return false
}
func (T *ConfigSitePlugins) ConfigSitePluginHTTP(origin *ConfigSitePlugin, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.HTTP[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin=c
		return true
	}
	return false
}

//ConfigSiteHeaderType 配置-标头-类型
type ConfigSiteHeaderType struct {
	Header          map[string][]string                                     // Header
    PageExpired     int64                                                   // 页面过期(秒单位)
}

//ConfigSiteHeader 配置-标头
type ConfigSiteHeader struct {
    //引用公共配置后，该以结构中的Header如果也有设置，将会使用优先使用。
	PublicName			string												// 引用公共配置的名字
    Static, Dynamic map[string]ConfigSiteHeaderType                         // 静态，动态Header，map[".html"]ConfigSiteHeaderType
    MIME            map[string]string                                       // MIME类型
}

//ConfigSiteDirectory 配置-目录
type ConfigSiteDirectory struct {
    Root       string                                                       // 主目录
    Virtual    []string                                                     // 虚目录
}

//RootDir	根目录
//	r *http.Request	    		请求
//	string			    		根目录路径
func (T *ConfigSiteDirectory) RootDir(upath string) string {
    var (
        p			= filepath.Clean(upath)//r.URL.Path
        root    	= filepath.FromSlash(T.Root)
        separator	= string(filepath.Separator)
    )

    for _, v := range T.Virtual {
        if v == ""{
        	continue
        }
    	v = filepath.FromSlash(v)
        pos	:= strings.LastIndex(v, separator)
        if strings.HasPrefix(p+separator, separator+v[pos+1:]+separator) {
        	if pos == 0 {
        		pos=1
        	}
			root = v[:pos]
			break
		}
    }
    return root
}

//ConfigSiteLogLevel 配置-日志-级别
type ConfigSiteLogLevel int
const (
    ConfigSiteLogLevelDisable ConfigSiteLogLevel =   iota                  // 禁用日志记录，默认不开启
)


//ConfigSiteLog 配置-日志，这个功能后面待加。
type ConfigSiteLog struct {
    Level       ConfigSiteLogLevel                                          // 级别
    Directory   string                                                      // 目录
}


//ConfigSitePropertySession 配置-性能-会话
type ConfigSiteSession struct {
    //引用公共配置后，该以结构中的Header如果也有设置，将会使用优先使用。
	PublicName		string													// 引用公共配置的名字
    Name            string                                                  // 会话名称
    Expired         int64                                                   // 过期时间(毫秒单位，默认20分钟)
    Size            int                                                     // 会话ID长度(默认长度40位)
    Salt            string                                                  // 加盐，由于计算机随机数是伪随机数。（可默认为空）

    // 如果客户端会话过期后。客户端被重新发送请求到服务端。服务端是否决定使用原会话ID。
    // 如果使用原ID，可能不安全。但在特殊情况下可以需要保持原ID。
    // 所以默认为不保持。如果需要请设置为false。
    ActivationID    bool                                                    // 为true，表示保留ID。否则重新生成新的ID
}

//ConfigSiteProperty 配置-性能
type ConfigSiteProperty struct {
    //引用公共配置后，该以结构中的Header如果也有设置，将会使用优先使用。
	PublicName		string													// 引用公共配置的名字
	
    ConnMaxNumber       int64                                               // 连接最大数量
    ConnSpeed           int64                                               // 连接宽带速度
    BuffSize       		int64                                             	// 缓冲区大小
}

//ConfigSite 配置-站点
type ConfigSite struct {
    Status              bool                                                // 状态，是否启动此站点
    Name                string                                              // 站点别名
    Identity			string												// 站点维一码，可以说是池名

    Host                []string                                            // 域名绑定
    Forward          	map[string]ConfigSiteForward          				// 转发
    Plugin              ConfigSitePlugins									// 插件
    Directory           ConfigSiteDirectory                                 // 目录

    IndexFile           []string                                            // 默认页
    DynamicExt          []string                                            // 动态文件后缀
    DynamicCache		bool												// 动态文件缓存解析，非缓存执行

    Header              ConfigSiteHeader                                    // HTTP头
    Log                 ConfigSiteLog                                       // 日志
    ErrorPage           map[string]string                                   // 错误页
    
    Session             ConfigSiteSession                           		// 会话
    Property            ConfigSiteProperty                                  // 性能

}
type ConfigSitePublic struct {
	Header				map[string]ConfigSiteHeader
	Session				map[string]ConfigSiteSession
	Plugin				ConfigSitePlugins
	Forward				map[string]ConfigSiteForward
	Property			map[string]ConfigSiteProperty
}

func (T *ConfigSitePublic) ConfigSiteSession(origin *ConfigSiteSession, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.Session[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin = c
		return true
	}
	return false
}
func (T *ConfigSitePublic) ConfigSiteHeader(origin *ConfigSiteHeader, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.Header[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin = c
		return true
	}
	return false
}
func (T *ConfigSitePublic) ConfigSiteForward(origin *ConfigSiteForward, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.Forward[origin.PublicName]
	if ok {
		origin.List = append(origin.List, c.List...)
		return true
	}
	return false
}
func (T *ConfigSitePublic) ConfigSiteProperty(origin *ConfigSiteProperty, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.Property[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin = c
		return true
	}
	return false
}

type ConfigSites struct {
	Public		ConfigSitePublic
    Site       	[]ConfigSite                                                  // 站点
}

type ConfigServerTLSFile struct {
    CertFile, KeyFile   string                                              // 证书，key 文件地址
}

type ConfigServerTLS struct {
    RootCAs             []ConfigServerTLSFile                                                   // 服务端证书文件
    NextProtos          []string                                                                // http版本
    CipherSuites        []uint16                                                                // 密码套件
    PreferServerCipherSuites    bool                                                            // 控制服务器是否选择客户端的最首选的密码套件
    SessionTicketsDisabled      bool                                                            // 设置为 true 可禁用会话票证 (恢复) 支持。
    SessionTicketKey            [32]byte                                                        // TLS服务器提供会话恢复
    SetSessionTicketKeys		[][32]byte														// 会话恢复票证
    DynamicRecordSizingDisabled	bool															// 禁用TLS动态记录自适应大小
    MinVersion                  uint16                                                          // 最小SSL/TLS版本。如果为零，则SSLv3的被取为最小。
    MaxVersion                  uint16                                                          // 最大SSL/TLS版本。如果为零，则该包所支持的最高版本被使用。
    ClientCAs 					[]string                                                        // 客户端拥有的“权威组织”证书的列表。(Server/Client端使用)

}

type ConfigServer struct {
    //引用公共配置后，该以结构中的CC和CS如果也有设置，将会使用优先使用。
	PublicName			string												// 引用公共配置的名字
    ReadTimeout         int64                                               // 设置读取超时(毫秒单位)
    WriteTimeout        int64                                               // 设置写入超时(毫秒单位)
    ReadHeaderTimeout   int64                                               // 读取标头超时(毫秒单位）
    IdleTimeout			int64												// 保持连接空闲超时，如果为0，使用 ReadTimeout,(毫秒单位）
    MaxHeaderBytes      int                                                 // 如果0，最大请求头的大小，http.DefaultMaxHeaderBytes
    KeepAlivesEnabled   bool                                                // 支持客户端Keep-Alive
    ShutdownConn		bool												// 服务器关闭监听，不会即时关闭正在下载的连接。空闲后再关闭。(默认即时关闭)
    TLS                 *ConfigServerTLS                                    // TLS
}
type  ConfigConn struct{
    //引用公共配置后，该以结构中的CC和CS如果也有设置，将会使用优先使用。
	PublicName			string												// 引用公共配置的名字
    Deadline            int64                                               // 设置读写超时(毫秒单位)
    WriteDeadline       int64                                               // 设置写入超时(毫秒单位)
    ReadDeadline        int64                                               // 设置读取超时(毫秒单位)
    KeepAlive           bool                                                // 即使没有任何通信，一个客户端可能希望保持连接到服务器的状态。
    KeepAlivePeriod     int64                                               // 保持连接超时(毫秒单位)
    Linger              int                                                 // 数据等待发送或待确认（不太清楚这个功能有什么用？）
    NoDelay             bool                                                // 设置操作系统是否延迟发送数据包,默认是无延迟的
    ReadBuffer          int                                                 // 在缓冲区读取数据大小
    WriteBuffer         int                                                 // 写入数据到缓冲区大小
}
type ConfigServerPublic struct {
    CC                  map[string]ConfigConn                                          // 连接设置
    CS                  map[string]ConfigServer                                        // 服务器设置
}

func (T *ConfigServerPublic) ConfigConn(origin *ConfigConn, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.CC[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin = c
		return true
	}
	return false
}

func (T *ConfigServerPublic) ConfigServer(origin *ConfigServer, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.CS[origin.PublicName]
	if ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
		*origin = c
		return true
	}
	return false
}

type ConfigListen struct {
    Status              bool                                                // 状态，是否启动此服务器
    CC                  ConfigConn                                          // 连接设置
    CS                  ConfigServer                                        // 服务器设置
}
type ConfigServers struct {
	Public				ConfigServerPublic
    Listen				map[string]ConfigListen
}

//Config 配置
type Config struct {
    Servers     	ConfigServers                               			// 服务器集
    Sites       	ConfigSites                                             // 站点集
}

//ConfigFileParse 解析服务器配置文件，一个JSON格式的文件。
//    参：
//      conf *Config    配置
//      file string     文件
//    返：
//      error           错误，如果文件无法打开，或无法解析的情况
func ConfigFileParse(conf *Config, file string) error {
    osFile, err := os.Open(file)
    defer osFile.Close()
    if err != nil {
        return err
    }
    jsonDecoder := json.NewDecoder(osFile)
    return jsonDecoder.Decode(conf)
}


//ConfigDataParse 解析服务器配置数据，一个JSON格式的数据。
//    参：
//      conf *Config        配置
//      r   io.Reader       读接口
//    返：
//      error               错误，如果无法解析的情况
func ConfigDataParse(conf *Config, r io.Reader) error {
    jsonDecoder := json.NewDecoder(r)
    return jsonDecoder.Decode(conf)
}