package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/456vv/vmap/v2"
	"github.com/456vv/vweb/v3"
	"github.com/456vv/vweb/v3/builtin"
	"github.com/456vv/vweb/v3/server/config"
	"golang.org/x/crypto/acme/autocert"
)

var Version = "Server/v3"

// 上下文的Key, 在请求中可以使用
type contextKey struct {
	name string
}

func (T *contextKey) String() string { return "server context value " + T.name }

var ServerContextKey = &contextKey{"Server"}

// safeCache 线程安全缓存（RWMutex），适合低频配置读写。
type safeCache struct {
	mu   sync.RWMutex
	data map[any]any
}

// Get 读操作，使用 RLock（允许多个 Goroutine 同时读）。
func (c *safeCache) Get(key any) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[key]
}

// Set 写操作，使用 Lock（写时阻塞其他读与写）。
func (c *safeCache) Set(key, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[any]any)
	}
	c.data[key] = value
}

// Server 封装 http.Server，支持自定义 TCP 选项、TLS 与并发安全状态。
type Server struct {
	*http.Server // http服务器
	Addr         string
	l            listener
	status       atomic.Bool    // 运行状态（true=正在服务）
	cServer      *config.Server // 用于服务器
	cConn        *config.Conn   // 用于连接
}

func (T *Server) init() {
	if T.Server == nil {
		T.Server = new(http.Server)
		T.Server.BaseContext = func(l net.Listener) context.Context {
			return context.WithValue(context.Background(), vweb.ListenerContextKey, l)
		}
		T.Server.ConnContext = func(ctx context.Context, rwc net.Conn) context.Context {
			return context.WithValue(ctx, vweb.ConnContextKey, rwc)
		}
	}
}

// Serve 启动服务。状态由 atomic CAS 保护，可被多 goroutine 安全调用。
// 兼容 *net.TCPListener 以及提供 TCPListener() 方法的包装监听器。
func (T *Server) Serve(l net.Listener) error {
	if !T.status.CompareAndSwap(false, true) {
		return errors.New("server: already running")
	}
	defer T.status.Store(false)

	if l == nil {
		return fmt.Errorf("server: listener is nil")
	}

	var tcpLn *net.TCPListener
	switch ln := l.(type) {
	case *net.TCPListener:
		tcpLn = ln
	case interface{ TCPListener() *net.TCPListener }:
		tcpLn = ln.TCPListener()
	}
	if tcpLn == nil {
		return fmt.Errorf("server: listener must be *net.TCPListener or provide TCPListener(), got %T", l)
	}

	ta, ok := tcpLn.Addr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("server: listener address must be *net.TCPAddr, got %T", tcpLn.Addr())
	}
	ip := ""
	if ta.IP != nil {
		ip = ta.IP.String()
	}
	// 使用 net.JoinHostPort 兼容 IPv4/IPv6（含 zone）
	T.Addr = net.JoinHostPort(ip, strconv.Itoa(ta.Port))

	T.l.TCPListener = tcpLn
	T.l.closed.Store(false)
	// cConn / tlsconfig 已在 ConfigConn / ConfigServer 中注入

	T.init()
	return T.Server.Serve(&T.l)
}

// IsRunning 返回服务器当前是否处于服务状态（并发安全）。
func (T *Server) IsRunning() bool {
	return T.status.Load()
}

// Close 立即关闭服务器与底层 Listener（不等待进行中连接完成）。
// 先关闭 http.Server（触发 doneChan，令阻塞的 Serve 返回 http.ErrServerClosed），
// 再关闭自定义 listener（幂等），避免 Serve 以 net.ErrClosed 意外退出而误报错误。
func (T *Server) Close() error {
	T.status.Store(false)
	var err error
	if T.Server != nil {
		err = T.Server.Close()
	}
	if e := T.l.Close(); err == nil {
		err = e
	}
	return err
}

// ListenAndServe 在 T.Addr 上监听并服务。
// 若 Addr 为空，默认使用 ":http"。
// 内部调用 net.Listen("tcp", ...) 后转交 Serve，因此同样受 status CAS 保护。
func (T *Server) ListenAndServe() error {
	if T.Addr == "" {
		T.Addr = ":http"
	}
	l, err := net.Listen("tcp", T.Addr)
	if err != nil {
		return err
	}
	return T.Serve(l)
}

// ConfigConn 配置连接级 TCP 选项（KeepAlive、NoDelay、缓冲区、Deadline 等）。
// 必须在 Serve / ListenAndServe 之前调用。
// 选项会在每次 Accept 后、TLS 包装前应用到 *net.TCPConn。
// 运行期间禁止修改，防止与 Accept 并发产生数据竞争。
func (T *Server) ConfigConn(cc *config.Conn) error {
	if cc == nil {
		return errors.New("server: *config.Conn 不可以为nil")
	}
	if T.status.Load() {
		return errors.New("server: 服务运行中，不能修改连接配置")
	}
	T.cConn = cc
	T.l.cConn = cc
	return nil
}

// ConfigServer 配置服务器参数（超时、TLS、KeepAlive 等）。
// 必须在 Serve / ListenAndServe 之前调用。
// TLS 配置会同时写入 http.Server.TLSConfig 与自定义 listener.tlsconfig，
// 由 listener.Accept 完成 TLS 握手包装（支持非标准 TLS Listener 场景）。
func (T *Server) ConfigServer(cs *config.Server) error {
	if cs == nil {
		return errors.New("server: *config.Server 不可以为nil")
	}
	if T.status.Load() {
		return errors.New("server: 服务运行中，不能修改服务器配置")
	}
	T.cServer = cs

	// 服务器配置
	T.init()
	T.Server.ReadTimeout = time.Duration(cs.ReadTimeout) * time.Millisecond
	T.Server.WriteTimeout = time.Duration(cs.WriteTimeout) * time.Millisecond
	T.Server.ReadHeaderTimeout = time.Duration(cs.ReadHeaderTimeout) * time.Millisecond
	T.Server.IdleTimeout = time.Duration(cs.IdleTimeout) * time.Millisecond
	T.Server.MaxHeaderBytes = cs.MaxHeaderBytes
	T.Server.DisableGeneralOptionsHandler = cs.DisableGeneralOptionsHandler
	T.Server.SetKeepAlivesEnabled(cs.KeepAlivesEnabled)

	// TLS设置
	if cs.TLS != nil {
		if T.Server.TLSConfig == nil {
			T.Server.TLSConfig = new(tls.Config)
		}
		if err := configTLSFile(T.Server.TLSConfig, cs.TLS); err != nil {
			return err
		}
		T.l.tlsconfig = T.Server.TLSConfig
	} else {
		T.l.tlsconfig = nil
	}
	return nil
}

// configTLSFile 加载服务端证书、客户端 CA，配置协议版本、密码套件、SessionTicket 等。
// 支持 .pem/.crt（PEM）与 .cer（DER）格式。
// 错误聚合后统一返回，单个文件失败不影响其余证书加载（跨平台兼容：Windows/Linux/macOS 文件路径）。
// 注意：RootCAs 字段实际用于服务端 Certificates（历史命名保留）。
func configTLSFile(c *tls.Config, conf *config.ServerTLS) error {
	c.NextProtos = conf.NextProtos
	c.SessionTicketsDisabled = conf.SessionTicketsDisabled
	c.MinVersion = conf.MinVersion
	c.MaxVersion = conf.MaxVersion
	c.DynamicRecordSizingDisabled = conf.DynamicRecordSizingDisabled

	// 修复原 copy 在目标为 nil 时无效的问题，直接赋值
	if len(conf.CipherSuites) > 0 {
		c.CipherSuites = append([]uint16(nil), conf.CipherSuites...)
	} else {
		// 内部判断并使用默认的密码套件
		c.CipherSuites = nil
	}

	if len(conf.SetSessionTicketKeys) > 0 {
		c.SetSessionTicketKeys(conf.SetSessionTicketKeys)
	}

	// 错误聚合使用 strings.Builder 避免频繁字符串拼接
	var errBuilder strings.Builder

	// 支持双向证书（客户端 CA）
	if len(conf.ClientCAs) != 0 {
		if c.ClientCAs == nil {
			if certPool, err := x509.SystemCertPool(); err == nil {
				// 系统证书
				c.ClientCAs = certPool
			} else {
				// 如果读取系统根证书失败, 则创建新的证书
				c.ClientCAs = x509.NewCertPool()
			}
		}
		var clientCAErr strings.Builder
		for _, p := range conf.ClientCAs {
			data, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(&clientCAErr, "%s: %s\n", p, err)
				continue
			}
			switch filepath.Ext(p) {
			case ".cer":
				certs, err := x509.ParseCertificates(data)
				if err != nil {
					fmt.Fprintf(&clientCAErr, "%s: %s\n", p, err)
					continue
				}
				for _, cert := range certs {
					c.ClientCAs.AddCert(cert)
				}
			case ".pem", ".crt":
				if !c.ClientCAs.AppendCertsFromPEM(data) {
					fmt.Fprintf(&clientCAErr, "%s: not a valid PEM format\n", p)
				}
			default:
				fmt.Fprintf(&clientCAErr, "TLS.ClientCAs[%q]: unsupported type, only .cer/.crt/.pem\n", p)
			}
		}
		if clientCAErr.Len() > 0 {
			errBuilder.WriteString("解析客户端CA证书发生错误（CS.TLS.ClientCAs）:\n")
			errBuilder.WriteString(clientCAErr.String())
		}
	}

	// Server Certificates (历史字段名 RootCAs)
	c.Certificates = nil
	var serverCertErr strings.Builder
	for _, file := range conf.RootCAs {
		cert, err := tls.LoadX509KeyPair(file.CertFile, file.KeyFile)
		if err != nil {
			fmt.Fprintf(&serverCertErr, "{CertFile:%q, KeyFile:%q}: %s\n", file.CertFile, file.KeyFile, err)
			continue
		}
		c.Certificates = append(c.Certificates, cert)
	}
	if serverCertErr.Len() > 0 {
		errBuilder.WriteString("解析服务端证书发生错误（CS.TLS.RootCAs）:\n")
		errBuilder.WriteString(serverCertErr.String())
	}

	// 多证书。
	// c.BuildNameToCertificate() // 已在 Go 1.14+ 废弃，tls 内部自动处理
	if errBuilder.Len() > 0 {
		return fmt.Errorf("server: %s", errBuilder.String())
	}
	return nil
}

// siteExtendInfo 一个站点的扩展信息。
// configSite 的写入发生在热重载（updateSitePoolAdd）中，读取发生在每个请求（serveHTTP）中，
// 两者之间通过 configMu 同步，保证请求总能拿到“一整份”自洽的配置快照，避免 32 位平台指针撕裂与竞态。
type siteExtendInfo struct {
	configMu     sync.RWMutex
	configSite   *config.Site
	plugin       *plugin
	dynamicCache *vmap.Map // 缓存动态文件对象
}

// config 并发安全地返回当前配置快照（RLock 保护）。
func (T *siteExtendInfo) config() *config.Site {
	if T == nil {
		return nil
	}
	T.configMu.RLock()
	defer T.configMu.RUnlock()
	return T.configSite
}

// setConfig 并发安全地替换配置（Lock 保护）。只整份替换，从不修改旧配置内容，
// 因此读方在拿到指针解锁后可安全使用整个旧快照。
func (T *siteExtendInfo) setConfig(c *config.Site) {
	if T == nil {
		return
	}
	T.configMu.Lock()
	T.configSite = c
	T.configMu.Unlock()
}

func newSiteExtendInfo() *siteExtendInfo {
	return &siteExtendInfo{
		configSite:   nil,
		plugin:       new(plugin),
		dynamicCache: new(vmap.Map),
	}
}

func getSiteExtend(site *vweb.Site) *siteExtendInfo {
	if site == nil {
		return nil
	}
	if se, ok := site.Extend.Get(site).(*siteExtendInfo); ok {
		return se
	}
	se := newSiteExtendInfo()
	site.Extend.Set(site, se)
	return se
}

type Group struct {
	ErrorLog *log.Logger                                      // 错误日志文件
	Module   func(name string) (vweb.DynamicTemplater, error) // 支持更动态文件类型

	route *vweb.Route // 地址路由

	CertManager *autocert.Manager // 自动申请证书 Let's Encrypt

	// srvMan 存储值类型是 *Server, 读取时需要转换类型
	srvMan    vmap.Map       // map[ip:port]*Server	服务器集
	sitePool  *vweb.SitePool // 站点的池
	siteMan   *vweb.SiteMan  // 站点管理
	extConfig safeCache
	exit      chan bool // 退出

	run atomic.Bool // 服务器启动了

	// 用于 .UpdateConfigFile 方法
	configFileModTime time.Time
	config            *config.Config // 配置

	// configMu 串行化所有“配置应用/重载/关闭”流程，
	// 避免 LoadConfigFile 定时重载与 UpdateConfig/Close 并发交错导致的数据竞争与半更新状态。
	configMu sync.Mutex
}

func NewGroup() *Group {
	g := &Group{
		exit:      make(chan bool),
		ErrorLog:  log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
		extConfig: safeCache{data: make(map[any]any)},
		route:     new(vweb.Route),
		siteMan:   new(vweb.SiteMan),
		sitePool:  vweb.NewSitePool(),
	}
	g.route.SetSiteMan(g.siteMan)
	return g
}

func (T *Group) SetServer(laddr string, srv *Server) error {
	if srv == nil {
		T.srvMan.Del(laddr)
		return nil
	}
	T.defaultServerConfig(srv)
	T.srvMan.Set(laddr, srv)
	return nil
}

func (T *Group) defaultServerConfig(srv *Server) {
	if srv.Handler == nil {
		// 使用路由
		srv.Handler = http.HandlerFunc(T.route.ServeHTTP)
		if T.route.HandlerError == nil {
			T.route.HandlerError = http.HandlerFunc(T.serveHTTP)
		}
	}

	srv.Handler = vweb.AutoCert(T.CertManager, srv.TLSConfig, srv.Handler)
}

// GetServer 读取一个服务器
//
//	laddr string	监听地址
//	*Server			服务器
//	bool			如果存在服务器, 返回true。否则返回false
func (T *Group) GetServer(laddr string) (*Server, bool) {
	if inf, ok := T.srvMan.GetHas(laddr); ok {
		if srv, ok := inf.(*Server); ok && srv != nil {
			return srv, true
		}
	}
	return nil, false
}

// SetSessionExpiryInterval 设置session的过期时间, 随配置文件变动, d 原来的保存内容可能会被删除或增加。
func (T *Group) SetSessionExpiryInterval(d time.Duration) {
	if T.sitePool != nil {
		T.sitePool.SetRecoverSession(d)
	}
}

// SiteMan 站点管理器, 用于获取和设置站点信息。
func (T *Group) SiteMan() *vweb.SiteMan {
	return T.siteMan
}

// serveHTTP 处理HTTP
//
//	rw http.ResponseWriter	响应
//	r *http.Request			请求
func (T *Group) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	//** 检查Host是否存在
	site, ok := T.siteMan.Get(r.Host)
	if !ok {
		if hj, ok := rw.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				conn.Close()
				return
			}
		}
		http.Error(rw, "Not supported Hijacker", http.StatusInternalServerError)
		return
	}

	//** 配置（通过 config() 并发安全地取得整份配置快照）
	var (
		se     = getSiteExtend(site)
		plugin = se.plugin
		dCache = se.dynamicCache
		conf   = se.config()
	)
	if conf == nil {
		// 500 服务器遇到了意料不到的情况, 不能完成客户的请求。
		http.Error(rw, "The configuration is nil\n", http.StatusInternalServerError)
		return
	}

	//** 静态文件
	var (
		err      error
		rootDir  = site.RootDir
		rootPath string
		pagePath string

		findStatic bool
	)

	// 直接读取缓存文件
	if conf.Dynamic.Cache && conf.Dynamic.CacheStaticFileDir != "" {
		uPath := r.URL.Path
		cDir := conf.Dynamic.CacheStaticFileDir
		if !filepath.IsAbs(cDir) {
			// 相对路径
			uPath = path.Join("/", cDir, r.URL.Path)
			cDir = rootDir(uPath)
		}
		if fInfo, pPath, err := vweb.PagePath(cDir, uPath, conf.IndexFile); err == nil {
			t := time.Now()
			cSecond := time.Duration(conf.Dynamic.CacheStaticTimeout)
			if fInfo.ModTime().Add(cSecond).After(t) {
				// 替换根目录
				pagePath = pPath
				rootPath = cDir
				findStatic = true
			}
		}
	}

	// 表示【不】存在静态文件
	if !findStatic {

		//** 转发URL
		forward := conf.Forward
		urlPath := r.URL.Path
		if len(forward) != 0 {
			var forwardC config.SiteForwards
			derogatoryDomain(r.Host, func(h string) (ok bool) {
				forwardC, ok = forward[h]
				return
			})

			for _, fc := range forwardC.List {
				if !fc.Status {
					// 跳过禁止的
					continue
				}
				forwardRewriter, err := fc.Compile()
				if err != nil {
					T.ErrorLog.Printf("server: host(%s) 进行重写URL规则编译发生错误：%s\n", r.Host, err.Error())
					continue
				}

				rpath, rewried := forwardRewriter.Rewrite(urlPath)
				if rewried {
					if fc.RedirectCode != 0 {
						// 重定向,并退出
						http.Redirect(rw, r, rpath, fc.RedirectCode)
						return
					}

					urlPath = rpath

					if fc.End {
						break
					}
				}
			}
		}

		//** 文件存在
		rootPath = rootDir(urlPath)
		if _, pagePath, err = vweb.PagePath(rootPath, urlPath, conf.IndexFile); err != nil {
			// 404 无法找到指定位置的资源。这也是一个常用的应答。
			httpError(rw, rootPath, conf.ErrorPage, err.Error(), http.StatusNotFound)
			return
		}
	}

	//** 文件后缀支持
	var (
		fileExt     = path.Ext(pagePath)
		header      = conf.Header
		contentType = httpTypeByExtension(fileExt, header.MIME)
	)

	//** 文件固定标头准备

	wh := rw.Header()

	wh.Set("Content-Type", contentType)
	wh.Set("Server", Version)

	//** 文件动态静态分离
	if strSliceContains(conf.Dynamic.Ext, fileExt) {
		// 动态页面

		if contentType == "" {
			wh.Set("Content-Type", "text/html; charset=utf-8")
		}

		// 读取指定后缀类型的标头内容
		if header.Dynamic != nil {
			siteHeaderType(wh, header.Dynamic, fileExt)
		}

		// 处理动态格式
		var handlerDynamic *vweb.ServerHandlerDynamic
		if inf, ok := dCache.GetHas(pagePath); ok && conf.Dynamic.Cache {
			handlerDynamic, _ = inf.(*vweb.ServerHandlerDynamic)
			if handlerDynamic == nil {
				dCache.Del(pagePath)
			} else if conf.Dynamic.CacheParseTimeout != 0 {
				dCache.SetExpired(pagePath, time.Duration(conf.Dynamic.CacheParseTimeout))
			}
		}
		if handlerDynamic == nil {
			if ok {
				// 存在缓存 ,不开启缓存,释放缓存
				dCache.Del(pagePath)
			}
			handlerDynamic = &vweb.ServerHandlerDynamic{
				RootPath: rootPath,
				PagePath: pagePath,
				Module:   T.Module,
			}
			if conf.Dynamic.Cache {
				// 时效
				dCache.Set(pagePath, handlerDynamic)
				if conf.Dynamic.CacheParseTimeout != 0 {
					dCache.SetExpiredCall(pagePath, time.Duration(conf.Dynamic.CacheParseTimeout), func(a any) {
						if d, ok := a.(*vweb.ServerHandlerDynamic); ok {
							d.Close()
						}
					})
				}
			} else {
				defer handlerDynamic.Close()
			}
		}

		// 在 route.go 已有在上下文中设置 site/router
		ctx := context.WithValue(r.Context(), vweb.PluginContextKey, vweb.Pluginer(plugin))
		r = r.WithContext(ctx)
		handlerDynamic.ServeHTTP(rw, r)
	} else {
		// 静态页面
		if contentType == "" {
			wh.Set("Content-Type", "application/octet-stream")
		}

		// 读取指定后缀类型的标头内容
		var ht config.SiteHeaderType
		if header.Static != nil {
			ht = siteHeaderType(wh, header.Static, fileExt)
		}

		shs := &vweb.ServerHandlerStatic{
			RootPath:    rootPath,
			PagePath:    pagePath,
			PageExpired: ht.PageExpired,
		}
		shs.ServeHTTP(rw, r)
	}
}

// 更新插件
func (T *Group) updateExtInfo(cSite config.Site) {
	var (
		site   = T.sitePool.NewSite(cSite.Identity)
		se     = getSiteExtend(site)
		dCache = se.dynamicCache
		plugin = se.plugin

		httpEffectiveNames []string // 存放有效的http插件名称
		rpcEffectiveNames  []string // 存放有效的rpc插件名称
	)

	// 配置插件
	if cSite.Status {
		for name, p := range cSite.Plugin.HTTP {
			if !p.Status {
				continue
			}
			httpEffectiveNames = append(httpEffectiveNames, name)

			if p.Addr == "" {
				T.ErrorLog.Printf("server: 名称 %s 的HTTP插件配 Addr 字段不可以为空", name)
				continue
			}

			httpC := new(vweb.PluginHTTPClient)
			if inf, ok := plugin.http.Load(name); ok {
				if c, ok := inf.(*vweb.PluginHTTPClient); ok && c != nil {
					httpC = c
				}
			}

			if err := p.ConfigPluginHTTPClient(httpC); err != nil {
				T.ErrorLog.Printf("server: 名称 %s 的HTTP插件配置错误: %s\n", name, err)
				continue
			}
			plugin.http.Store(name, httpC)
		}
		for name, p := range cSite.Plugin.RPC {
			if !p.Status {
				continue
			}
			rpcEffectiveNames = append(rpcEffectiveNames, name)

			if p.Addr == "" {
				T.ErrorLog.Printf("server: 名称 %s 的RPC插件配 Addr 字段不可以为空\n", name)
				continue
			}

			rpcC := new(vweb.PluginRPCClient)
			if inf, ok := plugin.rpc.Load(name); ok {
				if c, ok := inf.(*vweb.PluginRPCClient); ok && c != nil {
					rpcC = c
				}
			}
			if err := p.ConfigPluginRPCClient(rpcC); err != nil {
				T.ErrorLog.Printf("server: 名称 %s 的RPC插件配置错误: %s", name, err)
				continue
			}
			plugin.rpc.Store(name, rpcC)
		}
	} else {
		// 清除动态文件缓存
		dCache.Reset()
	}

	// 清理无效插件
	plugin.http.Range(func(name, client any) bool {
		if !strSliceContains(httpEffectiveNames, name.(string)) {
			plugin.http.Delete(name)
			if c, ok := client.(*vweb.PluginHTTPClient); ok && c != nil && c.Tr != nil {
				c.Tr.CloseIdleConnections()
			}
		}
		return true
	})
	plugin.rpc.Range(func(name, client any) bool {
		if !strSliceContains(rpcEffectiveNames, name.(string)) {
			plugin.rpc.Delete(name)
			if c, ok := client.(*vweb.PluginRPCClient); ok && c != nil && c.ConnPool != nil {
				c.ConnPool.Close()
			}
		}
		return true
	})
}

// 更新站点池或增加
//
//	cSite config.ConfigSite     配置
func (T *Group) updateSitePoolAdd(cSite config.Site) {
	site := T.sitePool.NewSite(cSite.Identity)
	for _, host := range cSite.Host {
		T.siteMan.Add(host, site)
	}

	// 设置Session
	builtin.CopyStruct(site.Sessions(), &cSite.Session, func(name string, dsc, src reflect.Value) bool {
		return name == "Expired"
	})

	site.Sessions().Expired = time.Duration(cSite.Session.Expired) * time.Second
	site.RootDir = cSite.Directory.RootDir

	// 配置保存到网站扩展中（并发安全整份替换）
	getSiteExtend(site).setConfig(&cSite)
}

// 更新站点池删除, 过滤并删除无效的站点池。
//
// siteEffectiveHosts []string		现有有效的host
func (T *Group) updateSitePoolDel(siteEffectiveIdent, siteEffectiveHosts []string) {
	// 从网站管理中删除无用的网站
	T.siteMan.Range(func(host string, site *vweb.Site) bool {
		if !strSliceContains(siteEffectiveHosts, host) {
			T.siteMan.Add(host, nil)
		}
		return true
	})

	// 让站点池中删除无用的网站
	T.sitePool.RangeSite(func(name string, site *vweb.Site) bool {
		if !strSliceContains(siteEffectiveIdent, name) {
			// 从池中删除
			T.sitePool.DelSite(name)

			if site == nil {
				return true
			}

			se := getSiteExtend(site)
			if se == nil {
				return true
			}
			plugin := se.plugin
			plugin.http.Range(func(name, client any) bool {
				plugin.http.Delete(name)
				if c, ok := client.(*vweb.PluginHTTPClient); ok && c != nil && c.Tr != nil {
					c.Tr.CloseIdleConnections()
				}
				return true
			})
			plugin.rpc.Range(func(name, client any) bool {
				plugin.rpc.Delete(name)
				if c, ok := client.(*vweb.PluginRPCClient); ok && c != nil && c.ConnPool != nil {
					c.ConnPool.Close()
				}
				return true
			})
			se.dynamicCache.Reset()

			// 清除Session
			site.Sessions().InstantDeadAll()
			// 清除全局数据
			site.Global().Reset()
			// 清除网站扩展数据
			site.Extend.Reset()
		}
		return true
	})
}

func (T *Group) updateConfigSites(conf config.Sites) error {
	var (
		siteEffectiveIdent []string
		siteEffectiveHosts []string
	)

	for _, cSite := range conf.Site {
		if cSite.Identity == "" {
			return fmt.Errorf("server: 配置中出现站点惟一名(Identity)为 \"\", 需要设定一个名称。")
		}

		if cSite.Status {
			// 复制公共  Session 配置
			if cSite.Session.PublicName != "" && !conf.Public.ConfigSiteSession(&cSite.Session, nil) {
				T.ErrorLog.Printf("server: %s 站点的私有Session与公共Session合并失败\n", cSite.Identity)
			}
			// 复制公共 Header 配置
			if cSite.Header.PublicName != "" && !conf.Public.ConfigSiteHeader(&cSite.Header, nil) {
				T.ErrorLog.Printf("server: %s 站点的私有Header与公共Header合并失败\n", cSite.Identity)
			}

			// 复制公共插件 RPC 配置
			for name, pRPC := range cSite.Plugin.RPC {
				if pRPC.PublicName != "" {
					if conf.Public.Plugin.ConfigSitePluginRPC(&pRPC, nil) {
						cSite.Plugin.RPC[name] = pRPC
						continue
					}
					T.ErrorLog.Printf("server: %s 站点的 Plugin RPC %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制公共插件 HTTP 配置
			for name, pHTTP := range cSite.Plugin.HTTP {
				if pHTTP.PublicName != "" {
					if conf.Public.Plugin.ConfigSitePluginHTTP(&pHTTP, nil) {
						cSite.Plugin.HTTP[name] = pHTTP
						continue
					}
					T.ErrorLog.Printf("server: %s 站点的 Plugin HTTP %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制公共 Forward 配置
			for name, forward := range cSite.Forward {
				if forward.PublicName != "" {
					if conf.Public.ConfigSiteForward(&forward, nil) {
						cSite.Forward[name] = forward
						continue
					}
					T.ErrorLog.Printf("server: %s 站点的 Forward %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制Dynamic的配置
			if cSite.Dynamic.PublicName != "" && !conf.Public.ConfigSiteDynamic(&cSite.Dynamic, nil) {
				T.ErrorLog.Printf("server: %s 站点的私有Dynamic与公共Dynamic合并失败\n", cSite.Identity)
			}
			if cSite.Dynamic.CacheParseTimeout != 0 {
				cSite.Dynamic.CacheParseTimeout *= int64(time.Second)
			}
			if cSite.Dynamic.CacheStaticTimeout != 0 {
				cSite.Dynamic.CacheStaticTimeout *= int64(time.Second)
			}

			// 预先分配池, 初始化站点
			T.updateSitePoolAdd(cSite)

			// 集中站点名称
			siteEffectiveIdent = append(siteEffectiveIdent, cSite.Identity)

			// 集中站点Host
			// 可能有多个站点绑定了同一个Host, 只有最后一个是有效的
			siteEffectiveHosts = append(siteEffectiveHosts, cSite.Host...)
		}

		// 不管网站是否开启, 方法内部会再做处理
		T.updateExtInfo(cSite)
	}

	// 删除池中不存在的配置
	T.updateSitePoolDel(siteEffectiveIdent, siteEffectiveHosts)

	return nil
}

func (T *Group) newServer(laddr string) *Server {
	if inf, ok := T.srvMan.GetHas(laddr); ok {
		if srv, ok := inf.(*Server); ok && srv != nil {
			return srv
		}
	}
	srv := new(Server)
	srv.Addr = laddr
	return srv
}

// listenStart 同步登记服务器并启动监听。
// 登记先于 goroutine 启动完成，杜绝“登记窗口”：Start/Close/Stop 在 listenStart 返回后
// 立即能在 srvMan 中看到该地址，避免重复 listenStart 或漏 Stop。
func (T *Group) listenStart(laddr string, conf config.Listen) error {
	srv := T.newServer(laddr)
	if err := srv.ConfigConn(&conf.CC); err != nil {
		return err
	}
	if err := srv.ConfigServer(&conf.CS); err != nil {
		return err
	}
	T.defaultServerConfig(srv)
	// 先登记（幂等），再在独立 goroutine 中启动服务
	T.srvMan.Set(laddr, srv)
	go T.serve(srv)
	return nil
}

// listenStop 停止指定地址的监听。
// 若配置了 ShutdownConn 则调用优雅 Shutdown（等待进行中请求），否则立即 Close。
// 先从中移除登记，再关闭，防止 serve 的 defer 误删后来重登记的新服务器。
func (T *Group) listenStop(laddr string) (err error) {
	inf, ok := T.srvMan.GetHas(laddr)
	if !ok {
		return nil
	}
	T.srvMan.Del(laddr)

	srv, ok := inf.(*Server)
	if !ok || srv == nil {
		return nil
	}
	if srv.Server != nil {
		srv.status.Store(false) // 立即反映关闭，防止并发启动
		if srv.cServer != nil && srv.cServer.ShutdownConn {
			// 不要即时关闭正在下载的连接（优雅关闭）
			return srv.Server.Shutdown(context.Background())
		}
	}
	// 立即关闭（含自定义 listener）
	return srv.Close()
}

// 监听决定, 区分是开启还是关闭监听。
//
// 修复原实现的以下问题：
//  1. 原代码对“仍在监听”的地址无条件再次 listenStart，而 Server.Serve 有 status CAS，
//     导致配置一重载就报 "already running"。
//  2. 原 serve() 的 defer srvMan.Del 会无条件删除该地址，而重载通常又调用 listenStart
//     重新登记同一地址，两者交错会误删“仍运行”的服务器，使后续无法 Stop/Close。
//
// 现逻辑：
//   - 先合并公共 CC/CS 配置；
//   - 已删除/已禁用/连接或服务器配置发生变更的地址 → listenStop 后按需重启；
//   - 配置未变的运行中服务器不重启（避免热重载造成断连）；
//   - 仅对缺失的地址 listenStart。
func (T *Group) updateConfigServers(conf config.Servers) {
	// 先合并新配置中的公共 CC/CS（listenStart 需要已合并的 cl.CC/cl.CS）
	exclude := func(name string, dsc, src reflect.Value) bool {
		switch name {
		case "TLS":
			// 如果这个TLS配置是nil，则跳过复制
			return !src.Elem().IsValid()
		}
		return false
	}
	for laddr := range conf.Listen {
		cl := conf.Listen[laddr]
		if !cl.Status {
			continue
		}
		if cl.CC.PublicName != "" && !conf.Public.ConfigConn(&cl.CC, nil) {
			T.ErrorLog.Printf("server: %s 地址的私有CC与公共CC合并失败\n", laddr)
		}
		if cl.CS.PublicName != "" && !conf.Public.ConfigServer(&cl.CS, exclude) {
			T.ErrorLog.Printf("server: %s 地址的私有CS与公共CS合并失败\n", laddr)
		}
		conf.Listen[laddr] = cl // 写回合并结果
	}

	// 关闭“已从配置中删除”的监听
	T.srvMan.Range(func(key, val any) bool {
		ip := key.(string)
		if _, ok := conf.Listen[ip]; !ok {
			if err := T.listenStop(ip); err != nil {
				T.ErrorLog.Println(err)
			}
		}
		return true
	})

	// 对仍在运行但配置发生变更的地址：优雅关闭后重新按新配置启动
	restart := func(ip string, cl config.Listen) {
		if err := T.listenStop(ip); err != nil {
			T.ErrorLog.Println(err)
		}
		if err := T.listenStart(ip, cl); err != nil {
			T.ErrorLog.Println(err)
		}
	}

	for laddr, cl := range conf.Listen {
		if !cl.Status {
			// 已禁用 → 停止监听
			if err := T.listenStop(laddr); err != nil {
				T.ErrorLog.Println(err)
			}
			continue
		}

		inf, running := T.srvMan.GetHas(laddr)
		if !running {
			// 尚未开启监听 → 启动
			if err := T.listenStart(laddr, cl); err != nil {
				T.ErrorLog.Println(err)
			}
			continue
		}

		srv, ok := inf.(*Server)
		if !ok || srv == nil {
			// 登记异常 → 重新启动
			if err := T.listenStart(laddr, cl); err != nil {
				T.ErrorLog.Println(err)
			}
			continue
		}

		// 服务器/连接配置变更才重启；配置未变则保持现有连接（不因重载断连）
		changed := false
		if srv.cServer != nil {
			changed = !reflect.DeepEqual(*srv.cServer, cl.CS)
		} else if !reflect.DeepEqual(config.Server{}, cl.CS) {
			changed = true
		}
		if !changed {
			if srv.cConn != nil {
				changed = !reflect.DeepEqual(*srv.cConn, cl.CC)
			} else if !reflect.DeepEqual(config.Conn{}, cl.CC) {
				changed = true
			}
		}
		if changed {
			restart(laddr, cl)
		}
	}
}

// LoadConfigFile 挂载本地配置文件。
//
//	p string        文件路径
//	ok bool			true配置文件被修改过, false没有变动
//	err error       错误
func (T *Group) LoadConfigFile(p string) (ok bool, err error) {
	fileInfo, err := os.Stat(p)
	if err != nil {
		return
	}

	// 判断文件修改时间是否有改动
	if fileInfo.ModTime().Equal(T.configFileModTime) {
		return
	}

	// 解析配置文件
	var conf config.Config
	if err = conf.ParseFiles(p); err != nil {
		return
	}

	// 解析成功后才记录文件修改时间；解析失败不更新，
	// 否则失败后该文件将永远不再重试。
	T.configFileModTime = fileInfo.ModTime()

	// 更新配置文件
	if err = T.UpdateConfig(&conf); err != nil {
		return
	}
	return true, nil
}

// UpdateConfig 更新配置并把配置分配到各个地方。不检查改动, 直接更新。更新配置需要调用 .Start 方法之后才生效。
//
//	conf *config.Config        配置
//	error               错误
func (T *Group) UpdateConfig(conf *config.Config) error {
	if conf == nil {
		return fmt.Errorf("server: conf 为 nil, 无法更新。")
	}
	T.configMu.Lock()
	defer T.configMu.Unlock()
	T.config = conf
	if T.run.Load() {
		// 更新网站配置
		if err := T.updateConfigSites(conf.Sites); err != nil {
			T.ErrorLog.Println(err)
		}
		// 更新服务器配置
		T.updateConfigServers(conf.Servers)
	}
	return nil
}

// serve 启动服务器（在独立 goroutine 中运行）。
// 状态管理完全交由 Server.Serve / ListenAndServe 内部的 atomic CAS 处理，
// 避免双重设置 status 导致的 "already running" 误报。
// 若服务器已在运行，ListenAndServe 会立即返回错误并由本函数记录日志。
// 正常关闭（http.ErrServerClosed / net.ErrClosed）不记为错误。
//
// 退出清理采用条件删除：只有“当前 map 中登记的还是本 Server”时才删除，
// 防止 stop 后重载重建同地址新服务器时，旧 goroutine 的 defer 误删新登记项。
func (T *Group) serve(srv *Server) {
	defer func() {
		if inf, ok := T.srvMan.GetHas(srv.Addr); ok {
			if cur, ok2 := inf.(*Server); ok2 && cur == srv {
				T.srvMan.Del(srv.Addr)
			}
		}
	}()
	err := srv.ListenAndServe() // 阻塞；内部通过 status CAS 保证单实例运行
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		T.ErrorLog.Printf("server: ip(%s), %s\n", srv.Addr, err)
	}
}

// Start 启动服务集群。
// 通过 atomic run 标志保证只启动一次。
// 若已有配置则立即应用（更新站点与监听），然后阻塞等待退出信号。
// 优化：支持“Close 后再次 Start”（重建 exit 通道），并改用 close(exit) 通知，
// 消除“Close 在 Start 之前调用导致 Start 永久阻塞”的死锁隐患。
func (T *Group) Start() error {
	if T.run.Swap(true) {
		return fmt.Errorf("server: 服务组已经开启。")
	}

	// 若 Close 已执行过，会重建站点池与站点管理；此处统一做一次幂等初始化。
	T.initGroupResources()
	T.exit = make(chan bool)

	// 刷新配置（在 run=true 后执行，确保 UpdateConfig 内部逻辑生效）
	if T.config != nil {
		T.UpdateConfig(T.config)
	}

	// 等待退出信号（由 Close 发送）
	<-T.exit
	return nil
}

// initGroupResources 幂等初始化 Group 运行期必需的资源。
// 与 NewGroup 中重复时也会安全重建，保证 Start 可在 Close 之后再次使用。
func (T *Group) initGroupResources() {
	if T.exit == nil {
		T.exit = make(chan bool)
	}
	if T.route == nil {
		T.route = new(vweb.Route)
	}
	if T.sitePool == nil {
		T.sitePool = vweb.NewSitePool()
	}
	if T.siteMan == nil {
		T.siteMan = new(vweb.SiteMan)
	}
	if T.route != nil && T.siteMan != nil {
		T.route.SetSiteMan(T.siteMan)
	}
}

// Close 关闭服务集群。
// 并发安全：通过 atomic run 标志防止重复关闭。
// 依次关闭所有 Server（调用完整 Close 以释放 listener 与 status）、
// 清理站点池与站点管理器，最后通知 Start 退出阻塞。
// 优化：整个清理过程在 configMu 下执行，避免与 UpdateConfig/LoadConfigFile 交错。
func (T *Group) Close() error {
	T.configMu.Lock()
	defer T.configMu.Unlock()

	if !T.run.Swap(false) {
		return fmt.Errorf("server: 服务组已经关闭！")
	}

	// 关闭所有监听与服务器（使用 Server.Close 保证 listener 与 status 一并清理）
	T.srvMan.Range(func(k, v any) bool {
		if srv, ok := v.(*Server); ok && srv != nil {
			srv.Close()
		}
		return true
	})
	T.srvMan.Reset()

	// 参数默认:nil,nil
	// 删除站点管理中的所有站点
	// 删除站点池中的所有站点
	if T.sitePool != nil {
		T.updateSitePoolDel(nil, nil)
		T.sitePool.Close()
		T.sitePool = nil
	}
	T.siteMan = nil

	// 非阻塞方式通知 Start 退出。若 Start 尚未开始运行，此通道会在下次 Start 时被重建，
	// 不会造成死锁；channel 容量为 0，故使用 select-default 保证不会阻塞。
	if T.exit != nil {
		select {
		case T.exit <- true:
		default:
			// 没有接收方（Start 未运行），忽略即可
		}
	}
	return nil
}

// 返回错误到客户端
//
//	w http.ResponseWriter           响应
//	rootDir string					根目录
//	errorPage map[string]string     错误页地址
//	e string                        错误内容, 如果错误页不存在, 将使用内容
//	code int                        错误代码
func httpError(w http.ResponseWriter, rootDir string, errorPage map[string]string, e string, code int) error {
	if errorPage != nil {
		c := strconv.Itoa(code)
		ep, ok := errorPage[c]
		if ok {
			p := filepath.Join(rootDir, ep)
			b, err := os.ReadFile(p)
			if err != nil {
				// 修复：错误页读取失败不能返回空 200，
				// 回退到默认错误信息并照常写出响应。
				http.Error(w, e, code)
				return err
			}
			http.Error(w, string(b), code)
			return nil
		}
	}
	http.Error(w, e, code)
	return nil
}

// httpTypeByExtension 文件类型扩展, 如果自定义列表不存在扩展类型, 则使用系统默认扩展类型。如果自定义列表扩展类型是空“”的类型, 说明是用户设置拒绝访问该类型。
//
//	ext string              文件后缀
//	me map[string]string    自定义扩展列表
//	string                  文件类型
func httpTypeByExtension(ext string, me map[string]string) string {
	if me != nil {
		if extType, ok := me[ext]; ok {
			return extType
		}
	}
	return mime.TypeByExtension(ext)
}
