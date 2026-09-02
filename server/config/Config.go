package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/456vv/vconnpool/v3"
	"github.com/456vv/verror"
	"github.com/456vv/vweb/v3"
	"github.com/456vv/vweb/v3/builtin"
)

// configExclude 返回一个过滤函数：跳过零值源字段，或由自定义 handle 决定跳过。
// 兼容指针、切片、map、接口、数组、chan、func 等反射类型的零值判断。
// 返回 true 表示跳过该字段（不从 src 写入 dst）。
func configExclude(handle func(name string, dsc, src reflect.Value) bool) func(name string, dsc, src reflect.Value) bool {
	return func(name string, dsc, src reflect.Value) bool {
		if handle != nil && handle(name, dsc, src) {
			return true
		}
		if !src.IsValid() {
			return true
		}
		// 更完整的零值判断，覆盖常见可配置类型
		switch src.Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
			return src.IsNil()
		case reflect.Array:
			// 数组全零才视为零
			for i := 0; i < src.Len(); i++ {
				if !src.Index(i).IsZero() {
					return false
				}
			}
			return true
		default:
			return src.IsZero()
		}
	}
}

// SiteForward 配置-转发-配置
type SiteForward struct {
	Status       bool     // 启用或禁止
	Path         []string // 多种路径匹配
	ExcludePath  []string // 排除多种路径匹配
	RePath       string   // 重写路径
	RedirectCode int      // 重定向状态码，默认不转向
	End          bool     // 不进行二次匹配/转发
	// forwardWriter 为运行时状态，不属于持久化配置；合并公共配置时不应共享该指针。
	forwardWriter *vweb.ForwardRewriter
}

func (T *SiteForward) Compile() (fw *vweb.ForwardRewriter, err error) {
	if T.forwardWriter == nil {
		forward := vweb.Forward{
			Path:        T.Path,
			ExcludePath: T.ExcludePath,
			RePath:      T.RePath,
		}
		fw, err = forward.Compile()
		T.forwardWriter = fw
		return
	}
	return T.forwardWriter, nil
}

// SiteForwards 转发集合
type SiteForwards struct {
	// 引用公共配置后，该结构中的字段如果也有设置，将会优先使用。
	PublicName string // 引用公共配置的名字

	List []SiteForward
}

// SitePluginTLS 配置-插件-TLS
type SitePluginTLS struct {
	ServerName         string        // 服务器名称
	InsecureSkipVerify bool          // 跳过证书验证
	NextProtos         []string      // ALPN 协议，如：http/1.1
	CipherSuites       []uint16      // 密码套件列表
	ClientSessionCache int           // TLS 会话恢复 ClientSessionState 缓存大小（Client 端）
	CurvePreferences   []tls.CurveID // ECDHE 握手曲线偏好（Client 端）
	RootCAs            []string      // 根证书文件路径列表
}

// SitePlugin 配置-插件
type SitePlugin struct {
	// 引用公共配置后，该结构中的字段如果也有设置，将会优先使用。
	PublicName string // 引用公共配置的名字
	Status     bool   // 状态，是否启用

	// 公共
	Addr          string // 地址
	LocalAddr     string // 本地拨号 IP
	Timeout       int64  // 拨号超时（毫秒单位）
	KeepAlive     int64  // 保持连接超时（毫秒单位）
	FallbackDelay int64  // 后退延时，等待双协议栈延时，（毫秒单位，默认 300ms）
	IdeConn       int    // 空闲连接数（历史字段名，保持导出兼容）

	// RPC
	Path    string // 路径
	MaxConn int    // 最大连接数

	// HTTP
	ProxyURL               string         // 代理 URL（可含用户密码或 socks5）
	Host                   string         // Host
	Scheme                 string         // 协议
	TLS                    *SitePluginTLS // TLS
	TLSHandshakeTimeout    int64          // 握手超时（毫秒单位）
	DisableKeepAlives      bool           // 禁止长连接
	DisableCompression     bool           // 禁止压缩
	MaxIdleConnsPerHost    int            // 每个主机最大空闲连接
	MaxConnsPerHost        int            // 每个主机最大连接
	IdleConnTimeout        int64          // 空闲连接超时（毫秒单位）
	ResponseHeaderTimeout  int64          // 请求 Header 超时（毫秒单位）
	ExpectContinueTimeout  int64          // 发送 Expect: 100-continue 的 PUT 超时
	ProxyConnectHeader     http.Header    // CONNECT 代理请求额外标头
	MaxResponseHeaderBytes int64          // 最大响应标头限制（字节）
	ReadBufferSize         int            // 读取缓冲大小
	WriteBufferSize        int            // 写入缓冲大小
	ForceAttemptHTTP2      bool           // 支持 HTTP/2
}

// clone 对 SitePlugin 进行深度复制，以防并发协程共享指针/切片导致的数据竞争或非预期修改。
func (T *SitePlugin) clone() *SitePlugin {
	if T == nil {
		return nil
	}
	cp := *T
	if T.TLS != nil {
		cp.TLS = T.TLS.clone()
	}
	if T.ProxyConnectHeader != nil {
		cp.ProxyConnectHeader = T.ProxyConnectHeader.Clone()
	}
	return &cp
}

func (T *SitePlugin) ConfigPluginHTTPClient(c *vweb.PluginHTTPClient) error {
	return configHTTPClient(c, T)
}

func (T *SitePlugin) ConfigPluginRPCClient(c *vweb.PluginRPCClient) error {
	return configRPCClient(c, T)
}

// SitePlugins 配置-插件
type SitePlugins struct {
	RPC  map[string]SitePlugin
	HTTP map[string]SitePlugin
}

// clone 对 SitePluginTLS 进行深度复制。
func (T *SitePluginTLS) clone() *SitePluginTLS {
	if T == nil {
		return nil
	}
	cp := *T
	if len(T.NextProtos) > 0 {
		cp.NextProtos = make([]string, len(T.NextProtos))
		copy(cp.NextProtos, T.NextProtos)
	}
	if len(T.CipherSuites) > 0 {
		cp.CipherSuites = make([]uint16, len(T.CipherSuites))
		copy(cp.CipherSuites, T.CipherSuites)
	}
	if len(T.CurvePreferences) > 0 {
		cp.CurvePreferences = make([]tls.CurveID, len(T.CurvePreferences))
		copy(cp.CurvePreferences, T.CurvePreferences)
	}
	if len(T.RootCAs) > 0 {
		cp.RootCAs = make([]string, len(T.RootCAs))
		copy(cp.RootCAs, T.RootCAs)
	}
	return &cp
}

// ConfigSitePluginHTTP 从公共 HTTP 配置模板中合并配置到 origin 中
func (T *SitePlugins) ConfigSitePluginHTTP(origin *SitePlugin, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.HTTP[origin.PublicName]
	if !ok {
		return false
	}

	// 深度复制副本，隔离原公共模板中 TLS、ProxyConnectHeader 等指针，规避并发数据竞争
	cCloned := c.clone()
	if builtin.CopyStructDeep(cCloned, origin, configExclude(handle)) == nil {
		*origin = *cCloned
		return true
	}
	return false
}

// ConfigSitePluginRPC 从公共 RPC 配置模板中合并配置到 origin 中
func (T *SitePlugins) ConfigSitePluginRPC(origin *SitePlugin, handle func(name string, dsc, src reflect.Value) bool) bool {
	if origin == nil {
		return false
	}
	c, ok := T.RPC[origin.PublicName]
	if !ok {
		return false
	}

	// 深度复制副本，隔离原公共模板中 TLS 等指针，规避并发数据竞争
	cCloned := c.clone()
	if builtin.CopyStructDeep(cCloned, origin, configExclude(handle)) == nil {
		*origin = *cCloned
		return true
	}
	return false
}

// SiteHeaderType 配置-标头-类型
type SiteHeaderType struct {
	Header      map[string][]string // Header
	PageExpired int64               // 页面过期(秒单位)
}

// SiteHeader 配置-标头
type SiteHeader struct {
	// 引用公共配置后，该结构中的字段如果也有设置，将会优先使用。
	PublicName      string                    // 引用公共配置的名字
	Static, Dynamic map[string]SiteHeaderType // 静态 / 动态 Header，key 形如 ".html"
	MIME            map[string]string         // MIME 类型
}

// SiteDirectory 配置-目录
type SiteDirectory struct {
	Root    string   // 主目录
	Virtual []string // 虚目录
}

// RootDir 根目录
//
//	r *http.Request	    		请求
//	string			    		根目录路径
func (T *SiteDirectory) RootDir(upath string) string {
	var (
		p         = filepath.Clean(upath) // r.URL.Path
		root      = filepath.FromSlash(T.Root)
		separator = string(filepath.Separator)
	)

	for _, v := range T.Virtual {
		if v == "" {
			continue
		}
		v = filepath.FromSlash(v)
		pos := strings.LastIndex(v, separator)
		if strings.HasPrefix(p+separator, separator+v[pos+1:]+separator) {
			if pos == 0 {
				pos = 1
			}
			root = v[:pos]
			break
		}
	}
	return root
}

// SiteLogLevel 配置-日志-级别
type SiteLogLevel int

const (
	SiteLogLevelDisable SiteLogLevel = iota // 禁用日志记录，默认不开启
)

// SiteLog 配置-日志，这个功能后面待加。
type SiteLog struct {
	Level     SiteLogLevel // 级别
	Directory string       // 目录
}

// SiteSession 配置-会话
type SiteSession struct {
	// 引用公共配置后，该结构中的字段如果也有设置，将会优先使用。
	PublicName string // 引用公共配置的名字
	Name       string // 会话名称
	Expired    int64  // 过期时间（秒单位，默认 20 分钟）
	Size       int    // 会话 ID 长度（默认 40）
	Salt       string // 加盐（伪随机数补充熵，可为空）
}

// SiteDynamic 动态相关配置
type SiteDynamic struct {
	// 引用公共配置后，该结构中的字段如果也有设置，将会优先使用。
	PublicName string // 引用公共配置的名字

	Ext                []string // 动态文件后缀
	Cache              bool     // 动态文件缓存解析，非缓存执行
	CacheParseTimeout  int64    // 动态文件缓存解析超时，（秒为单位）
	CacheStaticFileDir string   // 缓存静态文件目录，仅适于 markdown 转 HTML
	CacheStaticTimeout int64    // 缓存静态超时，（秒为单位）
}

// Site 配置-站点
type Site struct {
	Status   bool   // 状态，是否启动此站点
	Name     string // 站点别名
	Identity string // 站点维一码，可以说是池名

	Host      []string                // 域名绑定
	Forward   map[string]SiteForwards // 转发
	Plugin    SitePlugins             // 插件
	Directory SiteDirectory           // 目录

	IndexFile []string // 默认页
	Dynamic   SiteDynamic

	Header    SiteHeader        // HTTP头
	Log       SiteLog           // 日志
	ErrorPage map[string]string // 错误页

	Session SiteSession // 会话
}

// SitePublic 公共站点配置
type SitePublic struct {
	Header  map[string]SiteHeader
	Session map[string]SiteSession
	Plugin  SitePlugins
	Forward map[string]SiteForwards
	Dynamic map[string]SiteDynamic
}

// ConfigSiteSession 将公共 Session 配置合并到 origin（公共为底，origin 非零字段覆盖）。
func (T *SitePublic) ConfigSiteSession(origin *SiteSession, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}
	c, ok := T.Session[origin.PublicName]
	if !ok {
		return false
	}
	if builtin.CopyStructDeep(&c, origin, handle) != nil {
		return false
	}
	*origin = c
	return true
}

// ConfigSiteHeader 将公共 Header 配置合并到 origin（公共为底，origin 非零字段覆盖）。
func (T *SitePublic) ConfigSiteHeader(origin *SiteHeader, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}
	c, ok := T.Header[origin.PublicName]
	if !ok {
		return false
	}
	if builtin.CopyStructDeep(&c, origin, handle) != nil {
		return false
	}
	*origin = c
	return true
}

// ConfigSiteForward 将公共 Forward 列表追加到 origin.List。
// 与其它 Config* 不同：采用追加语义（公共规则在前或后由调用方 List 初始内容决定），
// 且不覆盖 origin 已有项。合并时会复制元素并清空运行时字段 forwardWriter，避免共享可变状态。
// handle 保留签名以兼容既有调用方；转发列表为切片追加，不走字段级深拷贝过滤。
func (T *SitePublic) ConfigSiteForward(origin *SiteForwards, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}
	c, ok := T.Forward[origin.PublicName]
	if !ok || len(c.List) == 0 {
		return ok // 找到但为空列表时仍视为“引用成功”，与旧行为接近
	}
	// 在锁内完成切片拷贝，避免解锁后公共配置被并发修改导致读到不一致数据
	srcList := c.List
	copied := make([]SiteForward, len(srcList))
	for i := range srcList {
		copied[i] = srcList[i]
		// 运行时 rewriter 不应随配置合并共享
		copied[i].forwardWriter = nil
		// Path / ExcludePath 为切片，浅拷贝 header 后仍共享底层数组；
		// 配置通常只读，若需完全隔离可再 copy。此处做防御性拷贝以提升并发安全。
		if n := len(copied[i].Path); n > 0 {
			copied[i].Path = append([]string(nil), srcList[i].Path...)
		}
		if n := len(copied[i].ExcludePath); n > 0 {
			copied[i].ExcludePath = append([]string(nil), srcList[i].ExcludePath...)
		}
	}
	origin.List = append(origin.List, copied...)
	return true
}

// ConfigSiteDynamic 将公共 Dynamic 配置合并到 origin。
// 规则：以公共配置为底，origin 中非零字段覆盖公共配置，结果写回 origin。
// 返回 true 表示成功合并；PublicName 为空、未找到或拷贝失败时返回 false。
func (T *SitePublic) ConfigSiteDynamic(origin *SiteDynamic, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}
	c, ok := T.Dynamic[origin.PublicName]
	if !ok {
		return false
	}
	if builtin.CopyStructDeep(&c, origin, handle) != nil {
		return false
	}
	*origin = c
	return true
}

// ConfigSitePlugin 按 kind（"rpc" / "http"，大小写不敏感）与 PublicName 合并公共插件配置到 origin。
// 补全原结构体存在 Plugin 字段却无对应合并入口的功能盲区。
// 未找到或参数非法时返回 false。
func (T *SitePublic) ConfigSitePlugin(origin *SitePlugin, kind string, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}
	var (
		c  SitePlugin
		ok bool
	)
	switch {
	case equalFoldASCII(kind, "rpc"):
		c, ok = T.Plugin.RPC[origin.PublicName]
	case equalFoldASCII(kind, "http"):
		c, ok = T.Plugin.HTTP[origin.PublicName]
	default:
		return false
	}
	if !ok {
		return false
	}
	if builtin.CopyStructDeep(&c, origin, handle) != nil {
		return false
	}
	*origin = c
	return true
}

// equalFoldASCII 仅处理 ASCII 的大小写不敏感比较，避免引入 strings 包额外依赖路径差异，跨平台行为一致。
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Sites 站点集合
type Sites struct {
	Public SitePublic
	Site   []Site // 站点
}

// ServerTLSFile 证书文件对
type ServerTLSFile struct {
	CertFile, KeyFile string // 证书，key 文件地址
}

// ServerTLS TLS 相关配置
type ServerTLS struct {
	RootCAs                     []ServerTLSFile // 服务端证书文件
	NextProtos                  []string        // http版本
	CipherSuites                []uint16        // 密码套件（仅影响 TLS 1.0–1.2；TLS 1.3 由运行时固定安全套件处理，不可配置）
	SessionTicketsDisabled      bool            // 设置为 true 可禁用会话票证 (恢复) 支持。
	SetSessionTicketKeys        [][32]byte      // 会话恢复票证
	DynamicRecordSizingDisabled bool            // 禁用TLS动态记录自适应大小
	MinVersion                  uint16          // 最小SSL/TLS版本。如果为零，则SSLv3的被取为最小（现代 Go 实际最低为 TLS 1.0，推荐 TLS 1.2+）。
	MaxVersion                  uint16          // 最大SSL/TLS版本。如果为零，则该包所支持的最高版本被使用。
	ClientCAs                   []string        // 客户端拥有的“权威组织”证书的列表。(Server/Client端使用)
}

// CipherSuitesAuto 根据 MinVersion/MaxVersion 自动填充安全密码套件。
// 仅填充 TLS 1.0–1.2 套件；TLS 1.3 套件由 crypto/tls 自动管理且不可配置。
// 注意：若多个 Server 共享同一 *ServerTLS 指针，首次并发调用存在竞态窗口，
// 建议在配置加载阶段单线程调用。方法本身对已填充的 CipherSuites 直接返回。
func (T *ServerTLS) CipherSuitesAuto() {
	if T == nil {
		return
	}
	if T.MaxVersion == 0 {
		T.MaxVersion = tls.VersionTLS13
	}
	// 防止版本倒挂导致空列表
	if T.MinVersion > T.MaxVersion {
		T.MinVersion = T.MaxVersion
	}

	if len(T.CipherSuites) > 0 {
		return // 已有手动配置或已被填充，不覆盖
	}

	// 预分配并本地构建，最后一次性赋值，缩小并发窗口
	suites := tls.CipherSuites()
	out := make([]uint16, 0, len(suites))
	for _, cs := range suites {
		if cs == nil || cs.Insecure {
			continue
		}
		for _, version := range cs.SupportedVersions {
			if version >= T.MinVersion && version <= T.MaxVersion {
				out = append(out, cs.ID)
				break
			}
		}
	}
	T.CipherSuites = out
}

// Server 服务器参数
type Server struct {
	// 引用公共配置后，该结构中的 CC 和 CS 如果也有设置，将会优先使用。
	PublicName                   string     // 引用公共配置的名字
	ReadTimeout                  int64      // 设置读取超时(毫秒单位)
	WriteTimeout                 int64      // 设置写入超时(毫秒单位)
	ReadHeaderTimeout            int64      // 读取标头超时(毫秒单位）
	IdleTimeout                  int64      // 保持连接空闲超时，如果为0，使用 ReadTimeout,(毫秒单位）
	MaxHeaderBytes               int        // 如果0，最大请求头的大小，http.DefaultMaxHeaderBytes
	KeepAlivesEnabled            bool       // 支持客户端Keep-Alive
	ShutdownConn                 bool       // 服务器关闭监听，不会即时关闭正在下载的连接。空闲后再关闭。(默认即时关闭)
	DisableGeneralOptionsHandler bool       // 如果为真，将“OPTIONS *”请求传递给处理程序，否则响应 200 OK 和 Content-Length: 0。
	TLS                          *ServerTLS // TLS
}

// Conn 连接相关参数
type Conn struct {
	// 引用公共配置后，该以结构中的CC和CS如果也有设置，将会使用优先使用。
	PublicName      string // 引用公共配置的名字
	Deadline        int64  // 设置读写超时(毫秒单位)
	WriteDeadline   int64  // 设置写入超时(毫秒单位)
	ReadDeadline    int64  // 设置读取超时(毫秒单位)
	KeepAlive       bool   // 即使没有任何通信，一个客户端可能希望保持连接到服务器的状态。
	KeepAlivePeriod int64  // 保持连接超时(毫秒单位)
	Linger          int    // 连接关闭后，等待发送或待确认的数据（秒单位)。如果 sec > 0，经过sec秒后，所有剩余的未发送数据都可能会被丢弃。则与sec < 0 一样在后台发送数据。
	NoDelay         bool   // 设置操作系统是否延迟发送数据包,建议设置为true(无延迟)
	ReadBuffer      int    // 在缓冲区读取数据大小
	WriteBuffer     int    // 写入数据到缓冲区大小
}

// ServerPublic 公共服务器配置
type ServerPublic struct {
	CC map[string]Conn   // 连接设置
	CS map[string]Server // 服务器设置
}

// ConfigConn 将公共连接配置合并到 origin（origin 非零字段优先）。
// 返回 true 表示成功合并。并发安全（读锁保护 map 查找）。
func (T *ServerPublic) ConfigConn(origin *Conn, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}

	c, ok := T.CC[origin.PublicName]
	if !ok {
		return false
	}

	// 以公共配置为基，origin 非零字段覆盖
	if builtin.CopyStructDeep(&c, origin, configExclude(handle)) == nil {
		*origin = c
		return true
	}
	return false
}

// ConfigServer 优化版本（关键改动）
func (T *ServerPublic) ConfigServer(origin *Server, handle func(name string, dsc, src reflect.Value) bool) bool {
	if T == nil || origin == nil || origin.PublicName == "" {
		return false
	}

	c, ok := T.CS[origin.PublicName]
	if !ok {
		return false
	}

	// 对嵌套的 TLS 指针做值拷贝，避免共享修改
	if c.TLS != nil {
		tlsCopy := *c.TLS // 值拷贝
		c.TLS = &tlsCopy
	}

	// 以公共配置副本为基进行合并
	if builtin.CopyStructDeep(&c, origin, configExclude(handle)) != nil {
		return false
	}

	*origin = c

	// 现在 Auto 只修改副本，不会影响公共配置
	if origin.TLS != nil && len(origin.TLS.CipherSuites) == 0 {
		origin.TLS.CipherSuitesAuto()
	}
	return true
}

// Listen 监听配置
type Listen struct {
	Status bool   // 状态，是否启动此服务器
	CC     Conn   // 连接设置
	CS     Server // 服务器设置
}

// Servers 服务器集合
type Servers struct {
	Public ServerPublic
	Listen map[string]Listen
}

// Config 配置
type Config struct {
	Servers Servers      // 服务器集
	Sites   Sites        // 站点集
	mu      sync.RWMutex // 保护并发解析与修改（未导出）
}

// ParseFile 解析服务器配置文件，一个 JSON 格式的文件。
//
//	参：
//	  file string     文件
//	返：
//	  error           错误，如果文件无法打开，或无法解析的情况
func (T *Config) ParseFile(file string) error {
	T.mu.Lock()
	defer T.mu.Unlock()
	return T.parseFileLocked(file)
}

// ParseFiles 先加载主配置，再自动加载同目录下所有 .site.json / .listen.json 子配置。
func (T *Config) ParseFiles(file string) error {
	T.mu.Lock()
	defer T.mu.Unlock()

	// 加载主配置文件
	if err := T.parseFileLocked(file); err != nil {
		return err
	}

	// 加载子配置文件（仅处理常规文件且后缀匹配，避免无效 I/O）
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read config dir %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".site.json") && !strings.HasSuffix(name, ".listen.json") {
			continue
		}
		subPath := filepath.Join(dir, name)
		if err := loadSubConf(subPath, T); err != nil {
			return err
		}
	}
	return nil
}

// ParseReader 解析服务器配置数据，一个 JSON 格式的数据。
//
//	参：
//	  r   io.Reader       读接口
//	返：
//	  error               错误，如果无法解析的情况
func (T *Config) ParseReader(r io.Reader) error {
	T.mu.Lock()
	defer T.mu.Unlock()

	if err := json.NewDecoder(r).Decode(T); err != nil {
		return fmt.Errorf("decode config reader: %w", err)
	}
	T.ensureMaps()
	return nil
}

// parseFileLocked 内部使用（调用方必须已持有写锁）
func (T *Config) parseFileLocked(file string) error {
	// 优先判断加载子配置文件
	if strings.HasSuffix(file, ".site.json") || strings.HasSuffix(file, ".listen.json") {
		return loadSubConf(file, T)
	}

	// 加载主配置文件
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("open config file %q: %w", file, err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(T); err != nil {
		return fmt.Errorf("decode config file %q: %w", file, err)
	}
	T.ensureMaps()
	return nil
}

// ensureMaps 保证本包会写入的 map 已初始化，防止 nil map 赋值 panic
func (T *Config) ensureMaps() {
	if T.Servers.Listen == nil {
		T.Servers.Listen = make(map[string]Listen)
	}
}

// configHTTPClient 配置HTTP插件客户端。
//
// 并发说明：
//   - 本函数本身不保证并发安全，调用方需确保同一 *vweb.PluginHTTPClient 不被并发配置。
//   - 配置完成后，*http.Transport 是并发安全的，可被多个 goroutine 共享使用。
//   - 所有切片与 Header 均做了独立拷贝，避免与 conf 共享底层数据产生数据竞争。
func configHTTPClient(c *vweb.PluginHTTPClient, conf *SitePlugin) error {
	if c == nil || conf == nil {
		return fmt.Errorf("configHTTPClient: c or conf is nil")
	}

	c.Addr = conf.Addr
	c.Host = conf.Host
	c.Scheme = conf.Scheme

	// ---------- Dialer 配置 ----------
	if c.Dialer == nil {
		c.Dialer = new(net.Dialer)
	}

	if conf.LocalAddr != "" {
		// 支持 "IP:port"、"host:port" 或纯 IP/主机名（自动补端口 0）。
		// 跨平台兼容：优先 tcp，失败再尝试 tcp4/tcp6。
		addr := conf.LocalAddr
		if _, _, err := net.SplitHostPort(addr); err != nil {
			// 无端口时补 :0（兼容 IPv4/IPv6 字面量及主机名）
			addr = net.JoinHostPort(addr, "0")
		}

		var (
			netTCPAddr *net.TCPAddr
			lastErr    error
		)
		for _, network := range []string{"tcp", "tcp4", "tcp6"} {
			resolved, err := net.ResolveTCPAddr(network, addr)
			if err == nil {
				netTCPAddr = resolved
				lastErr = nil
				break
			}
			lastErr = err
		}
		if lastErr != nil {
			return fmt.Errorf("ConfigSitePlugin.LocalAddr 地址无法解析(%s)。格式应为 111.222.333.444:0 或 www.example.com:0，错误: %v", conf.LocalAddr, lastErr)
		}
		c.Dialer.LocalAddr = netTCPAddr
	}

	// 超时单位统一为毫秒；零值表示使用系统/Go 默认行为（不强制覆盖）。
	c.Dialer.Timeout = time.Duration(conf.Timeout) * time.Millisecond
	c.Dialer.KeepAlive = time.Duration(conf.KeepAlive) * time.Millisecond
	c.Dialer.FallbackDelay = time.Duration(conf.FallbackDelay) * time.Millisecond

	// ---------- Transport 配置 ----------
	if c.Tr == nil {
		c.Tr = new(http.Transport)
		// 默认从环境变量读取代理（HTTP_PROXY / HTTPS_PROXY / NO_PROXY）。
		c.Tr.Proxy = http.ProxyFromEnvironment
	}

	if conf.ProxyURL != "" {
		u, err := url.Parse(conf.ProxyURL)
		if err != nil {
			return fmt.Errorf("代理地址无效 ConfigSitePlugin.ProxyURL(%s): %v", conf.ProxyURL, err)
		}
		c.Tr.Proxy = http.ProxyURL(u)
	}

	c.Tr.DisableKeepAlives = conf.DisableKeepAlives
	c.Tr.DisableCompression = conf.DisableCompression
	c.Tr.MaxIdleConns = conf.IdeConn
	c.Tr.MaxIdleConnsPerHost = conf.MaxIdleConnsPerHost
	c.Tr.MaxConnsPerHost = conf.MaxConnsPerHost
	c.Tr.MaxResponseHeaderBytes = conf.MaxResponseHeaderBytes
	c.Tr.ReadBufferSize = conf.ReadBufferSize
	c.Tr.WriteBufferSize = conf.WriteBufferSize
	c.Tr.ForceAttemptHTTP2 = conf.ForceAttemptHTTP2

	// 仅在配置了非零值时才覆盖默认超时，避免意外将超时设为 0。
	if d := conf.ResponseHeaderTimeout; d != 0 {
		c.Tr.ResponseHeaderTimeout = time.Duration(d) * time.Millisecond
	}
	if d := conf.ExpectContinueTimeout; d != 0 {
		c.Tr.ExpectContinueTimeout = time.Duration(d) * time.Millisecond
	}
	if d := conf.IdleConnTimeout; d != 0 {
		c.Tr.IdleConnTimeout = time.Duration(d) * time.Millisecond
	}
	if d := conf.TLSHandshakeTimeout; d != 0 {
		c.Tr.TLSHandshakeTimeout = time.Duration(d) * time.Millisecond
	}

	// ProxyConnectHeader 使用 Clone，防止外部修改影响已配置的 Transport（并发安全）。
	if len(conf.ProxyConnectHeader) != 0 {
		c.Tr.ProxyConnectHeader = conf.ProxyConnectHeader.Clone()
	}

	// ---------- TLS 配置 ----------
	// 只要 conf.TLS 非空就创建配置（原逻辑强制要求 ServerName 非空，导致其他 TLS 字段失效）。
	var tlsConfig *tls.Config
	if conf.TLS != nil {
		tlsConfig = &tls.Config{
			ServerName:         conf.TLS.ServerName,
			InsecureSkipVerify: conf.TLS.InsecureSkipVerify,
		}

		// 正确复制切片：使用 append 分配独立底层数组，避免 copy 在 nil 切片上失效，以及数据竞争。
		if len(conf.TLS.NextProtos) > 0 {
			tlsConfig.NextProtos = append([]string(nil), conf.TLS.NextProtos...)
		}
		if len(conf.TLS.CipherSuites) > 0 {
			tlsConfig.CipherSuites = append([]uint16(nil), conf.TLS.CipherSuites...)
		}
		// CipherSuites 为 nil 时由 crypto/tls 使用内部默认安全套件，无需显式赋值。
		if len(conf.TLS.CurvePreferences) > 0 {
			tlsConfig.CurvePreferences = append([]tls.CurveID(nil), conf.TLS.CurvePreferences...)
		}

		// ClientSessionCache：仅正数时启用；0 或负数均不创建缓存。
		if conf.TLS.ClientSessionCache > 0 {
			tlsConfig.ClientSessionCache = tls.NewLRUClientSessionCache(conf.TLS.ClientSessionCache)
		}

		// 根证书池：优先使用系统证书，并 Clone 后再追加用户证书，避免修改全局系统池导致并发问题。
		if sysPool, err := x509.SystemCertPool(); err == nil && sysPool != nil {
			tlsConfig.RootCAs = sysPool.Clone()
		} else {
			tlsConfig.RootCAs = x509.NewCertPool()
		}

		for _, filename := range conf.TLS.RootCAs {
			caData, err := os.ReadFile(filename)
			if err != nil {
				return verror.TrackErrorf("%s: %s", filename, err.Error())
			}

			// 扩展名大小写不敏感，兼容 Windows / macOS / Linux 文件系统差异。
			ext := strings.ToLower(filepath.Ext(filename))
			switch ext {
			case ".cer":
				certificates, err := x509.ParseCertificates(caData)
				if err != nil {
					return fmt.Errorf("%s: %s", filename, err.Error())
				}
				for _, cert := range certificates {
					tlsConfig.RootCAs.AddCert(cert)
				}
			case ".pem", ".crt":
				if !tlsConfig.RootCAs.AppendCertsFromPEM(caData) {
					return fmt.Errorf("%s: not a valid PEM format", filename)
				}
			default:
				return fmt.Errorf("TLS.RootCAs[\"%s\"]: unsupported file type, only \".cer/.crt/.pem\" are supported", filename)
			}
		}
	}
	c.Tr.TLSClientConfig = tlsConfig

	return nil
}

// configRPCClient 快速的配置RPC客户端。
// 该函数是一次性配置操作，调用方应保证同一 *vweb.PluginRPCClient 不会被并发配置。
// 连接池的并发安全与资源回收由 vconnpool.Pool 自身保证。
func configRPCClient(c *vweb.PluginRPCClient, conf *SitePlugin) error {
	if c == nil {
		return fmt.Errorf("configRPCClient: PluginRPCClient 不能为 nil")
	}
	if conf == nil {
		return fmt.Errorf("configRPCClient: SitePlugin 配置不能为 nil")
	}

	// 基本字段赋值
	c.Addr = conf.Addr
	c.Path = conf.Path

	// 确保连接池存在
	if c.ConnPool == nil {
		c.ConnPool = new(vconnpool.Pool)
	}

	// 空闲连接数与最大连接数（负值视为 0，表示不限制或禁用复用）
	idleConn := max(conf.IdeConn, 0)
	maxConn := max(conf.MaxConn, 0)
	c.ConnPool.IdleConn = idleConn
	c.ConnPool.MaxConn = maxConn

	// 确保 Dialer 可用。仅在为 nil 时创建标准 net.Dialer；
	// 若已存在非 *net.Dialer 实现，则拒绝设置超时/本地地址，避免静默破坏用户自定义行为。
	if c.ConnPool.Dialer == nil {
		c.ConnPool.Dialer = new(net.Dialer)
	}

	d, ok := c.ConnPool.Dialer.(*net.Dialer)
	if !ok {
		return fmt.Errorf("configRPCClient: ConnPool.Dialer 已存在且不是 *net.Dialer，无法安全设置 Timeout/KeepAlive/LocalAddr/FallbackDelay")
	}

	// 设置本地拨号地址（跨平台支持 IPv4/IPv6/域名，纯 IP 或无端口时自动补 :0）
	if conf.LocalAddr != "" {
		localAddr := conf.LocalAddr
		// 判断是否已包含端口：SplitHostPort 失败即表示无端口（或格式不完整）
		if _, _, err := net.SplitHostPort(localAddr); err != nil {
			localAddr = net.JoinHostPort(localAddr, "0") // 自动处理 IPv6 方括号
		}

		netTCPAddr, err := net.ResolveTCPAddr("tcp", localAddr)
		if err != nil {
			return fmt.Errorf("ConfigSitePlugin.LocalAddr 地址无法解析(%s)：%w。格式应为 IP:0、[IPv6]:0 或 domain:0", conf.LocalAddr, err)
		}
		d.LocalAddr = netTCPAddr
	}

	// 超时相关设置（毫秒 → time.Duration，负值钳制为 0）
	// Timeout/KeepAlive 为 0 表示无超时；FallbackDelay 为 0 时保留 net.Dialer 内部默认 300ms（双栈 Happy Eyeballs）
	timeout := max(conf.Timeout, 0)
	d.Timeout = time.Duration(timeout) * time.Millisecond

	keepAlive := max(conf.KeepAlive, 0)
	d.KeepAlive = time.Duration(keepAlive) * time.Millisecond

	fallbackDelay := max(conf.FallbackDelay, 0)
	d.FallbackDelay = time.Duration(fallbackDelay) * time.Millisecond

	return nil
}

// parseConf 读取并反序列化单个 JSON 文件
func parseConf(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read sub-config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal sub-config %q: %w", path, err)
	}
	return nil
}

// loadSubConf 加载并合并子配置（.site.json / .listen.json）
// 注意：调用方必须已持有 Config.mu 写锁。
func loadSubConf(path string, conf *Config) error {
	switch {
	case strings.HasSuffix(path, ".site.json"):
		var incoming []Site
		if err := parseConf(path, &incoming); err != nil {
			return err
		}
		if len(incoming) == 0 {
			return nil
		}

		// 为了在保持「按顺序替换第一个匹配」语义的同时提升性能，
		// 使用 Identity → 当前索引的映射。Identity 被设计为唯一码，
		// 若出现重复，后出现的会覆盖先前的索引（与常见配置预期一致）。
		identityIndex := make(map[string]int, len(conf.Sites.Site))
		for i, s := range conf.Sites.Site {
			identityIndex[s.Identity] = i
		}

		var toAppend []Site
		for _, s := range incoming {
			if idx, exists := identityIndex[s.Identity]; exists {
				// 替换已存在的站点（同一 Identity 以最后一次为准）
				conf.Sites.Site[idx] = s
			} else {
				toAppend = append(toAppend, s)
			}
		}
		if len(toAppend) > 0 {
			conf.Sites.Site = append(conf.Sites.Site, toAppend...)
		}

	case strings.HasSuffix(path, ".listen.json"):
		var incoming map[string]Listen
		if err := parseConf(path, &incoming); err != nil {
			return err
		}
		if len(incoming) == 0 {
			return nil
		}
		if conf.Servers.Listen == nil {
			conf.Servers.Listen = make(map[string]Listen, len(incoming))
		}
		for k, v := range incoming {
			conf.Servers.Listen[k] = v
		}
	}
	return nil
}
