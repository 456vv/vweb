# vweb [![Build Status](https://travis-ci.org/456vv/vweb.svg?branch=master)](https://travis-ci.org/456vv/vweb)
golang vweb, 简单的web服务器。


# **列表：**
```go
Constants
const (
    Version string = "VWEB/v2.0.0"                                                                                  // 版本号
)
Variables
var DefaultSitePool = NewSitePool()                                                                                 // 网站池（默认）
var TemplateFunc = map[string]interface{...}                                                                        // 模板函数映射
func AddSalt(rnd []byte, salt string) string                                                                        // 加盐，与操作
func CopyStruct(dsc, src interface{}, handle func(name string, dsc, src reflect.Value) bool) error                  // 复制结构
func CopyStructDeep(dsc, src interface{}, handle func(name string, dsc, src reflect.Value) bool) error              // 复制结构深度
func DepthField(s interface{}, index ...interface{}) (field interface{}, err error)                                 // 快速深入读取字段
func ExtendTemplatePackage(pkgName string, deputy map[string]interface{})                                           // 扩展模板的包
func ForMethod(x interface{}) string                                                                                // 遍历方法
func ForType(x interface{}, all bool) string                                                                        // 遍历字段
func GenerateRandom(length int) ([]byte, error)                                                                     // 生成标识符
func GenerateRandomId(rnd []byte) error                                                                             // 生成标识符（字节）
func GenerateRandomString(length int) (string, error)                                                               // 生成标识符(字符)
func InDirect(v reflect.Value) reflect.Value                                                                        // 指针到内存
func PagePath(root, p string, index []string) (os.FileInfo, string, error)                                          // 页路径
type Cookie struct {                                                                                            // cookie
    R *http.Request                                                                                                 //请求
    W http.ResponseWriter                                                                                           //响应
}
    func (c *Cookie) Add(name, value, path, domain string, maxAge int, secure, only bool, sameSite http.SameSite)   // 增加
    func (c *Cookie) Del(name string)                                                                               // 删除
    func (c *Cookie) Get(name string) string                                                                        // 读取
    func (c *Cookie) ReadAll() map[string]string                                                                    // 读取所有
    func (c *Cookie) RemoveAll()                                                                                    // 移除所有Cookie
type Cookier interface {                                                                                        // cookie接口
    ReadAll() map[string]string                                                                                     // 读取所有
    RemoveAll()                                                                                                     // 删除所用
    Get(name string) string                                                                                         // 读取
    Add(name, value, path, domain string, maxAge int, secure, only bool, sameSite http.SameSite)                    // 增加
    Del(name string)                                                                                                // 删除
}
type DotContexter interface {                                                                                   // 点上下文
    Context() context.Context                                                                                       // 上下文
    WithContext(ctx context.Context)                                                                                // 替换上下文
}
type DynamicTemplate interface {                                                                                //  动态模板
    ParseFile(path string) error                                                                                    // 解析文件
    ParseText(content, name string) error                                                                           // 解析文本
    SetPath(rootPath, pagePath string)                                                                              // 设置路径
    Parse(r *bufio.Reader) (err error)                                                                              // 解析
    Execute(out *bytes.Buffer, dot interface{}) error                                                               // 执行
}
type Forward struct {                                                                                           // 转发
    Path        []string                                                                                            // 多种路径匹配
    ExcludePath []string                                                                                            // 排除多种路径匹配
    RePath      string                                                                                              // 重写路径
}
    func (T *Forward) Rewrite(upath string) (rpath string, rewrited bool, err error)                                // 重写
type Globaler interface {                                                                                       // 局部
    Set(key, val interface{})                                                                                       // 设置
    Has(key interface{}) bool                                                                                       // 检查
    Get(key interface{}) interface{}                                                                                // 读取
    Del(key interface{})                                                                                            // 删除
    Reset()                                                                                                         // 重置
}
type PluginHTTP interface {                                                                                     // 插件HTTP
    Type() PluginType                                                                                               // 类型
    ServeHTTP(w http.ResponseWriter, r *http.Request)                                                               // 服务HTTP
    RoundTrip(r *http.Request) (resp *http.Response, err error)                                                     // 代理
    CancelRequest(req *http.Request)                                                                                // 取消HTTP请求
    CloseIdleConnections()                                                                                          // 关闭空闲连接
    RegisterProtocol(scheme string, rt http.RoundTripper)                                                           // 注册新协议
}
type PluginHTTPClient struct {                                                                                  // HTTP客户端
    Tr     *http.Transport                                                                                          // 客户端
    Addr   string                                                                                                   // 地址
    Scheme string                                                                                                   // 协议（用于默认填充）
    Host   string                                                                                                   // 请求Host（用于默认填充）
    Dialer *net.Dialer                                                                                              // 拨号
}
    func (T *PluginHTTPClient) Connection() (PluginHTTP, error)                                                     // 快速连接HTTP
type PluginRPC interface {                                                                                      // 插件RPC
    Type() PluginType                                                                                               // 类型
    Register(value interface{})                                                                                     // 注册struct类型
    Call(name string, arg interface{}) (interface{}, error)                                                         // 调用
    Discard() error                                                                                                 // 废弃连接
    Close() error                                                                                                   // 关闭
}
type PluginRPCClient struct {                                                                                   // RPC客户端
    ConnPool *vconnpool.ConnPool                                                                                    // 连接池
    Addr     string                                                                                                 // 地址
    Path     string                                                                                                 // 路径
}
    func (T *PluginRPCClient) Connection() (PluginRPC, error)                                                       // 快速连接RPC
type PluginType int                                                                                             // 插件类型
const (
    PluginTypeRPC PluginType = iota                                                                                 // RPC
    PluginTypeHTTP                                                                                                  // HTTP
)
type Responser interface {                                                                                      // 响应
    Write([]byte) (int, error)                                                                                      // 写入字节
    WriteString(string) (int, error)                                                                                // 写入字符串
    ReadFrom(io.Reader) (int64, error)                                                                              // 读取并写入
    Redirect(string, int)                                                                                           // 转向
    WriteHeader(int)                                                                                                // 状态码
    Error(string, int)                                                                                              // 错误
    Flush()                                                                                                         // 刷新缓冲
    Push(target string, opts *http.PushOptions) error                                                               // HTTP/2推送
    Hijack() (net.Conn, *bufio.ReadWriter, error)                                                                   // 劫持，能双向互相发送信息
}
type Route struct {                                                                                             // 路由
    HandlerError func(w http.ResponseWriter, r *http.Request)                                                       // 错误访问处理
}
    func (T *Route) HandleFunc(url string, handler func(w http.ResponseWriter, r *http.Request))                    // 绑定处理函数
    func (T *Route) ServeHTTP(w http.ResponseWriter, r *http.Request)                                               // 服务HTTP
type ServerHandlerDynamic struct {                                                                              // 动态
    //必须的
    RootPath string                                                                                                 // 根目录
    PagePath string                                                                                                 // 主模板文件路径

    //可选的
    BuffSize int64                                                                                                  // 缓冲块大小
    Site     *Site                                                                                                  // 网站配置
    Context  context.Context                                                                                        // 上下文
    Plus     map[string]DynamicTemplate                                                                             // 支持更动态文件类型
}
    func (T *ServerHandlerDynamic) Execute(bufw *bytes.Buffer, dock interface{}) (err error)                        // 执行模板
    func (T *ServerHandlerDynamic) Parse(bufr *bytes.Buffer) (err error)                                            // 解析模板
    func (T *ServerHandlerDynamic) ParseFile(path string) error                                                     // 解析模板文件
    func (T *ServerHandlerDynamic) ParseText(content, name string) error                                            // 解析模板文本
    func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request)                             // 服务HTTP
type ServerHandlerStatic struct {                                                                               // 静态
    RootPath, PagePath string                                                                                       // 根目录, 页路径
    PageExpired        int64                                                                                        // 页面过期时间（秒为单位）
    BuffSize           int64                                                                                        // 缓冲块大小
}
    func (T *ServerHandlerStatic) ServeHTTP(rw http.ResponseWriter, req *http.Request)                              // 服务HTTP
type Session struct {                                                                                           // 会话
    vmap.Map                                                                                                        // 数据，用户存储的数据
}
    func (T *Session) Defer(call interface{}, args ...interface{}) error                                            // 退出调用
    func (T *Session) Free()                                                                                        // 释放调用
    func (T *Session) Token() string                                                                                // 编号
type Sessioner interface {                                                                                      // 会话接口
    Token() string                                                                                                  // 编号
    Set(key, val interface{})                                                                                       // 设置
    Has(key interface{}) bool                                                                                       // 判断
    Get(key interface{}) interface{}                                                                                // 读取
    GetHas(key interface{}) (val interface{}, ok bool)                                                              // 读取判断
    Del(key interface{})                                                                                            // 删除
    SetExpired(key interface{}, d time.Duration)                                                                    // 过期
    SetExpiredCall(key interface{}, d time.Duration, f func(interface{}))                                           // 过期调用
    Reset()                                                                                                         // 重置
    Defer(call interface{}, args ...interface{}) error                                                              // 退出调用
    Free()                                                                                                          // 释放调用
}
type Sessions struct {                                                                                          // 会话集
    Expired      time.Duration                                                                                      // 保存session时间长
    Name         string                                                                                             // 标识名称
    Size         int                                                                                                // 会话ID长度
    Salt         string                                                                                             // 加盐，由于计算机随机数是伪随机数。（可默认为空）
    ActivationID bool                                                                                               // 为true，保持会话ID
}
    func (T *Sessions) DelSession(id string)                                                                        // 使用id删除的会话
    func (T *Sessions) GetSession(id string) (Sessioner, error)                                                     // 使用id读取会话
    func (T *Sessions) Len() int                                                                                    // 数量
    func (T *Sessions) NewSession() Sessioner                                                                       // 新建会话
    func (T *Sessions) ProcessDeadAll() []interface{}                                                               // 过期处理
    func (T *Sessions) Session(rw http.ResponseWriter, req *http.Request) Sessioner                                 // 会话
    func (T *Sessions) SessionId(req *http.Request) (id string, err error)                                          // 从请求中读取会话标识
    func (T *Sessions) SetSession(id string, s Sessioner) Sessioner                                                 // 使用id写入新的会话
type Site struct {                                                                                              // 网站
    Sessions *Sessions                                                                                              // 会话集
    Global   Globaler                                                                                               // Global
    RootDir  func(path string) string                                                                               // 网站的根目录
    Extend   interface{}                                                                                            // 接口类型，可以自己存在任何类型
}
    func (T *Site) PoolName() string                                                                                // 池名
type SiteMan struct {}                                                                                          // 网站管理
    func (T *SiteMan) Add(host string, site *Site)                                                                  // 设置一个站点
    func (T *SiteMan) Get(host string) (*Site, bool)                                                                // 读取一个站点
    func (T *SiteMan) Range(f func(host string, site *Site) bool)                                                   // 迭举站点
type SitePool struct {}                                                                                         // 网站池
    func NewSitePool() *SitePool                                                                                    // 新建
    func (T *SitePool) Close() error                                                                                // 关闭池
    func (T *SitePool) DelSite(name string)                                                                         // 删除站点
    func (T *SitePool) NewSite(name string) *Site                                                                   // 创建一个站点，如果存在返回已经存在的
    func (T *SitePool) RangeSite(f func(name string, site *Site) bool)                                              // 迭举站点
    func (T *SitePool) SetRecoverSession(d time.Duration)                                                           // 设置回收无效时间隔（默认1秒）
    func (T *SitePool) Start() error                                                                                // 启动池
type TemplateDot struct {                                                                                       // 模板点
    R        *http.Request                                                                                          // 请求
    W        http.ResponseWriter                                                                                    // 响应
    BuffSize int64                                                                                                  // 缓冲块大小
    Site     *Site                                                                                                  // 网站配置
    Writed   bool                                                                                                   // 表示已经调用写入到客户端。这个是只读的
}
    func (T *TemplateDot) Context() context.Context                                                                 // 上下文
    func (T *TemplateDot) Cookie() Cookier                                                                          // Cookie
    func (T *TemplateDot) Defer(call interface{}, args ...interface{}) error                                        // 退同调用
    func (T *TemplateDot) Free()                                                                                    // 释放Defer
    func (T *TemplateDot) Global() Globaler                                                                         // 全站缓存
    func (T *TemplateDot) Header() http.Header                                                                      // 标头
    func (T *TemplateDot) Request() *http.Request                                                                   // 请求的信息
    func (T *TemplateDot) RequestLimitSize(l int64) *http.Request                                                   // 请求限制大小
    func (T *TemplateDot) Response() Responser                                                                      // 数据写入响应
    func (T *TemplateDot) ResponseWriter() http.ResponseWriter                                                      // 数据写入响应
    func (T *TemplateDot) RootDir(upath string) string                                                              // 网站的根目录
    func (T *TemplateDot) Session() Sessioner                                                                       // 用户的会话
    func (T *TemplateDot) Swap() *vmap.Map                                                                          // 信息交换
    func (T *TemplateDot) WithContext(ctx context.Context)                                                          // 替换上下文
type TemplateDoter interface {                                                                                  // 模板点
    RootDir(path string) string                                                                                     // 网站的根目录
    Request() *http.Request                                                                                         // 用户的请求信息
    RequestLimitSize(l int64) *http.Request                                                                         // 请求限制大小
    Header() http.Header                                                                                            // 标头
    Response() Responser                                                                                            // 数据写入响应
    ResponseWriter() http.ResponseWriter                                                                            // 数据写入响应
    Session() Sessioner                                                                                             // 用户的会话缓存
    Global() Globaler                                                                                               // 全站缓存
    Cookie() Cookier                                                                                                // 用户的Cookie
    Swap() *vmap.Map                                                                                                // 信息交换
    Defer(call interface{}, args ...interface{}) error                                                              // 退回调用
    DotContexter
}
```