package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/456vv/verror"
	"github.com/456vv/vmap/v2"
	"github.com/456vv/vweb/v2"
	"github.com/456vv/vweb/v2/server/config"
	"golang.org/x/crypto/acme/autocert"
)

// 默认4K
var (
	defaultDataBufioSize int    = 4096
	Version              string = "Server/2.5.0"
)

// 上下文的Key, 在请求中可以使用
type contextKey struct {
	name string
}

func (T *contextKey) String() string { return "server context value " + T.name }

var ServerContextKey = &contextKey{"Server"}

// 响应完成设置
type atomicBool int32

func (T *atomicBool) isTrue() bool   { return atomic.LoadInt32((*int32)(T)) != 0 }
func (T *atomicBool) isFalse() bool  { return atomic.LoadInt32((*int32)(T)) != 1 }
func (T *atomicBool) setTrue() bool  { return !atomic.CompareAndSwapInt32((*int32)(T), 0, 1) }
func (T *atomicBool) setFalse() bool { return !atomic.CompareAndSwapInt32((*int32)(T), 1, 0) }

type siteInformation struct {
	config       *config.Site
	plugin       *plugin
	dynamicCache *vmap.Map // 缓存动态文件对象
}

func newSiteExtend() *siteInformation {
	return &siteInformation{
		plugin:       new(plugin),
		dynamicCache: new(vmap.Map),
	}
}

// Server 服务器,使用在 ServerGroup.srvMan 字段。
type Server struct {
	*http.Server // http服务器
	Addr         string
	l            listener
	status       atomicBool
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

	if T.l.ser == nil {
		T.l.ser = T
	}
}

func (T *Server) Serve(l net.Listener) error {
	T.init()
	addr := l.Addr().(*net.TCPAddr)
	ip := addr.IP.To4()
	if ip == nil {
		ip = addr.IP.To16()
	}
	T.Addr = net.JoinHostPort(ip.String(), strconv.Itoa(addr.Port))
	T.l.TCPListener = l.(*net.TCPListener)
	return T.Server.Serve(&T.l)
}

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

func (T *Server) ConfigConn(cc *config.Conn) error {
	if cc == nil {
		return verror.TrackError("server: *config.Conn 不可以为nil")
	}
	T.cConn = cc
	return nil
}

func (T *Server) ConfigServer(cs *config.Server) error {
	if cs == nil {
		return verror.TrackError("server: *config.Server 不可以为nil")
	}
	T.cServer = cs
	T.init()

	// 服务器配置
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

// TLS文件配置
func configTLSFile(c *tls.Config, conf *config.ServerTLS) error {
	c.NextProtos = conf.NextProtos
	c.SessionTicketsDisabled = conf.SessionTicketsDisabled
	c.MinVersion = conf.MinVersion
	c.MaxVersion = conf.MaxVersion
	c.DynamicRecordSizingDisabled = conf.DynamicRecordSizingDisabled

	if len(conf.CipherSuites) > 0 {
		copy(c.CipherSuites, conf.CipherSuites)
	} else {
		// 内部判断并使用默认的密码套件
		c.CipherSuites = nil
	}

	if len(conf.SetSessionTicketKeys) > 0 {
		c.SetSessionTicketKeys(conf.SetSessionTicketKeys)
	}

	var errStr string
	// 支持双向证书
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
		var errClientCA string
		for _, path := range conf.ClientCAs {
			// 打开文件
			caData, err := os.ReadFile(path)
			if err != nil {
				errClientCA = fmt.Sprintf("%s%s: %s\n", errClientCA, path, err.Error())
				continue
			}

			switch filepath.Ext(path) {
			case ".cer":
				{
					certificates, err := x509.ParseCertificates(caData)
					if err != nil {
						errClientCA = fmt.Sprintf("%s%s: %s\n", errClientCA, path, err.Error())
						continue
					}
					for _, cert := range certificates {
						c.ClientCAs.AddCert(cert)
					}
				}
			case ".pem", ".crt":
				{
					if !c.ClientCAs.AppendCertsFromPEM(caData) {
						errClientCA = fmt.Sprintf("%s%s: %s\n", errClientCA, path, "not is a valid PEM format")
						continue
					}
				}
			default:
				{
					errClientCA = fmt.Sprintf("TLS.RootCAs[\"%s\"], the file type is not supported, only support \".cer/.crt/.pem\" file type", path)
				}
			}
		}
		if errClientCA != "" {
			errStr = "解析客户端CA证书发生错误（CS.TLS.ClientCAs）: \n" + errClientCA
		}
	}

	c.Certificates = nil
	var errServerCert string
	for _, file := range conf.RootCAs {
		cert, err := tls.LoadX509KeyPair(file.CertFile, file.KeyFile)
		if err != nil {
			// 日志
			errServerCert = fmt.Sprintf("%s{CertFile:%q, KeyFile:%q}: %s\n", errServerCert, file.CertFile, file.KeyFile, err.Error())
			continue
		}
		c.Certificates = append(c.Certificates, cert)
	}
	if errServerCert != "" {
		errStr = errStr + "解析服务端证书发生错误（CS.TLS.RootCAs）: \n" + errServerCert
	}

	// 多证书。
	// c.BuildNameToCertificate()
	if errStr != "" {
		return verror.TrackErrorf("server: %s", errStr)
	}
	return nil
}

type Group struct {
	ErrorLog      *log.Logger                         // 错误日志文件
	DynamicModule map[string]vweb.DynamicTemplateFunc // 支持更多动态

	Route *vweb.Route // 地址路由

	CertManager *autocert.Manager // 自动申请证书 Let's Encrypt

	// srvMan 存储值类型是 *Server, 读取时需要转换类型
	srvMan   vmap.Map       // map[ip:port]*Server	服务器集
	sitePool *vweb.SitePool // 站点的池
	siteMan  *vweb.SiteMan  // 站点管理
	exit     chan bool      // 退出

	run atomicBool // 服务器启动了

	// 用于 .UpdateConfigFile 方法
	backupConf []byte         // 备份配置数据。如果是相同数据, 则不更新
	config     *config.Config // 配置
}

func NewServerGroup() *Group {
	return &Group{
		exit:     make(chan bool),
		ErrorLog: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
	}
}

// 增加一个服务器
//
//	laddr string	监听地址
//	srv *Server		服务器, 如果为nil, 则删除已存在的记录
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
		if T.Route != nil {
			// 使用路由
			srv.Handler = http.HandlerFunc(T.Route.ServeHTTP)
			if T.Route.HandlerError == nil {
				T.Route.HandlerError = http.HandlerFunc(T.serveHTTP)
			}
		} else {
			// 服务组默认处理
			srv.Handler = http.HandlerFunc(T.serveHTTP)
		}
	}

	srv.Handler = vweb.AutoCert(T.CertManager, srv.TLSConfig, srv.Handler)
}

// 读取一个服务器
//
//	laddr string	监听地址
//	*Server			服务器
//	bool			如果存在服务器, 返回true。否则返回false
func (T *Group) GetServer(laddr string) (*Server, bool) {
	if inf, ok := T.srvMan.GetHas(laddr); ok {
		return inf.(*Server), true
	}
	return nil, false
}

// 设置一个站点池, 随配置文件变动, pool 原来的保存内容可能会被删除或增加。
//
//	pool *vweb.SitePool	池
//	error				错误
func (T *Group) SetSitePool(pool *vweb.SitePool) error {
	if pool == nil {
		return errors.New("disallow setting up an empty site pool")
	}
	T.sitePool = pool
	return nil
}

// 设置一个站点管理, 随配置文件变动, man 原来的保存内容可能会被删除或增加。
//
//	man *vweb.SiteMan	站点管理
//	error				错误
func (T *Group) SetSiteMan(man *vweb.SiteMan) error {
	if man == nil {
		return errors.New("disallow setting up an empty site manage")
	}
	T.siteMan = man
	return nil
}

// serveHTTP 处理HTTP
//
//	rw http.ResponseWriter	响应
//	r *http.Request			请求
func (T *Group) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	//** 检查Host是否存在
	site, ok := T.siteMan.Get(r.Host)
	if !ok {
		// 如果在站点集中没有找到存在的Host, 则关闭连接。
		hj, ok := rw.(http.Hijacker)
		if !ok {
			// 500 服务器遇到了意料不到的情况, 不能完成客户的请求。
			http.Error(rw, "Not supported Hijacker", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			// 500 服务器遇到了意料不到的情况, 不能完成客户的请求。
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		// 直接关闭连接
		defer conn.Close()
		return
	}

	//** 配置
	var (
		se     = getSiteExtend(site)
		plugin = se.plugin
		dCache = se.dynamicCache
		conf   = se.config
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

		cacheStaticAtFunc func(string, io.Reader, int) (int, error)
		findStatic        bool
	)
	if rootDir == nil {
		// 没有设置外部根目录调用, 将使用默认的
		rootDir = conf.Directory.RootDir
	}

	// 直接读取缓存文件
	if conf.Dynamic.Cache && conf.Dynamic.CacheStaticFileDir != "" {
		uPath := r.URL.Path
		cDir := conf.Dynamic.CacheStaticFileDir
		cacheStaticAtFunc = staticAt(T, cDir, conf.Dynamic)
		if !filepath.IsAbs(cDir) {
			// 相对路径
			uPath = path.Join("/", cDir, r.URL.Path)
			cDir = rootDir(uPath)
			// 必须在相对的缓存路径前面加上根目录
			cacheStaticAtFunc = staticAt(T, path.Join(cDir, conf.Dynamic.CacheStaticFileDir), conf.Dynamic)
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
				rpath, rewried, err := fc.Rewrite(urlPath)
				if err != nil {
					T.ErrorLog.Printf("server: host(%s) 进行重写URL规则发发生错误：%s\n", r.Host, err.Error())
					continue
				}
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
	var (
		buffSize = conf.Property.BuffSize
		wh       = rw.Header()
	)
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
			handlerDynamic = inf.(*vweb.ServerHandlerDynamic)
			if conf.Dynamic.CacheParseTimeout != 0 {
				dCache.SetExpired(pagePath, time.Duration(conf.Dynamic.CacheParseTimeout))
			}
		} else {
			if ok {
				// 释放缓存
				dCache.Del(pagePath)
			}
			handlerDynamic = &vweb.ServerHandlerDynamic{
				PagePath: pagePath,
				Module:   T.DynamicModule,
			}
			if conf.Dynamic.Cache {
				// 时效
				dCache.Set(pagePath, handlerDynamic)
				if conf.Dynamic.CacheParseTimeout != 0 {
					dCache.SetExpired(pagePath, time.Duration(conf.Dynamic.CacheParseTimeout))
				}
				// 转存静态
				handlerDynamic.SaveStatic = cacheStaticAtFunc
			}
		}
		handlerDynamic.RootPath = rootPath
		handlerDynamic.BuffSize = buffSize
		handlerDynamic.Site = site
		handlerDynamic.Context = context.WithValue(r.Context(), vweb.PluginContextKey, plugin)

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
			BuffSize:    buffSize,
		}
		shs.ServeHTTP(rw, r)
	}
}

// 更新插件
func (T *Group) updatePluginConn(cSite config.Site) {
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
				httpC = inf.(*vweb.PluginHTTPClient)
			}

			if err := p.ConfigPluginHTTPClient(httpC); err != nil {
				T.ErrorLog.Printf("server: 名称 %s 的HTTP插件配错误, %s\n", name, err.Error())
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
				rpcC = inf.(*vweb.PluginRPCClient)
			}
			if err := p.ConfigPluginRPCClient(rpcC); err != nil {
				T.ErrorLog.Printf("server: 名称 %s 的HTTP插件配错误, %s", name, err.Error())
				continue
			}
			plugin.rpc.Store(name, rpcC)
		}
	} else {
		// 清除动态文件缓存
		dCache.Reset()
	}

	// 关闭无效的插件
	plugin.http.Range(func(name, client any) bool {
		if !strSliceContains(httpEffectiveNames, name.(string)) {
			plugin.http.Delete(name)

			client.(*vweb.PluginHTTPClient).Tr.CloseIdleConnections()
		}
		return true
	})
	plugin.rpc.Range(func(name, client any) bool {
		if !strSliceContains(rpcEffectiveNames, name.(string)) {
			plugin.rpc.Delete(name)

			client.(*vweb.PluginRPCClient).ConnPool.Close()
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
	vweb.CopyStruct(site.Sessions, &cSite.Session, func(name string, dsc, src reflect.Value) bool {
		return name == "Expired"
	})

	site.Sessions.Expired = time.Duration(cSite.Session.Expired) * time.Second
	site.RootDir = cSite.Directory.RootDir

	// 配置保存在网站扩展中
	getSiteExtend(site).config = &cSite
}

var siteInformationName int

func getSiteExtend(site *vweb.Site) *siteInformation {
	se, ok := site.Extend.Get(&siteInformationName).(*siteInformation)
	if !ok {
		se = newSiteExtend()
		site.Extend.Set(&siteInformationName, se)
	}
	return se
}

// 更新站点池删除, 过滤并删除无效的站点池。
//
//	siteEffectiveIdent []string      现有的站点列表
func (T *Group) updateSitePoolDel(siteEffectiveIdent []string) {
	T.sitePool.RangeSite(func(name string, site *vweb.Site) bool {
		if !strSliceContains(siteEffectiveIdent, name) {
			// 从池中删除
			T.sitePool.DelSite(name)

			// 设置过期时间
			sec := time.Now().Unix()
			site.Sessions.Expired = time.Duration(^sec) * time.Second
			site.Sessions.ProcessDeadAll()
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
			return verror.TrackErrorf("server: 配置中出现站点惟一名(Identity)为 \"\", 需要设定一个名称。")
		}

		if cSite.Status {
			// 复制Session的配置
			if cSite.Session.PublicName != "" && !conf.Public.ConfigSiteSession(&cSite.Session, nil) {
				T.ErrorLog.Printf("server: %s 站点的私有Session与公共Session合并失败\n", cSite.Identity)
			}

			// 复制Header的配置
			merge := func(name string, dsc, src reflect.Value) bool {
				switch name {
				case "MIME", "Header":
					mr := src.MapRange()
					for mr.Next() {
						v := mr.Value()
						if v.IsZero() {
							v = reflect.Value{}
						}
						dsc.SetMapIndex(mr.Key(), v)
					}
					return true
				default:
				}
				return false
			}
			if cSite.Header.PublicName != "" && !conf.Public.ConfigSiteHeader(&cSite.Header, merge) {
				T.ErrorLog.Printf("server: %s 站点的私有Header与公共Header合并失败\n", cSite.Identity)
			}

			// 复制Plugin的配置
			merge = func(name string, dsc, src reflect.Value) bool {
				switch name {
				case "TLS":
					return !src.Elem().IsValid()
				default:
				}
				return false
			}
			for name, pRPC := range cSite.Plugin.RPC {
				if pRPC.PublicName != "" {
					if conf.Public.Plugin.ConfigSitePluginRPC(&pRPC, merge) {
						cSite.Plugin.RPC[name] = pRPC
						continue
					}
					T.ErrorLog.Printf("server: %s 站点的 Plugin RPC %s 合并失败\n", cSite.Identity, name)
				}
			}
			for name, pHTTP := range cSite.Plugin.HTTP {
				if pHTTP.PublicName != "" {
					if conf.Public.Plugin.ConfigSitePluginHTTP(&pHTTP, merge) {
						cSite.Plugin.HTTP[name] = pHTTP
						continue
					}
					T.ErrorLog.Printf("server: %s 站点的 Plugin HTTP %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制Forward的配置
			for name, forward := range cSite.Forward {
				if forward.PublicName != "" {
					if conf.Public.ConfigSiteForward(&forward, nil) {
						cSite.Forward[name] = forward
						continue
					}
					T.ErrorLog.Printf("server: %s 站点的 Forward %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制Property的配置
			if cSite.Property.PublicName != "" && !conf.Public.ConfigSiteProperty(&cSite.Property, merge) {
				T.ErrorLog.Printf("server: %s 站点的私有Property与公共Property合并失败\n", cSite.Identity)
			}

			// 复制Dynamic的配置
			if cSite.Dynamic.PublicName != "" && !conf.Public.ConfigSiteDynamic(&cSite.Dynamic, merge) {
				T.ErrorLog.Printf("server: %s 站点的私有Dynamic与公共Dynamic合并失败\n", cSite.Identity)
			}
			if cSite.Dynamic.CacheParseTimeout != 0 {
				cSite.Dynamic.CacheParseTimeout *= int64(time.Second)
			}
			if cSite.Dynamic.CacheStaticTimeout != 0 {
				cSite.Dynamic.CacheStaticTimeout *= int64(time.Second)
			}
			// 预选分配池, 初始化站点
			T.updateSitePoolAdd(cSite)

			// 集中名称
			siteEffectiveIdent = append(siteEffectiveIdent, cSite.Identity)

			// 集中站点Host
			// 可能有多个站点绑定了同一个Host, 只有最后一个是有效的
			siteEffectiveHosts = append(siteEffectiveHosts, cSite.Host...)
		}

		// 插件不关网站是否开启
		// 网站不开启, 否关闭插件
		T.updatePluginConn(cSite)
	}

	// 更新网站
	T.siteMan.Range(func(host string, site *vweb.Site) bool {
		if !strSliceContains(siteEffectiveHosts, host) {
			T.siteMan.Add(host, nil)
		}
		return true
	})

	// 删除池中不存在的配置
	T.updateSitePoolDel(siteEffectiveIdent)

	return nil
}

func (T *Group) newServer(laddr string) *Server {
	if inf, ok := T.srvMan.GetHas(laddr); ok {
		return inf.(*Server)
	}
	srv := new(Server)
	srv.Addr = laddr
	return srv
}

// 启动监听端口
func (T *Group) listenStart(laddr string, conf config.Listen) error {
	srv := T.newServer(laddr)
	if err := srv.ConfigConn(&conf.CC); err != nil {
		return err
	}
	if err := srv.ConfigServer(&conf.CS); err != nil {
		return err
	}
	T.defaultServerConfig(srv)
	go T.serve(srv)
	return nil
}

// 关闭监听
func (T *Group) listenStop(laddr string) (err error) {
	if inf, ok := T.srvMan.GetHas(laddr); ok {
		if srv, ok := inf.(*Server); ok {
			if srv.Server != nil {
				if srv.cServer != nil && srv.cServer.ShutdownConn {
					// 不要即时关闭正在下载的连接
					return srv.Server.Shutdown(context.Background())
				} else {
					return srv.Server.Close()
				}
			}
		}
	}
	return nil
}

// 监听决定, 区分是开启还是关闭监听。
func (T *Group) updateConfigServers(conf config.Servers) {
	// 如果在新的IP例表中没有找到已经存在的开放监听端口IP, 而停止监听此IP
	T.srvMan.Range(func(key, val any) bool {
		ip := key.(string)
		if _, ok := conf.Listen[ip]; !ok {
			if err := T.listenStop(ip); err != nil {
				T.ErrorLog.Println(err.Error())
			}
		}
		return true
	})

	// 如果还没开启监听, 则启动他
	for laddr, cl := range conf.Listen {
		if cl.Status {
			// 复制的配置
			if cl.CC.PublicName != "" && !conf.Public.ConfigConn(&cl.CC, nil) {
				T.ErrorLog.Printf("server: %s 地址的私有CC与公共CC合并失败\n", laddr)
			}

			exclude := func(name string, dsc, src reflect.Value) bool {
				switch name {
				case "TLS":
					return !src.Elem().IsValid()
				default:
				}
				return false
			}
			if cl.CS.PublicName != "" && !conf.Public.ConfigServer(&cl.CS, exclude) {
				T.ErrorLog.Printf("server: %s 地址的私有CS与公共CS合并失败\n", laddr)
			}
			// 启动监听
			if err := T.listenStart(laddr, cl); err != nil {
				T.ErrorLog.Println(err.Error())
			}
		} else {
			// 停止监听
			if err := T.listenStop(laddr); err != nil {
				T.ErrorLog.Println(err.Error())
			}
		}
	}
}

// 挂载本地配置文件。
//
//	p string        文件路径
//	ok bool			true配置文件被修改过, false没有变动
//	err error       错误
func (T *Group) LoadConfigFile(p string) (ok bool, err error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	// 判断文件是否有改动
	if bytes.Equal(b, T.backupConf) {
		return false, nil
	}
	T.backupConf = b

	// 解析配置文件
	var conf config.Config
	r := bytes.NewReader(b)
	if err = conf.ParseReader(r); err != nil {
		return
	}
	// 更新配置文件
	if err = T.UpdateConfig(&conf); err != nil {
		return
	}
	return true, nil
}

// 更新配置并把配置分配到各个地方。不检查改动, 直接更新。更新配置需要调用 .Start 方法之后才生效。
//
//	conf *config.Config        配置
//	error               错误
func (T *Group) UpdateConfig(conf *config.Config) error {
	if conf == nil {
		return verror.TrackErrorf("server: conf 为 nil, 无法更新。")
	}
	T.config = conf
	if T.run.isTrue() {
		// 更新网站配置
		if err := T.updateConfigSites(conf.Sites); err != nil {
			T.ErrorLog.Println(err.Error())
		}
		// 更新服务器配置
		T.updateConfigServers(conf.Servers)
	}
	return nil
}

// serve 启动服务器
func (T *Group) serve(srv *Server) {
	if srv.status.setTrue() {
		return
	}
	T.srvMan.Set(srv.Addr, srv)
	defer T.srvMan.Del(srv.Addr)
	err := srv.ListenAndServe() // 阻塞
	srv.status.setFalse()       // 退出
	if err != nil {
		T.ErrorLog.Printf("server: ip(%s), %s\n", srv.Addr, err.Error())
	}
}

// 启动服务集群
//
//	error   错误
func (T *Group) Start() error {
	if T.run.setTrue() {
		return verror.TrackErrorf("server: 服务组已经开启。")
	}

	// 站点池
	if T.sitePool == nil {
		pool := vweb.DefaultSitePool
		if err := pool.Start(); err == nil {
			defer pool.Close()
		}
		T.sitePool = pool
	}

	// 站点管理
	if T.siteMan == nil {
		T.siteMan = new(vweb.SiteMan)
	}

	// 刷新配置
	if T.config != nil {
		T.UpdateConfig(T.config)
	}

	// 等待退出
	<-T.exit
	return nil
}

// 关闭服务集群
//
//	error   错误
func (T *Group) Close() error {
	if T.run.setFalse() {
		return verror.TrackErrorf("server: 服务组已经关闭！")
	}

	// 关闭监听
	T.srvMan.Range(func(k, v any) bool {
		if srv, ok := v.(*Server); ok {
			if srv.Server != nil {
				srv.Server.Close()
			}
		}
		return true
	})
	T.srvMan.Reset()

	T.siteMan.Range(func(host string, site *vweb.Site) bool {
		T.siteMan.Add(host, nil)
		T.sitePool.DelSite(site.PoolName())

		// 关闭插件中的空闲连接
		plugin := getSiteExtend(site).plugin
		plugin.http.Range(func(name, client any) bool {
			plugin.http.Delete(name)
			client.(*vweb.PluginHTTPClient).Tr.CloseIdleConnections()
			return true
		})
		plugin.rpc.Range(func(name, client any) bool {
			plugin.rpc.Delete(name)
			client.(*vweb.PluginRPCClient).ConnPool.Close()
			return true
		})

		// 释放动态缓存
		getSiteExtend(site).dynamicCache.Reset()

		// 网站扩展清空
		site.Extend.Reset()

		return true
	})

	T.sitePool = nil
	T.siteMan = nil
	T.exit <- true
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
				return err
			} else {
				http.Error(w, string(b), code)
				return nil
			}
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
