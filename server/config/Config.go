package config

import (
    "os"
    "encoding/json"
    "crypto/tls"
    "io"
	"io/ioutil"
	"net"
    "net/http"
	"net/url"
    "strings"
    "path/filepath"
    "reflect"
	"time"
	"crypto/x509"
    "github.com/456vv/vweb/v2"
	"github.com/456vv/vconnpool/v2"
	"github.com/456vv/verror"
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
	if c, ok := T.RPC[origin.PublicName]; ok && vweb.CopyStructDeep(&c, origin, configMerge(handle)) == nil {
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
    Expired         int64                                           		// 过期时间(秒单位，默认20分钟)
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
    BuffSize       		int                                             	// 缓冲区大小
}

type ConfigSiteDynamic struct {
    //引用公共配置后，该以结构中的Header如果也有设置，将会使用优先使用。
	PublicName		string													// 引用公共配置的名字]
	
    Ext          	[]string                                            	// 动态文件后缀
    Cache			bool													// 动态文件缓存解析，非缓存执行
    CacheParseTimeout	int64												// 动态文件缓存解析超时，（秒为单位）
    CacheStaticFileDir	string												// 缓存静态文件目录，仅适于markdown转HTML
    CacheStaticAllowPath	[]string										// 缓存静态路径，仅适于markdown转HTML
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
    Dynamic				ConfigSiteDynamic

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
	Dynamic				map[string]ConfigSiteDynamic
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
func (T *ConfigSitePublic) ConfigSiteDynamic(origin *ConfigSiteDynamic, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.Dynamic[origin.PublicName]
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
func (T *ConfigServerTLS) CipherSuitesAuto(){
	if T.MaxVersion == 0 {
		T.MaxVersion = tls.VersionTLS13
	}
	if len(T.CipherSuites) == 0 {
		for _, cs := range tls.CipherSuites() {
			for _, version := range cs.SupportedVersions {
				if version >= T.MinVersion && version <= T.MaxVersion {
					T.CipherSuites = append(T.CipherSuites, cs.ID)
				}
			}
		}
	}
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
    Linger              int                                                 // 连接关闭后，等待发送或待确认的数据（秒单位)。如果 sec > 0，经过sec秒后，所有剩余的未发送数据都可能会被丢弃。则与sec < 0 一样在后台发送数据。
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
		if origin.TLS != nil && len(origin.TLS.CipherSuites) == 0 {
			origin.TLS.CipherSuitesAuto()
		}
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

//ParseFile 解析服务器配置文件，一个JSON格式的文件。
//    参：
//      file string     文件
//    返：
//      error           错误，如果文件无法打开，或无法解析的情况
func (T *Config) ParseFile(file string) error {
    osFile, err := os.Open(file)
    if err != nil {
        return err
    }
    defer osFile.Close()
    return json.NewDecoder(osFile).Decode(T)
}


//ParseReader 解析服务器配置数据，一个JSON格式的数据。
//    参：
//      r   io.Reader       读接口
//    返：
//      error               错误，如果无法解析的情况
func (T *Config) ParseReader(r io.Reader) error {
    return json.NewDecoder(r).Decode(T)
}


//配置HTTP插件客户端
func configHTTPClient(c *vweb.PluginHTTPClient, conf *ConfigSitePlugin) error {
	
	c.Addr						= conf.Addr
	c.Host						= conf.Host
	c.Scheme					= conf.Scheme
	
	if c.Dialer == nil {
		c.Dialer = &net.Dialer{}
	}
   	if conf.LocalAddr != "" {
		//设置本地拨号地址
		netTCPAddr, err := net.ResolveTCPAddr("tcp", conf.LocalAddr)
		if err != nil {
   	    	return verror.TrackErrorf("ConfigSitePlugin.LocalAddr 地址无法解析这个(%s)。格式应该是 111.222.444.555:0 或者 www.xxx.com:0", conf.LocalAddr)
		}
		c.Dialer.LocalAddr = netTCPAddr
	}
    c.Dialer.Timeout 		= time.Duration(conf.Timeout) * time.Millisecond
    c.Dialer.KeepAlive   	= time.Duration(conf.KeepAlive) * time.Millisecond
    c.Dialer.FallbackDelay	= time.Duration(conf.FallbackDelay) * time.Millisecond
    c.Dialer.DualStack		= conf.DualStack
	
	if c.Tr == nil {
		c.Tr = new(http.Transport)
		c.Tr.Proxy = http.ProxyFromEnvironment
	}
	if conf.ProxyURL != "" {
		u, err := url.Parse(conf.ProxyURL)
		if err != nil {
			return verror.TrackErrorf("代理地址不是有效的ConfigSitePlugin.ProxyURL(%s)", conf.ProxyURL)
		}
		c.Tr.Proxy = http.ProxyURL(u)
	}
	c.Tr.DisableKeepAlives		= conf.DisableKeepAlives
	c.Tr.DisableCompression		= conf.DisableCompression
	c.Tr.MaxIdleConns			= conf.IdeConn
	c.Tr.MaxIdleConnsPerHost	= conf.MaxIdleConnsPerHost
	c.Tr.MaxConnsPerHost		= conf.MaxConnsPerHost
	c.Tr.MaxResponseHeaderBytes = conf.MaxResponseHeaderBytes
	c.Tr.ReadBufferSize			= conf.ReadBufferSize
	c.Tr.ForceAttemptHTTP2		= conf.ForceAttemptHTTP2
	c.Tr.WriteBufferSize		= conf.WriteBufferSize
	if d := conf.ResponseHeaderTimeout; d != 0 {
		c.Tr.ResponseHeaderTimeout   = time.Duration(d) * time.Millisecond
	}
	if d := conf.ExpectContinueTimeout; d != 0 {
		c.Tr.ExpectContinueTimeout   = time.Duration(d) * time.Millisecond
	}
	if d := conf.IdleConnTimeout; d != 0 {
		c.Tr.IdleConnTimeout   = time.Duration(d) * time.Millisecond
	}
	if d := conf.TLSHandshakeTimeout; d != 0 {
		c.Tr.TLSHandshakeTimeout   = time.Duration(d) * time.Millisecond
	}
    if len(conf.ProxyConnectHeader) != 0 {
    	c.Tr.ProxyConnectHeader = conf.ProxyConnectHeader.Clone()
    }
	var tlsconfig *tls.Config
    if conf.TLS != nil && conf.TLS.ServerName != "" {
        tlsconfig = &tls.Config{
             ServerName			: conf.TLS.ServerName,
             InsecureSkipVerify	: conf.TLS.InsecureSkipVerify,
        }
        if len(conf.TLS.NextProtos) > 0 {
            copy(tlsconfig.NextProtos, conf.TLS.NextProtos)
        }
        if len(conf.TLS.CipherSuites) > 0 {
            copy(tlsconfig.CipherSuites, conf.TLS.CipherSuites)
        }else{
			//内部判断并使用默认的密码套件
            tlsconfig.CipherSuites = nil
        }
        if conf.TLS.ClientSessionCache != 0 {
            tlsconfig.ClientSessionCache = tls.NewLRUClientSessionCache(conf.TLS.ClientSessionCache)
        }
        if len(conf.TLS.CurvePreferences) != 0 {
            copy(tlsconfig.CurvePreferences, conf.TLS.CurvePreferences)
        }
		
		if tlsconfig.RootCAs == nil {
			if certPool, err := x509.SystemCertPool(); err == nil {
				//系统证书
				tlsconfig.RootCAs = certPool
			}else{
				//如果读取系统根证书失败，则创建新的证书
				tlsconfig.RootCAs = x509.NewCertPool()
			}
		}
		
        for _, filename := range conf.TLS.RootCAs {
			//打开文件
			caData, err := ioutil.ReadFile(filename)
			if err != nil {
        		return verror.TrackErrorf("%s %s", filename, err.Error()) 
			}
			
			ext := filepath.Ext(filename)
			if ext == ".crt" {
				certificates, err := x509.ParseCertificates(caData)
				if err != nil {
        			return verror.TrackErrorf("%s %s", filename, err.Error()) 
				}
				for _, cert := range certificates {
					tlsconfig.RootCAs.AddCert(cert)
				}
			}else if ext == ".pem" {
				if !tlsconfig.RootCAs.AppendCertsFromPEM(caData) {
	        		return verror.TrackErrorf("%s %s\n", filename, "not is a valid PEM format")
				}
			}
        }
    }
   	c.Tr.TLSClientConfig = tlsconfig
   	
    return nil
}
//快速的配置RPC
func configRPCClient(c *vweb.PluginRPCClient, conf *ConfigSitePlugin) error {
    c.Addr	= conf.Addr
   	c.Path  = conf.Path
    //RPC客户端连接池
    if c.ConnPool == nil {
    	c.ConnPool = &vconnpool.ConnPool{}
    }
    c.ConnPool.IdeConn 				= conf.IdeConn
    c.ConnPool.MaxConn 				= conf.MaxConn
    
    if c.ConnPool.Dialer  == nil {
	    c.ConnPool.Dialer = &net.Dialer{}
	}
   	if conf.LocalAddr != "" {
		//设置本地拨号地址
		netTCPAddr, err := net.ResolveTCPAddr("tcp", conf.LocalAddr)
		if err != nil {
   	    	return verror.TrackErrorf("ConfigSitePlugin.LocalAddr 地址无法解析这个(%s)。格式应该是 111.222.444.555:0 或者 www.xxx.com:0", conf.LocalAddr)
		}
		c.ConnPool.Dialer.LocalAddr = netTCPAddr
	}
    c.ConnPool.Dialer.Timeout 		= time.Duration(conf.Timeout) * time.Millisecond
    c.ConnPool.Dialer.KeepAlive   	= time.Duration(conf.KeepAlive) * time.Millisecond
    c.ConnPool.Dialer.FallbackDelay	= time.Duration(conf.FallbackDelay) * time.Millisecond
    c.ConnPool.Dialer.DualStack		= conf.DualStack
	return nil
}
