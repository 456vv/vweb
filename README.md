# vweb [![Build Status](https://travis-ci.org/456vv/vweb.svg?branch=master)](https://travis-ci.org/456vv/vweb)
golang vweb, 简单的web服务器。


列表：
-----------------------------------
# **列表：**
```go
vweb.go======================================================================================================================
const (
    Version                 string = "VWEB/2.4.20181117"                                    // 版本号
>    defaultDataBufioSize    int64 = 32*1024                                                 // 默认数据缓冲32MB
)

var DotFuncMap      = make(map[string]map[string]interface{})                               // 点函数映射

>type atomicBool int32
>func (T *atomicBool) isTrue() bool 	{ return atomic.LoadInt32((*int32)(T)) != 0 }
>func (T *atomicBool) isFalse() bool	{ return atomic.LoadInt32((*int32)(T)) != 1 }
>func (T *atomicBool) setTrue() bool	{ return !atomic.CompareAndSwapInt32((*int32)(T), 0, 1)}
>func (T *atomicBool) setFalse() bool{ return !atomic.CompareAndSwapInt32((*int32)(T), 1, 0)}

func.go======================================================================================================================
func ExtendDotFuncMap(deputy map[string]map[string]interface{})                             // 扩展点函数映射，在模板上的点（.）可以调用
>func derogatoryDomain(host string, f func(string) bool)                                     // 贬域名
>func equalDomain(host, domain string) (ok bool)                                             // 贬域名比较
func GenerateRandomId(rnd []byte) error                                                     // 生成随机ID
func GenerateRandom(length int) ([]byte, error)												// 生成随机ID
func GenerateRandomString(length int) (string, error)										// 生成随机ID
func AddSalt(rnd []byte, salt string) string 												// 加盐
func PagePath(root, p string, index []string) (os.FileInfo, string, error)   				// 页路径
TemplateFuncMap.go======================================================================================================================
var TemplateFuncMap         = map[string]interface{...}                                     // 模板函数映射（默认）
>func templateFuncMapError(v interface{}) error                                              // 判断模板中的错误类型

reflect.go======================================================================================================================
func ForMethod(x interface{}) string                                                        // 遍历方法
func ForType(x interface{}) string                                                          // 遍历字段
func TypeSelect(v reflect.Value) interface{}                                                // 类型选择
func InDirect(v reflect.Value) reflect.Value                                                // 指针到内存
func DepthField(s interface{}, index ... string) (field interface{}, err error)             // 深入读取字段

cookie.go======================================================================================================================
type Cookier interface {                                                            // Cookie接口
    ReadAll() map[string]string                                                            // 增加
    RemoveAll()                                                                            // 删除
    Get(name string) string                                                                // 读取
    Add(name, value, path, domain string, maxAge int, secure, only bool)                   // 读出所有
    Del(name string)                                                                       // 删除所有
}
type Cookie struct{                                                                 // Cookie
    R   *http.Request                                                                       //请求
    W   http.ResponseWriter                                                                 //响应
}
    func (c *Cookie) Add(name, value, path, domain string, maxAge int, secure, only bool)   // 增加
    func (c *Cookie) Del(name string)                                                       // 删除
    func (c *Cookie) Get(name string) string                                                // 读取
    func (c *Cookie) ReadAll() map[string]string                                            // 读出所有
    func (c *Cookie) RemoveAll()                                                            // 删除所有

global.go======================================================================================================================
type Globaler interface {                                                            // Global接口（动态页中使用）
    Set(key, val interface{})                                                               // 设置
    Has(key interface{}) bool                                                               // 检查
    Get(key interface{}) interface{}                                                        // 读取
    Del(key interface{})                                                                    // 删除
    Reset()                                                                                 // 重置
}

Session.go======================================================================================================================
type Sessioner interface {                                                          // Sessione接口（动态页中使用）
    Set(key, val interface{})                                                               // 设置
    Has(key interface{}) bool                                                               // 检查
    Get(key interface{}) interface{}                                                        // 读取
    GetHas(key interface{}) (val interface{}, ok bool)                                      // 检查+读取
    Del(key interface{})                                                                    // 删除
    Reset()                                                                                 // 重置
    Defer(call interface{}, arg ...interface{}) error                                       // 过期调用函数
    Free()																					// 释放调用函数
}
>type deferFunc struct {                                                             // 过期函数
>    fun         reflect.Value                                                               // 函数
>    arg         []reflect.Value                                                             // 参数
>    argVariadic bool                                                                        // 有可变参数
>}
type Session struct{                                                                // 会话用于用户保存数据
    *vmap.Map                                                                               // 数据，用户存储的数据
>    id          string                                                                      // id，给Sessions使用的
>    expCall     []*deferFunc                                                                // 记录每个用户的函数
}
    func NewSession() *Session                                                              // 初始化
    func (s *Session) Defer(call interface{}, args ... interface{}) error                   // 会话过期后，调用函数
    func (s *Session) Free()                                                                // 执行结束Defer

sessions.go======================================================================================================================
>type manageSession struct{                                                          // 管制会话有效期
>    s       *Session                                                                        // 会话
>    recent  time.Time                                                                       // 最近访问时间
>}
type Sessions struct{                                                               // Sessions集
    Expired         time.Duration                                                           // 保存Session时间长（默认：20分钟）
    Name            string                                                                  // 标识名称(默认:BWID)
    Size            int                                                                     // 会话ID长度(默认长度40位)
    Salt            string                                                                  // 加盐，由于计算机随机数是伪随机数。（可默认为空）
    ActivationID    bool                                                                    // 为true，保持会话ID
>    sessions        *vmap.Map                                                               // 集，map[id]*Session
}
>    func newSessions() *Sessions                                                            // 初始化
>    func (T *Sessions) init()                                                               // 初始化
>    func (T *Sessions) update(confSession ConfigSitePropertySession)                        // 配置
    func (T *Sessions) GenerateSessionId() string                                           // 生成ID
	func (T *Sessions) SessionIdSalt(rnd []byte) string									 // 加盐
    func (T *Sessions) GenerateSessionIdSalt() string                                       // 生成ID(加盐)
    func (T *Sessions) SessionId(req *http.Request) (id string, err error)                  // 读取SessionID
	func (T *Sessions) NewSession(id string) *Session                              		 // 读取Session，如果不存在则新建
    func (T *Sessions) GetSession(id string) (*Session, error)                              // 读取Session
    func (T *Sessions) SetSession(id string, s *Session) *Session                           // 写入Session
	func (T *Sessions) DelSession(id string)											 	 // 删除Session
    func (T *Sessions) Session(rw http.ResponseWriter, req *http.Request) Sessioner         // 读出Session
    func (T *Sessions) ProcessDeadAll() []interface{}                                       // 处理用户过期的会话
>    func (T *Sessions) triggerDeadSession(ms *manageSession) (ok bool)                      // 由用户来触发处理Session
>    func (T *Sessions) writeToClient(rw http.ResponseWriter, id string) *Session            // 写入客户端

swap.go======================================================================================================================
type Swaper interface {
    New(key interface{}) *vmap.Map                                                          // 子Map，如果存在，则覆盖
    GetNewMap(key interface{}) *vmap.Map                                                    // 子Map，如果存在，则读取
    GetNewMaps(key ...interface{}) *vmap.Map                                                // 子子Map，如果存在，则读取
    Len() int                                                                               // 长度
    Set(key, val interface{})                                                               // 设置
    Has(key interface{}) bool                                                               // 检查
    Get(key interface{}) interface{}                                                        // 读取
    GetHas(key interface{}) (val interface{}, ok bool)                                      // 检查+读取
    GetOrDefault(key interface{}, def interface{}) interface{}                              // 读取，如果不存在返回默认值
    Index(key ...interface{}) interface{}                                                   // 索引
    IndexHas(key ...interface{}) (interface{}, bool)                                        // 索引判断
    Del(key interface{})                                                                    // 删除
    Dels(keys []interface{})                                                                // 删除多个
    ReadAll() interface{}                                                                   // 读取所有
    Copy(from *vmap.Map, over bool)                                                         // 复制
    WriteTo(mm interface{}) (err error)                                                     // 写入到mm
    ReadFrom(mm interface{}) error                                                          // 从mm读取
    Reset()                                                                                 // 重置
    MarshalJSON() ([]byte, error)                                                           // 编码JSON
    UnmarshalJSON(data []byte) error                                                        // 解码JSON
    String() string                                                                         // 字符串JSON
}

site.go======================================================================================================================
var DefaultSitePool    = NewSitePool()                                                      // 网站池（默认）
type SitePool struct {                                                              // 网站池
	Pool					*vmap.Map                                                       // map[池名]*Site
>    recoverSessionTick      time.Duration                                             	 	// 回收无效会话(默认20会分钟回收一次)
>    setTick					chan bool													// 触发更新回收时间
>    exit                    chan bool                                                      // 退出
>    run						atomicBool													// 已经启动
}
    func NewSitePool() *SitePool                                                            // 池对象
    func (sp *SitePool) SetRecoverSession(d time.Duration)                                  // 设置回收无效的会话
    func (sp *SitePool) Start() error                                                       // 启动池
    func (sp *SitePool) Close() error                                                       // 关闭池
type Site struct {                                                                  // 站点
    Sessions            *Sessions                                                           // 会话集
    Global              Globaler                                                            // Global
    Config              *ConfigSite                                                         // Config
    Plugin              *vmap.Map                                                           // 插件map[type]map[name]interface{}
}
    func NewSite() *Site                                                                    // 站点对象

sites.go======================================================================================================================
var DefaultSites        = NewSites()                                                        // 默认站点
type Sites struct {                                                                 // 站点集
    Host *vmap.Map                                                                          // map[host]*Site
}
    func NewSites() *Sites                                                                  // 站点集对象
    func (ss *Sites) Site(host string) (s *Site, ok bool)                                   // 读出站点

response.go======================================================================================================================
type Responser interface{                                                           // 响应接口
    Write([]byte) (int, error)                                                              // 写入字节
    WriteString(string) (int, error)                                                        // 写入字符串
    ReadFrom(io.Reader) (int64, error)                                                      // 读取并写入
    Redirect(string, int)                                                                   // 转向
    WriteHeader(int)                                                                        // 状态码
    Error(string, int)                                                                      // 错误
    Flush()                                                                                 // 刷新缓冲
    Push(target string, opts *http.PushOptions) error                                       // HTTP/2推送
    Hijack() (net.Conn, *bufio.ReadWriter, error)                                           // 劫持，能双向互相发送信息
}
>type response struct {                                                              // 模本点的响应写入
>    buffSize    int64                                                                       // 写入的缓冲大小
>    r           *http.Request                                                               // 请求
>    w           http.ResponseWriter                                                         // 响应
>    td          *TemplateDot                                                                // 模板点
>}
>    func (T *response) Write(p []byte) (int, error)                                         // 写入正文字节
>    func (T *response) WriteString(s string) (int, error)                                   // 写入正文字符串
>    func (T *response) ReadFrom(src io.Reader) (n int64, err error)                         // 读取写入正文
>    func (T *response) Redirect(urlStr string, code int)                                    // 重定向
>    func (T *response) WriteHeader(code int)                                                // 状态码
>    func (T *response) Error(err string, code int)                                          // 错误
>    func (T *response) Flush()                                                              // 刷新缓冲
>    func (T *response) Push(target string, opts *http.PushOptions) error                    // HTTP/2推送
>    func (T *response) Hijack() (net.Conn, *bufio.ReadWriter, error)                        // 劫持，能双向互相发送信息

tcpKeepAliveListener.go======================================================================================================================
>type tcpKeepAliveListener struct {                                                  // 连接设置
>    *net.TCPListener                                                                        // TCP监听
>    cc          *ConfigConn                                                                 // 配置连接
>}
>    func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error)                         // 新建连接
>    func (ln tcpKeepAliveListener) Close() error                                            // 关闭监听

server.go======================================================================================================================

type Server struct {                                                                // 服务器
    *http.Server                                                                            // http服务器
    Listener            net.Listener                                                        // 监听
>	status				atomicBool															// 已经监听
>    cc                  *ConfigConn                                                         // 连接配置
>    cs                  *ConfigServer                                                       // 服务器配置
}
    func (T *Server) ConfigListener(laddr string, CC *ConfigConn) error                     // 配置连接
    func (T *Server) ConfigServer(CS *ConfigServer) error                                   // 配置服务器
>    func configTLSFile(c *tls.Config, conf *ConfigServerTLS) error                          // 配置TLS
type ServerGroup struct {                                                           // 服务器集群
    ErrorLog            *log.Logger                                                         // 错误日志文件
    SrvMan              *Map                                                                // map[ip:port]*Server
    SitePool            *SitePool                                                           // 站点的池
    Sites               *Sites                                                              // 站点集

>    exit                chan bool															// 退出
>	run					atomicBool															// 服务器启动了
>    backConfigDate      []byte                                                              // 备份配置数据。如果是相同数据，则不更新
>    config              *Config                                                             // 配置
}
    func NewServerGroup() *ServerGroup                                                      // 服务器集群对象
    func (T *ServerGroup) SetServer(laddr string, srv *Server) error                        // 增加一个服务器
    func (T *ServerGroup) GetServer(laddr string) (*Server, bool)                           // 读取一个服务器
    func (T *ServerGroup) SetSitePool(pool *SitePool) error                                 // 设置站点池
    func (T *ServerGroup) SetSites(sites *Sites) error                                      // 设置站点集
>    func (T *ServerGroup) serveHTTP(rw http.ResponseWriter, r *http.Request)                // 处理HTTP
>    func (T *ServerGroup) httpTypeByExtension(ext string, me map[string]string) string      // 文件类型扩展
>    func (T *ServerGroup) httpRootPath(dir *ConfigSiteDirectory, r *http.Request) string    // 根目录
>    func (T *ServerGroup) updatePluginConn(cSite *ConfigSite, site *Site)                   // 更新插件连接池
>    func (T *ServerGroup) updateSiteConfig(hosts *vmap.Map)                                 // 更新站点配置
>    func (T *ServerGroup) updateSitePoolAdd(cSite *ConfigSite)                              // 更新站点池增加
>    func (T *ServerGroup) updateSitePoolDel(names []string)                                 // 更新站点池删除
>    func (T *ServerGroup) getServer(laddr string) *Server                                   // 读取或增加HTTP服务器
>    func (T *ServerGroup) listenStart(laddr string, conf ConfigServers) error               // 启动监听端口
>    func (T *ServerGroup) listenStop(laddr string) (err error)                              // 关闭监听端口
>    func (T *ServerGroup) updateConfigServers(conf map[string]ConfigServers)                // 更新服务器
    func (T *ServerGroup) LoadConfigFile(p string) (conf *Config, ok bool, err error)       // 挂载本地配置文件
    func (T *ServerGroup) UpdateConfig(conf *Config) error                                  // 更新配置并把配置分配到各个地方
>    func (T *ServerGroup) updateConfigSites(conf *ConfigSites) error                        // 更新站点
>    func (T *ServerGroup) serve(laddr string, srv *Server) error                            // 服务器监听
    func (T *ServerGroup) Start() error                                                     // 启动服务集群
    func (T *ServerGroup) Close() error                                                     // 关闭服务集群

>    func strSliceContains(ss []string, s string) bool                                       // 判断切片中是否存在相同（字符串）
>    func httpError(w http.ResponseWriter, errorPage map[string]string, e string, code int)  // http错误

TemplateDot.go======================================================================================================================
type TemplateDoter interface{                                                       // 可以在模本中使用的方法
    PKG(pkg string) map[string]interface{}                                                  // 调用包函数
    Request() *http.Request                                                                 // 用户的请求信息
    RequestLimitSize(l int64) *http.Request                                                 // 请求限制大小
    Header() http.Header                                                                    // 标头
    Response() Responser                                                                    // 数据写入响应
    ResponseWriter() http.ResponseWriter                                                    // 数据写入响应
    Session() Sessioner                                                                     // 用户的会话缓存
    Global() Globaler                                                                       // 全站缓存
    Cookie() Cookier                                                                        // 用户的Cookie
    Swap() Swaper                                                                           // 信息交换
    PluginRPC(name string) (PluginRPC, error)                                               // 插件RPC方法调用
    PluginHTTP(name string) (PluginHTTP, error)                                             // 插件HTTP方法调用
}
type TemplateDot struct {                                                           // 模板点
    Writed              bool                                                                // 模板或动态？
    R                   *http.Request                                                       // 请求
    W                   http.ResponseWriter                                                 // 响应
    BuffSize            int64                                                               // 缓冲块大小
    Site                *Site                                                               // 网站配置
    Exchange            *vmap.Map                                                           // 缓存映射
}
    func (T *TemplateDot) PKG(pkg string) map[string]interface{}                            // 调用包函数
    func (T *TemplateDot) Request() *http.Request                                           // 用户的请求信息
    func (T *TemplateDot) RequestLimitSize(l int64) *http.Request                           // 请求限制大小
    func (T *TemplateDot) Header() http.Header                                              // 标头
    func (T *TemplateDot) Response() Responser                                              // 数据写入响应
    func (T *TemplateDot) ResponseWriter() http.ResponseWriter                              // 数据写入响应
    func (T *TemplateDot) Session() Sessioner                                               // 用户的会话缓存
    func (T *TemplateDot) Global() Globaler                                                 // 全站缓存
    func (T *TemplateDot) Cookie() Cookier                                                  // 用户的Cookie
    func (T *TemplateDot) PluginRPC(name string) (PluginRPC, error)                         // 插件RPC方法调用
    func (T *TemplateDot) PluginHTTP(name string) (PluginHTTP, error)                       // 插件HTTP方法调用
    func (T *TemplateDot) Swap() Swaper                                                     // 信息交换

serverHandlerDynamic.go======================================================================================================================
type ServerHandlerDynamic struct {                                                  // 处理动态页面文件
    RootPath, PagePath  string                                                              // 根目录, 页路径
    BuffSize            int64                                                               // 缓冲块大小
    Site                *Site                                                               // 网站配置
}
    func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request)     // 服务HTTP

serverHandlerDynamicTemplate.go======================================================================================================================
>type shdtHeader struct{                                                             // 标头-模本-处理动态页面文件
>    filePath                []string                                                        // 文件路径, map[文件名或别名]文件路径
>    delimLeft,delimRight    string                                                          // 语法识别符
>}
>    func (h *shdtHeader) openFile(rootPath, pagePath  string) (map[string]string, error)    // 打开文件内容
>
>type serverHandlerDynamicTemplate struct {                                          // 模本-处理动态页面文件
>    rootPath, pagePath  string                                                              // 根目录, 页路径
>    buffSize            int64                                                               // 缓冲块大小
>    site                *Site                                                               // 网站配置
>    buf                 *bufio.Reader                                                       // 数据
>}
>    func (T *serverHandlerDynamicTemplate) serveHTTP(rw http.ResponseWriter, req *http.Request)  // 服务HTTP
>    func (T *serverHandlerDynamicTemplate) parse() (shdtHeader, string, error)              // 解析模本
>    func (T *serverHandlerDynamicTemplate) loadTmpl(delimLeft, delimRight string, t *template.Template, f map[string]string) (*template.Template, error) // 模本载入
>    func (T *serverHandlerDynamicTemplate) format(delimLeft, delimRight, c string) string    // 语法整合
>=
PluginHTTP.go======================================================================================================================
type PluginHTTP interface{                                                          // HTTP插件接口
    ServeHTTP(w http.ResponseWriter, r *http.Request)                                       // 服务HTTP
    RoundTrip(r *http.Request) (resp *http.Response, err error)                             // 代理
    CancelRequest(req *http.Request)                                                        // 取消HTTP请求
    CloseIdleConnections()                                                                  // 关闭空闲连接
    RegisterProtocol(scheme string, rt http.RoundTripper)                                   // 注册新协议
}
func ConfigPluginHTTPClient(c *PluginHTTPClient, config ConfigSitePlugin) (*PluginHTTPClient, error)    // 配置
>func configHTTPClient(c *PluginHTTPClient, config ConfigSitePlugin) (*PluginHTTPClient, error)          // 配置
type PluginHTTPClient struct{                                                       // 插件HTTP客户端
    Tr          *http.Transport                                                             // 传输
    Scheme      string                                                                      // 协议（默认）
    Host        string                                                                      // 请求Host（默认）
    Addr        string                                                                      // 地址（默认）
}
    func (T *PluginHTTPClient) Connection() (PluginHTTP, error)                             // 连接
>type pluginHTTP struct{                                                             // 插件HTTP
>    client  *PluginHTTPClient                                                               // 客户端
>}
>    func (T *pluginHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request)                  // 服务器处理
>    func (T *pluginHTTP) RoundTrip(r *http.Request) (resp *http.Response, err error)        // 单一的HTTP请求
>    func (T *pluginHTTP) fillCompleteURL(r *http.Request)                                   // 补充完整URL
>    func (T *pluginHTTP) CancelRequest(r *http.Request)                                     // 取消HTTP请求
>    func (T *pluginHTTP) CloseIdleConnections(t)                                            // 关闭空闲连接
>    func (T *pluginHTTP) RegisterProtocol(scheme string, rt http.RoundTripper)              // 注册新协议

PluginRPC.go======================================================================================================================
type PluginRPC interface{                                                           // RPC插件接口
    Register(value interface{})                                                             // 注册
    Call(name string, arg interface{}) (*Map, error)                                        // 调用
    Discard() error                                                                         // 弃用连接
    Close() error                                                                           // 关闭连接
}
func ConfigPluginRPCClient(c *PluginRPCClient, config ConfigSitePlugin) (*PluginRPCClient, error)       // 配置
>func configRPCClient(c *PluginRPCClient, config ConfigSitePlugin) (*PluginRPCClient, error)             // 配置
type PluginRPCClient struct {                                                       // 插件RPC客户端
    ConnPool            *vconnpool.ConnPool                                                 // 连接池
    Path                string                                                              // 路径
    Addr                string                                                              // 地址
}
    func(T *PluginRPCClient) Connection() (PluginRPC, error)                                // 连接
>    func connentRPCClient(conn net.Conn, p string) (*rpc.Client, error)                     // 连接
>type pluginRPC struct{                                                              // 插件RPC
>    *rpc.Client                                                                             // 配置端
>    conn    net.Conn                                                                        // 连接
>}
>    func (T *pluginRPC) Register(value interface{})                                         // RPC注册类型，仅用于RPC客户端。默认gob编码
>    func (T *pluginRPC) Call(name string, arg interface{}) (interface{}, error)             // 调用RPC，连接TCP，等待远程返回数据。
>    func (T *pluginRPC) Close() error                                                       // 关闭RPC连接
>    func (T *pluginRPC) Discard() error                                                     // 废弃, RPC这条连接不再回收

serverHandlerStatic.go======================================================================================================================
>type shshRange struct{                                                              // Range-标头-处理静态页面文件
>    seek, length    int64                                                                   // 偏移，长度
>}
>type serverHandlerStaticHeader struct{                                              // 标头-处理静态页面文件
>    fileInfo    os.FileInfo                                                                 // 文件信息
>    wh          http.Header                                                                 // 响应HTTP头
>}
>    func (T *serverHandlerStaticHeader) setETag()                                           // 设置内容不变标识
>    func (T *serverHandlerStaticHeader) etag() string                                       // 内容不变标识
>    func (T *serverHandlerStaticHeader) ranges(ranges string) (r []shshRange, n int64, err error)   // 格式化Range，并过滤无效的
>    func (T *serverHandlerStaticHeader) setLastModified()                                   // 设置文件最后修改时间
>    func (T *serverHandlerStaticHeader) lastModified() string                               // 文件最后修改时间
>    func (T *serverHandlerStaticHeader) setDate()                                           // 设置日期时间
>    func (T *serverHandlerStaticHeader) setContentLength()                                  // 设置文件大小字节
>    func (T *serverHandlerStaticHeader) contentLength() string                              // 文件大小字节
>    func (T *serverHandlerStaticHeader) setAcceptRanges()                                   // 设置Range支持的类型
>    func (T *serverHandlerStaticHeader) setPageExpired(pageExpired int64)                   // 设置页面固定过期时间
>    func (T *serverHandlerStaticHeader) addFixedHeader(h http.Header)                       // 设置配置中的固定Header
type ServerHandlerStatic struct{													// 静态页
    RootPath, PagePath  string          													// 根目录, 页路径
	PageExpired			int64																// 页面过期时间（秒为单位）
	BuffSize			int64																// 缓冲块大小
>    fileInfo        	os.FileInfo         												// 文件基本信息
}
    func (T *ServerHandlerStatic) ServeHTTP(rw http.ResponseWriter, req *http.Request)      // 服务HTTP
>    func (T *ServerHandlerStatic) header(rw http.ResponseWriter, req *http.Request) ([]shshRange, error)  // 处理静态文件的Header 报头
>    func (T *ServerHandlerStatic) body(rw http.ResponseWriter, rangeBlock []shshRange)      // 处理静态文件的 body 数据

route.go======================================================================================================================
type Route struct{																	// 路由器
	HandlerError	func(w http.ResponseWriter, r *http.Request)							// 错误网址访问处理
>	rt       		sync.Map																// 路由表 map[string]
}
	func (T *Route) HandleFunc(url string,  handler func(w http.ResponseWriter, r *http.Request)	// 绑定处理函数
	func (T *Route) ServeHTTP(w http.ResponseWriter, r *http.Request)								// 服务HTTP
config.go======================================================================================================================
type ConfigSiteForward struct {                                     // 转发
    Path        []string                                                    // 多种路径匹配
    ExcludePath []string                                                    // 排除多种路径匹配
    RePath      string                                                      // 重写路径
    Redirection int                                                         // 重定向状态码，默认不转向
    End         bool                                                        // 不进行二次
}
type ConfigSitePluginTLS struct {                                   // 插件-TLS
    ServerName          string                                              // 服务器名称
    InsecureSkipVerify  bool                                                // 跳过证书验证
    NextProtos          []string                                            // TCP 协议，如：http/1.1
    CipherSuites        []uint16                                            // 密码套件的列表。
    ClientSessionCache  int                                                 // 是TLS会话恢复 ClientSessionState 条目的缓存。(Client端使用)
    CurvePreferences    []tls.CurveID                                       // 在ECDHE握手中使用(Client端使用)
    File                []string                                            // 证书文件
}
type ConfigSitePlugin struct {                                      // 插件
    //公共
    Addr                    string                                          // 地址
    LocalAddr               string                                          // 本地拨号IP
    Timeout                 int64                                           // 拨号超时（毫秒单位）
    KeepAlive               int64                                           // 保持连接超时（毫秒单位）
    FallbackDelay           int64                                           // 后退延时，等待双协议栈延时，（毫秒单位，默认300ms）。
    DualStack               bool                                            // 尝试建立多个IPv4和IPv6的连接
    IdeConn                 int                                             // 空闲连接数

    //RPC
    Path                    string                                          // 路径
    MaxConn                 int                                             // 最大连接数

    //HTTP
    Host                    string                                          // Host
    Scheme                  string                                          // 协议
    TLS                     ConfigSitePluginTLS                             // TLS
    TLSHandshakeTimeout     int64                                           // 握手超时（毫秒单位）
    DisableKeepAlives       bool                                            // 禁止长连接
    DisableCompression      bool                                            // 禁止压缩
    MaxIdleConnsPerHost     int                                             // 最大空闲连接每个主机
	MaxConnsPerHost			int												// 最大连接的每个主机
    IdleConnTimeout         int64                                           // 设置空闲连接超时
    ResponseHeaderTimeout   int64                                           // 请求Header超时
    ExpectContinueTimeout   int64                                           // 发送Expect: 100-continue标头的PUT请求超时
    ProxyConnectHeader      http.Header                                     // CONNECT代理请求中 增加标头 map[string][]string
    MaxResponseHeaderBytes  int64                                           // 最大的响应标头限制（字节）
}
type ConfigSitePlugins map[string]ConfigSitePlugin                  // 插件集
type ConfigSiteHeaderType struct {                                  // 标头-类型
    Header          map[string][]string                                     // Header
    PageExpired     int64                                                   // 页面过期(秒单位)
}
type ConfigSiteHeader struct {                                      // 标头
    Static, Dynamic map[string]ConfigSiteHeaderType                         // 静态，动态Header，map[".html"]ConfigSiteHeaderType
    MIME            map[string]string                                       // MIME类型
}
type ConfigSiteDirectory struct {                                   // 目录
    Root       string                                                       // 主目录
    Virtual    []string                                                     // 虚目录
}
type ConfigSiteLogLevel int                                         // 日志-级别
const (
    ConfigSiteLogLevelDisable ConfigSiteLogLevel =   iota                  // 禁用日志记录，默认不开启
)
type ConfigSiteLog struct {                                         // 日志，这个功能后面待加。
    Level       ConfigSiteLogLevel                                         // 级别
    Directory   string                                                     // 目录
}
type ConfigSitePropertySession struct {                             // 会话
    Name            string                                                  // 会话名称
    Expired         int64                                                   // 过期时间(毫秒单位，默认20分钟)
    Size            int                                                     // 会话ID长度(默认长度40位)
    Salt            string                                                  // 加盐，由于计算机随机数是伪随机数。（可默认为空）
    ActivationID    bool                                                    // 为true，表示保留ID。否则重新生成新的ID
}
type ConfigSiteProperty struct {                                    // 性能
    ConnMaxNumber       int64                                               // 连接最大数量
    ConnSpeed           int64                                               // 连接宽带速度
    BuffSize	       int64                                              	// 缓冲区大小
    Session             ConfigSitePropertySession                           // 会话
}
type ConfigSite struct {                                            // 站点
    Status              bool                                                // 状态，是否启动此站点
    Name                string                                              // 网站别名

    Host                []string                                            // 域名绑定
    Forward             map[string][]ConfigSiteForward                      // 转发
    Plugin              map[string]ConfigSitePlugins                        // 插件
    Directory           ConfigSiteDirectory                                 // 目录

    IndexFile           []string                                            // 默认页
    DynamicExt          []string                                            // 动态文件后缀

    Header              ConfigSiteHeader                                    // HTTP头
    Log                 ConfigSiteLog                                       // 日志
    ErrorPage           map[string]string                                   // 错误页

    Property            ConfigSiteProperty                                  // 性能

}
type ConfigSites struct {                                           // 站点池
    Site       []ConfigSite                                                 // 站点
}
type ConfigServerTLSFile struct {                                   // TLS文件
    CertFile, KeyFile   string                                              // 证书，key 文件地址
}
type ConfigServerTLS struct {                                       // TLS
    File                []ConfigServerTLSFile                               // 证书文件
    NextProtos          []string                                            // http版本
    CipherSuites        []uint16                                            // 密码套件
    PreferServerCipherSuites    bool                                        // 控制服务器是否选择客户端的最首选的密码套件
    SessionTicketsDisabled      bool                                        // 设置为 true 可禁用会话票证 (恢复) 支持。
    SessionTicketKey            [32]byte                                    // TLS服务器提供会话恢复
    SetSessionTicketKeys        [][32]byte                                  // 会话恢复票证
    DynamicRecordSizingDisabled bool                                        // 禁用TLS动态记录自适应大小
    MinVersion                  uint16                                      // 最小SSL/TLS版本。如果为零，则SSLv3的被取为最小。
    MaxVersion                  uint16                                      // 最大SSL/TLS版本。如果为零，则该包所支持的最高版本被使用。
}

type ConfigServer struct {                                          // 服务器集设置
    ReadTimeout         int64                                               // 设置读取超时(毫秒单位)
    WriteTimeout        int64                                               // 设置写入超时(毫秒单位)
    ReadHeaderTimeout   int64                                               // 读取标头超时(毫秒单位）
    MaxHeaderBytes      int                                                 // 如果0，最大请求头的大小，http.DefaultMaxHeaderBytes
    KeepAlivesEnabled   bool                                                // 支持客户端Keep-Alive
    ShutdownConn        bool                                                // 服务器关闭监听，不会即时关闭正在下载的连接。空闲后再关闭。(默认即时关闭)
    TLS                 ConfigServerTLS                                     // TLS
}
type  ConfigConn struct{                                            // 服务器集连接
    Deadline            int64                                               // 设置读写超时(毫秒单位)
    WriteDeadline       int64                                               // 设置写入超时(毫秒单位)
    ReadDeadline        int64                                               // 设置读取超时(毫秒单位)
    KeepAlive           bool                                                // 即使没有任何通信，一个客户端可能希望保持连接到服务器的状态。
    KeepAlivePeriod     int64                                               // 保持连接超时(毫秒单位)
    Linger              int                                                 // 数据等待发送或待确认（不太清楚这个功能有什么用？）
    NoDelay             bool                                                // 设置操作系统是否延迟发送数据包,默认是无延迟的
    ReadBuffer          int                                                 // 在缓冲区读取数据大小(字节单位)
    WriteBuffer         int                                                 // 写入数据到缓冲区大小(字节单位)
}
type ConfigServers struct {                                          // 服务器集
    Status              bool                                                // 状态，是否启动此服务器
    CC                  ConfigConn                                          // 连接设置
    CS                  ConfigServer                                        // 服务器设置
}
type Config struct {                                                // 配置
    Servers     map[string]ConfigServers                                    // 服务器集
    Sites       ConfigSites                                                // 站点集
}

func ConfigFileParse(conf *Config, file string) error                       // 解析服务器配置文件，一个JSON格式的文件。
func ConfigDataParse(conf *Config, r io.Reader) error                       // 解析服务器配置数据，一个JSON格式的数据。
```