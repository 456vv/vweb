package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"maps"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/456vv/vmap/v2"
	"github.com/456vv/vweb/v3"
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

const sessionFreeWaitTimeout = 3 * time.Second

// Server 封装 http.Server，支持自定义 TCP 选项、TLS 与并发安全状态。
type Server struct {
	*http.Server // http服务器
	Addr         string
	l            listener
	status       atomic.Bool    // 运行状态（true=正在服务）
	cServer      *config.Server // 用于服务器
	cConn        *config.Conn   // 用于连接

	// 原子状态只能保护单个值，不能保护 “检查运行状态 -> 修改配置/初始化监听器”的复合操作。
	mu sync.RWMutex

	// http.Server.Close 后不能重新 Serve；
	// 同时阻止 Close 先执行、后台 goroutine 随后才开始监听的情况。
	closed bool
}

// millis 把配置中的毫秒转为 Duration。
// 毫秒数过大时，int64 乘 time.Millisecond 会溢出成负数，读超时会变成立刻超时。
func millis(ms int64) time.Duration {
	if ms <= 0 {
		return 0
	}
	maxMs := int64(time.Duration(1<<63-1) / time.Millisecond)
	if ms > maxMs {
		ms = maxMs
	}
	return time.Duration(ms) * time.Millisecond
}

// seconds 与 millis 相同，单位是秒。
func seconds(s int64) time.Duration {
	if s <= 0 {
		return 0
	}
	maxS := int64(time.Duration(1<<63-1) / time.Second)
	if s > maxS {
		s = maxS
	}
	return time.Duration(s) * time.Second
}

func (T *Server) init() {
	if T.Server == nil {
		T.Server = new(http.Server)
	}
	if T.Server.BaseContext == nil {
		T.Server.BaseContext = func(l net.Listener) context.Context {
			ctx := context.WithValue(context.Background(), ServerContextKey, T)
			return context.WithValue(ctx, vweb.ListenerContextKey, l)
		}
	}
	if T.Server.ConnContext == nil {
		T.Server.ConnContext = func(ctx context.Context, rwc net.Conn) context.Context {
			return context.WithValue(ctx, vweb.ConnContextKey, rwc)
		}
	}
}

// Serve 启动服务。
// 启动准备与 ConfigConn、ConfigServer、Close 使用同一把锁。
// Serve 启动服务。
// 启动准备与 ConfigConn、ConfigServer、Close 使用同一把锁。
func (T *Server) Serve(l net.Listener) error {
	if err := func() error {
		T.mu.Lock()
		defer T.mu.Unlock()
		return T.prepareServeLocked(l)
	}(); err != nil {
		return err
	}
	return T.servePrepared()
}

// asTCPListener 统一校验并转换 listener，避免在 3 处重复 typed-nil / 类型检查。
// 返回的 *net.TCPListener 非 nil。
func asTCPListener(l net.Listener) (*net.TCPListener, error) {
	if l == nil {
		return nil, errors.New("server: listener is nil")
	}
	lv := reflect.ValueOf(l)
	switch lv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		if lv.IsNil() {
			return nil, errors.New("server: listener is nil")
		}
	}
	switch ln := l.(type) {
	case *net.TCPListener:
		return ln, nil
	case interface{ TCPListener() *net.TCPListener }:
		if tcpLn := ln.TCPListener(); tcpLn != nil {
			return tcpLn, nil
		}
	}
	return nil, fmt.Errorf("server: listener must be *net.TCPListener or provide TCPListener(), got %T", l)
}

// buildAddrFromTCP 从 *net.TCPAddr 构建监听字符串。
// 修复 #2：只有 IP 非 nil 时才拼 Zone；避免 [%zone] 非法地址。
func buildAddrFromTCP(ta *net.TCPAddr) string {
	ip := ""
	if ta.IP != nil {
		ip = ta.IP.String()
		if ta.Zone != "" {
			ip += "%" + ta.Zone
		}
	}
	return net.JoinHostPort(ip, strconv.Itoa(ta.Port))
}

// prepareServeLocked 准备监听。调用方必须持有 T.mu 写锁。
func (T *Server) prepareServeLocked(l net.Listener) error {
	if T.closed {
		return http.ErrServerClosed
	}
	if T.status.Load() {
		return errors.New("server: already running")
	}

	tcpLn, err := asTCPListener(l)
	if err != nil {
		return err
	}

	ta, ok := tcpLn.Addr().(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("server: listener address must be *net.TCPAddr, got %T", tcpLn.Addr())
	}

	T.Addr = buildAddrFromTCP(ta)
	T.l.TCPListener = tcpLn
	T.l.closed.Store(false)
	T.init()
	T.Server.Addr = T.Addr

	// 只在所有准备完成后发布运行状态；
	// 整个准备过程已经由 T.mu 串行化。
	T.status.Store(true)
	return nil
}

// servePrepared 执行已准备好的服务循环。
func (T *Server) servePrepared() error {
	if T.Server == nil {
		T.status.Store(false)
		return errors.New("server: http.Server is nil")
	}
	if T.Server.Handler == nil {
		T.status.Store(false)
		return errors.New("server: Handler is nil; set Group.serveHTTP or a custom Handler before Serve")
	}
	// 运行状态只在 Serve 真正退出时清除。
	// Close 不再提前清除状态，避免旧循环尚未退出又允许重新配置。
	defer T.status.Store(false)
	return T.Server.Serve(&T.l)
}

// ListenAndServe 在 T.Addr 上监听并服务。
func (T *Server) ListenAndServe() error {
	T.mu.Lock()
	if T.closed {
		T.mu.Unlock()
		return http.ErrServerClosed
	}
	if T.status.Load() {
		T.mu.Unlock()
		return errors.New("server: already running")
	}
	if T.Addr == "" {
		T.mu.Unlock()
		return errors.New("server: no address")
	}

	l, err := net.Listen("tcp", T.Addr)
	if err != nil {
		T.mu.Unlock()
		return err
	}
	// 这里的 listener 由本方法创建，准备失败必须关闭。
	defer l.Close()

	err = T.prepareServeLocked(l)
	T.mu.Unlock()
	if err != nil {
		return err
	}
	return T.servePrepared()
}

// IsRunning 返回服务器当前是否处于服务状态（并发安全）。
func (T *Server) IsRunning() bool {
	return T.status.Load()
}

// Close 立即关闭服务器与底层 Listener。
func (T *Server) Close() error {
	T.mu.Lock()
	defer T.mu.Unlock()

	// 即使 Serve goroutine 尚未执行，也不能在 Close 之后重新监听。
	T.closed = true

	var errs []error
	if T.Server != nil {
		if err := T.Server.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := T.l.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Shutdown 优雅关闭：先停止接新连接，再等待进行中的请求结束（受 ctx 取消约束）。
func (T *Server) Shutdown(ctx context.Context) error {
	T.mu.Lock()
	T.closed = true
	srv := T.Server
	T.mu.Unlock()

	var errs []error
	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	// Shutdown 成功后 listener 通常已关；再调一次保证自定义 listener 状态一致（幂等）。
	if err := T.l.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// configSnapshot 在锁下读取已提交的配置
func (T *Server) configSnapshot() (cs *config.Server, cc *config.Conn) {
	T.mu.RLock()
	defer T.mu.RUnlock()
	return T.cServer, T.cConn
}

// ConfigConn 配置连接级 TCP 选项（KeepAlive、NoDelay、缓冲区、Deadline 等）。
func (T *Server) ConfigConn(cc *config.Conn) error {
	if cc == nil {
		return errors.New("server: *config.Conn 不可以为nil")
	}

	T.mu.Lock()
	defer T.mu.Unlock()

	if T.closed {
		return http.ErrServerClosed
	}
	if T.status.Load() {
		return errors.New("server: 服务运行中，不能修改连接配置")
	}

	cp := *cc
	T.cConn = &cp // 仍保存完整配置，供 srvConfigChanged 比较

	probe := cp
	probe.PublicName = ""
	if probe == (config.Conn{}) {
		T.l.upConn(nil)
	} else {
		T.l.upConn(T.cConn)
	}
	return nil
}

// ConfigServer 配置服务器参数（超时、TLS、KeepAlive 等）。
func (T *Server) ConfigServer(cs *config.Server) error {
	if cs == nil {
		return errors.New("server: *config.Server 不可以为nil")
	}

	T.mu.Lock()
	defer T.mu.Unlock()

	if T.closed {
		return http.ErrServerClosed
	}
	if T.status.Load() {
		return errors.New("server: 服务运行中，不能修改服务器配置")
	}

	T.init()

	var tlsConfig *tls.Config
	if cs.TLS != nil {
		// 在临时对象上加载 TLS；失败时不修改当前服务器。
		tlsConfig = new(tls.Config)
		if err := configTLSFile(tlsConfig, cs.TLS); err != nil {
			return err
		}
	}

	T.Server.ReadTimeout = millis(cs.ReadTimeout)
	T.Server.WriteTimeout = millis(cs.WriteTimeout)
	T.Server.ReadHeaderTimeout = millis(cs.ReadHeaderTimeout)
	T.Server.IdleTimeout = millis(cs.IdleTimeout)
	T.Server.MaxHeaderBytes = cs.MaxHeaderBytes
	T.Server.DisableGeneralOptionsHandler = cs.DisableGeneralOptionsHandler
	T.Server.SetKeepAlivesEnabled(!cs.DisableKeepAlives)

	T.Server.TLSConfig = tlsConfig
	T.l.upTLS(tlsConfig)

	T.cServer = cloneConfigServer(cs)
	return nil
}

// cloneConfigServer 深拷贝 config.Server（含 TLS 切片），供快照比较与并发隔离。
func cloneConfigServer(cs *config.Server) *config.Server {
	if cs == nil {
		return nil
	}
	cp := *cs
	if cs.TLS != nil {
		tlsCopy := *cs.TLS
		if len(cs.TLS.NextProtos) > 0 {
			tlsCopy.NextProtos = append([]string(nil), cs.TLS.NextProtos...)
		}
		if len(cs.TLS.CipherSuites) > 0 {
			tlsCopy.CipherSuites = append([]uint16(nil), cs.TLS.CipherSuites...)
		}
		if len(cs.TLS.SetSessionTicketKeys) > 0 {
			tlsCopy.SetSessionTicketKeys = append([][32]byte(nil), cs.TLS.SetSessionTicketKeys...)
		}
		if len(cs.TLS.ClientCAs) > 0 {
			tlsCopy.ClientCAs = append([]string(nil), cs.TLS.ClientCAs...)
		}
		if n := len(cs.TLS.RootCAs); n > 0 {
			tlsCopy.RootCAs = make([]config.ServerTLSFile, n)
			copy(tlsCopy.RootCAs, cs.TLS.RootCAs)
		}
		cp.TLS = &tlsCopy
	}
	return &cp
}

// configTLSFile 加载服务端证书、客户端 CA，配置协议版本、密码套件、SessionTicket 等。
// 支持 .pem/.crt（PEM）与 .cer（DER）格式。
func configTLSFile(c *tls.Config, conf *config.ServerTLS) error {
	if conf == nil {
		// 避免空 conf 解引用 panic。
		return errors.New("server: TLS config is nil")
	}
	c.NextProtos = append([]string(nil), conf.NextProtos...)
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
	clientCAAdded := false
	if len(conf.ClientCAs) != 0 {
		if c.ClientCAs == nil {
			if certPool, err := x509.SystemCertPool(); err == nil && certPool != nil {
				c.ClientCAs = certPool.Clone()
			} else {
				c.ClientCAs = x509.NewCertPool()
			}
		}
		n, errs := loadCertPool(c.ClientCAs, conf.ClientCAs)
		if n > 0 {
			clientCAAdded = true
		}
		if len(errs) > 0 {
			errBuilder.WriteString("解析客户端CA证书发生错误（CS.TLS.ClientCAs）:\n")
			for _, e := range errs {
				errBuilder.WriteString(e)
				errBuilder.WriteByte('\n')
			}
		}
		// 仅当至少成功添加一张客户端 CA 时才提升 ClientAuth，
		// 避免空池导致所有连接被拒。
		// 注意：tls.Config 零值就是 NoClientCert，无法区分"用户未设"与"用户显式设 NoClientCert"；
		// 这里保持原有行为，仅在 NoClientCert 时提升。
		if clientCAAdded && c.ClientAuth == tls.NoClientCert {
			c.ClientAuth = tls.RequireAndVerifyClientCert
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

	// 显式配置了 TLS.RootCAs 却一张服务端证书都没有时，握手必然失败，启动期直接报错更可预期。
	if len(conf.RootCAs) > 0 && len(c.Certificates) == 0 {
		errBuilder.WriteString("TLS.RootCAs 已配置但未能加载任何服务端证书\n")
	}

	// 多证书。
	// c.BuildNameToCertificate() // 已在 Go 1.14+ 废弃，tls 内部自动处理
	if errBuilder.Len() > 0 {
		return fmt.Errorf("server: %s", errBuilder.String())
	}
	return nil
}

// loadCertPool 加载客户端 CA（支持 .cer / .crt / .pem）。
// 返回成功追加的数量与聚合错误。
func loadCertPool(pool *x509.CertPool, paths []string) (added int, errs []string) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", p, err))
			continue
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".cer":
			certs, err := x509.ParseCertificates(data)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %s", p, err))
				continue
			}
			for _, cert := range certs {
				pool.AddCert(cert)
				added++
			}
		case ".pem", ".crt":
			if !pool.AppendCertsFromPEM(data) {
				errs = append(errs, fmt.Sprintf("%s: not a valid PEM format", p))
			} else {
				added++
			}
		default:
			errs = append(errs, fmt.Sprintf("TLS.ClientCAs[%q]: unsupported type, only .cer/.crt/.pem", p))
		}
	}
	return added, errs
}

// compiledForward 是加载期编译好的转发规则。
type compiledForward struct {
	rewriter     *vweb.ForwardRewriter
	redirectCode int
	end          bool
}

// siteExtendInfo 一个站点的扩展信息。
type siteExtendInfo struct {
	configMu     sync.RWMutex
	configSite   *config.Site
	forwards     map[string][]compiledForward
	plugin       *plugin
	dynamicCache *vmap.Map // 缓存动态文件对象
	dynamicExt   map[string]struct{}
	dynamicMu    sync.Mutex
}

// snapshot 一次取出配置和转发表，避免两次加锁读到不一致的两份。
func (T *siteExtendInfo) snapshot() (conf *config.Site, forwards map[string][]compiledForward, dynamicExt map[string]struct{}) {
	T.configMu.RLock()
	conf = T.configSite
	forwards = T.forwards
	dynamicExt = T.dynamicExt
	T.configMu.RUnlock()
	return
}

// publish 整份替换配置和转发表。
func (T *siteExtendInfo) publish(c *config.Site, forwards map[string][]compiledForward) {
	var extSet map[string]struct{}
	if c != nil && len(c.Dynamic.Ext) > 0 {
		extSet = make(map[string]struct{}, len(c.Dynamic.Ext))
		for _, e := range c.Dynamic.Ext {
			e = strings.TrimSpace(e)
			// path.Ext 返回带点的后缀。配置写成 "go" 时原实现永远匹配不上，
			// 动态源码被当作静态文件原样下发。空字符串保留原义（匹配无扩展名文件）。
			if e != "" && !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			extSet[strings.ToLower(e)] = struct{}{}
		}
	}
	T.configMu.Lock()
	T.configSite = c
	T.forwards = forwards
	T.dynamicExt = extSet
	T.configMu.Unlock()
}

func newSiteExtendInfo() *siteExtendInfo {
	return &siteExtendInfo{
		configSite:   nil,
		plugin:       new(plugin),
		dynamicCache: new(vmap.Map),
		dynamicExt:   make(map[string]struct{}),
	}
}

// siteExtendInitMu 仅保护"首次创建并写入 site.Extend"的慢路径。
var siteExtendInitMu sync.Mutex

func getSiteExtend(site *vweb.Site) *siteExtendInfo {
	if site == nil {
		return nil
	}

	// 快路径：已初始化，无锁返回
	if se, ok := site.Extend.Get(site).(*siteExtendInfo); ok && se != nil {
		return se
	}

	// 慢路径：串行化首次创建
	siteExtendInitMu.Lock()
	defer siteExtendInitMu.Unlock()

	// 双重检查：等待锁期间可能已被其他 goroutine 安装
	if se, ok := site.Extend.Get(site).(*siteExtendInfo); ok && se != nil {
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
	srvMan   vmap.Map       // map[ip:port]*Server	服务器集
	sitePool *vweb.SitePool // 站点的池
	siteMan  *vweb.SiteMan  // 站点管理
	exit     chan bool      // 退出

	run atomic.Bool // 服务器启动了

	// 用于 .UpdateConfigFile 方法
	configFileModTime time.Time
	config            *config.Config // 配置
	configMu          sync.Mutex
	closing           atomic.Bool
}

func NewGroup() *Group {
	g := &Group{
		exit:     make(chan bool),
		ErrorLog: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
		route:    new(vweb.Route),
		siteMan:  new(vweb.SiteMan),
		sitePool: vweb.NewSitePool(),
	}
	g.route.SetSiteMan(g.siteMan)
	g.route.HandlerError = http.HandlerFunc(g.serveHTTP)
	return g
}

func (T *Group) errorf(format string, args ...any) {
	if T == nil || T.ErrorLog == nil {
		return
	}
	T.ErrorLog.Printf(format, args...)
}

func (T *Group) SetServer(laddr string, srv *Server) error {
	if srv == nil {
		// 保留原语义：这里只删除登记，不隐式关闭调用方的服务器。
		T.srvMan.Del(laddr)
		return nil
	}
	if err := T.defaultServerConfig(srv); err != nil {
		return err
	}
	T.srvMan.Set(laddr, srv)
	return nil
}

func (T *Group) defaultServerConfig(srv *Server) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if srv.closed {
		return http.ErrServerClosed
	}
	if srv.status.Load() {
		return errors.New("server: 服务运行中，不能修改默认服务器配置")
	}

	srv.init()

	routeHandler := http.HandlerFunc(T.route.ServeHTTP)
	srv.Handler = vweb.AutoCert(T.CertManager, srv.TLSConfig, routeHandler)
	return nil
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

// SiteMan 站点管理器, 用于获取和设置站点信息。
func (T *Group) SiteMan() *vweb.SiteMan {
	return T.siteMan
}

// serveHTTP 处理HTTP
//
//	rw http.ResponseWriter	响应
//	r *http.Request			请求
func (T *Group) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	if T.closing.Load() {
		http.Error(rw, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	//** 检查Host是否存在
	// 优先用 Route 已写入的 Site，避免重复查 Host；未知 Host 仍由 Route Hijack，不会进到这里。
	site, _ := r.Context().Value(vweb.SiteContextKey).(*vweb.Site)
	if site == nil {
		if T.siteMan == nil {
			http.Error(rw, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		var ok bool
		site, ok = T.siteMan.Get(r.Host)
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
	}

	//** 配置（通过 snapshot() 一次取出配置和转发表）
	se := getSiteExtend(site)
	// 热重载删除站点后残留请求可能读到空扩展；避免对 nil se 解引用 panic。
	if se == nil {
		http.Error(rw, "The site extend is nil\n", http.StatusInternalServerError)
		return
	}
	conf, forwards, dynamicExt := se.snapshot()
	plugin := se.plugin
	dCache := se.dynamicCache
	if conf == nil {
		// 500 服务器遇到了意料不到的情况, 不能完成客户的请求。
		http.Error(rw, "The configuration is nil\n", http.StatusInternalServerError)
		return
	}

	//** 静态文件
	var (
		err        error
		rootDir    = conf.Directory.RootDir
		rootPath   string
		pagePath   string
		findStatic bool
	)

	// 直接读取缓存文件
	if conf.Dynamic.Cache && conf.Dynamic.CacheStaticFileDir != "" {
		uPath := r.URL.Path
		cDir := conf.Dynamic.CacheStaticFileDir
		if !filepath.IsAbs(cDir) {
			uPath = path.Join("/", cDir, r.URL.Path)
			cDir = rootDir(uPath)
		}
		if fInfo, pPath, err := vweb.PagePath(cDir, uPath, conf.IndexFile); err == nil {
			t := time.Now()
			cSecond := seconds(conf.Dynamic.CacheStaticTimeout)
			// 0/负数动态解析缓存表示不过期；而 Add(0).After(now) 恒为 false。
			if cSecond <= 0 || fInfo.ModTime().Add(cSecond).After(t) {
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
		urlPath := r.URL.Path
		if len(forwards) != 0 {
			var rules []compiledForward
			derogatoryDomain(r.Host, func(h string) (ok bool) {
				rules, ok = forwards[h]
				return
			})
			for i := range rules {
				fc := &rules[i]
				if fc.rewriter == nil {
					continue
				}
				rpath, rewried := fc.rewriter.Rewrite(urlPath)
				if rewried {
					if fc.redirectCode != 0 {
						// 重定向,并退出
						http.Redirect(rw, r, rpath, fc.redirectCode)
						return
					}

					urlPath = rpath

					if fc.end {
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
		fileExt = strings.ToLower(path.Ext(pagePath))
		header  = conf.Header
	)

	if header.MIME != nil {
		if ct, ok := header.MIME[fileExt]; ok && ct == "" {
			httpError(rw, rootPath, conf.ErrorPage, "Forbidden file type", http.StatusForbidden)
			return
		}
	}
	contentType := httpTypeByExtension(fileExt, header.MIME)

	//** 文件固定标头准备

	wh := rw.Header()
	if contentType != "" {
		wh.Set("Content-Type", contentType)
	}
	wh.Set("Server", Version)

	//** 文件动态静态分离
	if _, isDynamic := dynamicExt[fileExt]; isDynamic {
		// 动态页面

		if contentType == "" {
			wh.Set("Content-Type", "text/html; charset=utf-8")
		}

		// 读取指定后缀类型的标头内容
		if header.Dynamic != nil {
			siteHeaderType(wh, header.Dynamic, fileExt)
		}

		// 处理动态格式
		var (
			handlerDynamic    *vweb.ServerHandlerDynamic
			cacheParseTimeout = seconds(conf.Dynamic.CacheParseTimeout)
		)

		// 缓存存在，从缓存读取
		if inf, ok := dCache.GetHas(pagePath); ok {
			handlerDynamic, _ = inf.(*vweb.ServerHandlerDynamic)
			if handlerDynamic != nil {
				if conf.Dynamic.Cache && !handlerDynamic.Closed() {
					if cacheParseTimeout != 0 {
						// 缓存过期调用函数
						dCache.SetExpiredCall(pagePath, cacheParseTimeout, func(a any) {
							if d, ok := a.(*vweb.ServerHandlerDynamic); ok {
								d.Close()
							}
						})
					} else {
						// 缴存改为无期限
						dCache.SetExpired(pagePath, 0)
					}
				} else {
					dCache.Del(pagePath)
					handlerDynamic.Close()
					handlerDynamic = nil
				}
			}
		}

		if handlerDynamic == nil || handlerDynamic.Closed() {
			handlerDynamic = &vweb.ServerHandlerDynamic{
				RootPath: rootPath,
				PagePath: pagePath,
				Module:   T.Module,
				Site:     site,
				ErrorLog: T.ErrorLog,
			}
			if conf.Dynamic.Cache {
				se.dynamicMu.Lock()
				if !dCache.Has(pagePath) {
					// 时效
					dCache.Set(pagePath, handlerDynamic)
					if cacheParseTimeout != 0 {
						dCache.SetExpiredCall(pagePath, cacheParseTimeout, func(a any) {
							if d, ok := a.(*vweb.ServerHandlerDynamic); ok {
								d.Close()
							}
						})
					}
				} else {
					// 这个情况是存多线程下，已经存在缓存，关闭这个动态处理
					defer handlerDynamic.Close()
				}
				se.dynamicMu.Unlock()
			} else {
				defer handlerDynamic.Close()
			}
		}

		// 在 route.go 已有在上下文中设置 site/router
		ctx := r.Context()
		ctx = context.WithValue(ctx, vweb.PluginContextKey, vweb.Pluginer(plugin))
		ctx = context.WithValue(ctx, vweb.SiteContextKey, site)
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

// closeDynamicCache 先关闭缓存里的动态处理器
func closeDynamicCache(dCache *vmap.Map) {
	if dCache == nil {
		return
	}
	var handlers []*vweb.ServerHandlerDynamic
	dCache.Range(func(_, v any) bool {
		if d, ok := v.(*vweb.ServerHandlerDynamic); ok && d != nil {
			handlers = append(handlers, d)
		}
		return true
	})
	dCache.Reset()
	for _, d := range handlers {
		d.Close()
	}
}

// 更新插件
func (T *Group) updateExtInfo(cSite config.Site) {
	var (
		site   = T.sitePool.NewSite(cSite.Identity)
		se     = getSiteExtend(site)
		plugin = se.plugin

		httpEffectiveNames []string // 存放有效的http插件名称
		rpcEffectiveNames  []string // 存放有效的rpc插件名称
	)

	// 配置插件（仅在启用时注册）
	if cSite.Status {
		for name, p := range cSite.Plugin.HTTP {
			if !p.Status {
				continue
			}
			if p.Addr == "" {
				T.errorf("server: 名称 %s 的HTTP插件配 Addr 字段不可以为空", name)
				continue
			}

			// 已发布的 http.Transport 可能正在被请求使用，不能原地修改其配置。
			httpC := new(vweb.PluginHTTPClient)
			if err := p.ConfigPluginHTTPClient(httpC); err != nil {
				T.errorf("server: 名称 %s 的HTTP插件配置错误: %s\n", name, err)
				continue
			}

			old, _ := plugin.http.Swap(name, httpC)
			httpEffectiveNames = append(httpEffectiveNames, name)
			if previous, ok := old.(*vweb.PluginHTTPClient); ok && previous != nil && previous.Tr != nil {
				previous.Tr.CloseIdleConnections()
			}
		}

		for name, p := range cSite.Plugin.RPC {
			if !p.Status {
				continue
			}
			if p.Addr == "" {
				T.errorf("server: 名称 %s 的RPC插件配 Addr 字段不可以为空\n", name)
				continue
			}

			if prev, ok := plugin.rpcConf.Load(name); ok && reflect.DeepEqual(prev, p) {
				if inf, ok := plugin.rpc.Load(name); ok {
					if c, ok := inf.(*vweb.PluginRPCClient); ok && c != nil {
						rpcEffectiveNames = append(rpcEffectiveNames, name)
						continue
					}
				}
			}

			rpcC := new(vweb.PluginRPCClient)
			if err := p.ConfigPluginRPCClient(rpcC); err != nil {
				T.errorf("server: 名称 %s 的RPC插件配置错误: %s", name, err)
				continue
			}
			old, loaded := plugin.rpc.Swap(name, rpcC)
			plugin.rpcConf.Store(name, p)
			rpcEffectiveNames = append(rpcEffectiveNames, name)
			if loaded {
				if c, ok := old.(*vweb.PluginRPCClient); ok && c != nil && c != rpcC && c.ConnPool != nil {
					c.ConnPool.Close()
				}
			}
		}
	} // ← 关闭 if cSite.Status（关键：必须在清理代码之前）

	// 清理：删除所有不在有效列表中的插件（站点禁用时也要执行，因此放在 if 外）
	httpSet := make(map[string]struct{}, len(httpEffectiveNames))
	for _, n := range httpEffectiveNames {
		httpSet[n] = struct{}{}
	}
	rpcSet := make(map[string]struct{}, len(rpcEffectiveNames))
	for _, n := range rpcEffectiveNames {
		rpcSet[n] = struct{}{}
	}
	plugin.http.Range(func(name, client any) bool {
		if _, ok := httpSet[name.(string)]; !ok {
			plugin.http.Delete(name)
			if c, ok := client.(*vweb.PluginHTTPClient); ok && c != nil && c.Tr != nil {
				c.Tr.CloseIdleConnections()
			}
		}
		return true
	})
	plugin.rpc.Range(func(name, client any) bool {
		if _, ok := rpcSet[name.(string)]; !ok {
			plugin.rpc.Delete(name)
			plugin.rpcConf.Delete(name)
			if c, ok := client.(*vweb.PluginRPCClient); ok && c != nil && c.ConnPool != nil {
				c.ConnPool.Close()
			}
		}
		return true
	})
} // ← 关闭函数体（原代码缺少这个）

// compileForwards 在配置加载时编译转发规则。
func (T *Group) compileForwards(cSite *config.Site) map[string][]compiledForward {
	if cSite == nil || len(cSite.Forward) == 0 {
		return nil
	}
	out := make(map[string][]compiledForward, len(cSite.Forward))
	for host, fcs := range cSite.Forward {
		rules := make([]compiledForward, 0, len(fcs.List))
		for i := range fcs.List {
			fc := fcs.List[i]
			if !fc.Status {
				continue
			}
			// RedirectCode 原样传给 http.Redirect。小于 100 或大于 999 时
			// WriteHeader 会 panic；200 等非 3xx 值不会真正跳转。只接受 300–399。
			if fc.RedirectCode != 0 && (fc.RedirectCode < 300 || fc.RedirectCode > 399) {
				T.errorf("server: host(%s) 转发规则 RedirectCode=%d 无效，仅支持 300-399，已忽略该规则\n", host, fc.RedirectCode)
				continue
			}
			fw := vweb.Forward{
				Path:        fc.Path,
				ExcludePath: fc.ExcludePath,
				RePath:      fc.RePath,
			}
			rewriter, err := fw.Compile()
			if err != nil {
				T.errorf("server: host(%s) 进行重写URL规则编译发生错误：%s\n", host, err.Error())
				continue
			}
			rules = append(rules, compiledForward{
				rewriter:     rewriter,
				redirectCode: fc.RedirectCode,
				end:          fc.End,
			})
		}
		if len(rules) > 0 {
			out[host] = rules
		}
	}
	return out
}

// 更新站点池或增加
//
//	cSite config.ConfigSite     配置
func (T *Group) updateSitePoolAdd(cSite config.Site) {
	site := T.sitePool.NewSite(cSite.Identity)

	site.Sessions().Configure(func(ss *vweb.Sessions) {
		expired := 20 * time.Minute
		if cSite.Session.Expired > 0 {
			expired = seconds(cSite.Session.Expired)
		}
		ss.Expired = expired
		ss.Name = cSite.Session.Name
		if ss.Name == "" {
			ss.Name = "VID"
		}
		ss.Size = cSite.Session.Size
		if ss.Size <= 0 {
			ss.Size = 64 // 与 vweb.SitePool.NewSite 的默认值一致
		}
		ss.Salt = cSite.Session.Salt
	})

	se := getSiteExtend(site)
	if site.RootDir == nil {
		site.RootDir = func(upath string) string {
			conf, _, _ := se.snapshot()
			if conf == nil {
				return "."
			}
			return conf.Directory.RootDir(upath)
		}
	}

	oldConf, _, _ := se.snapshot()

	// 配置和转发表一起发布，请求侧不会读到半新半旧。
	se.publish(&cSite, T.compileForwards(&cSite))

	if oldConf != nil && (!reflect.DeepEqual(oldConf.Directory, cSite.Directory) || !reflect.DeepEqual(oldConf.Dynamic, cSite.Dynamic)) {
		closeDynamicCache(se.dynamicCache)
	}

	// 防止配置发布前有请求过来，导致旧站点在这个窗口内返回 500
	for _, host := range cSite.Host {
		T.siteMan.Add(host, site)
	}
}

// 更新站点池删除, 过滤并删除无效的站点池。
//
// siteEffectiveHosts []string		现有有效的host
func (T *Group) updateSitePoolDel(siteEffectiveIdent, siteEffectiveHosts []string, wg *sync.WaitGroup) {
	// 有效 host/identity 转 set，删除扫描从 O(n*m) 降为 O(n)。
	hostSet := make(map[string]struct{}, len(siteEffectiveHosts))
	for _, h := range siteEffectiveHosts {
		hostSet[h] = struct{}{}
	}
	identSet := make(map[string]struct{}, len(siteEffectiveIdent))
	for _, id := range siteEffectiveIdent {
		identSet[id] = struct{}{}
	}

	// 从网站管理中删除无用的网站
	T.siteMan.Range(func(host string, site *vweb.Site) bool {
		if _, ok := hostSet[host]; !ok {
			T.siteMan.Add(host, nil)
		}
		return true
	})

	// 让站点池中删除无用的网站
	T.sitePool.RangeSite(func(name string, site *vweb.Site) bool {
		if _, ok := identSet[name]; !ok {
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
				plugin.rpcConf.Delete(name)
				if c, ok := client.(*vweb.PluginRPCClient); ok && c != nil && c.ConnPool != nil {
					c.ConnPool.Close()
				}
				return true
			})
			closeDynamicCache(se.dynamicCache)

			// 关闭服务组时需要等待会话 Defer 执行完毕。
			if wg != nil {
				wg.Go(func() {
					var local sync.WaitGroup
					site.Sessions().InstantDeadAllInto(&local)
					local.Wait()
					site.Global().Reset()
					site.Extend.Reset()
				})
			} else {
				site.Sessions().InstantDeadAllInto(nil)
				site.Global().Reset()
				site.Extend.Reset()
			}
		}
		return true
	})
}

func (T *Group) updateConfigSites(conf config.Sites) error {
	seen := make(map[string]struct{}, len(conf.Site))
	for i := range conf.Site {
		id := conf.Site[i].Identity
		if id == "" {
			return fmt.Errorf("server: 配置中出现站点惟一名(Identity)为 \"\", 需要设定一个名称。")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("server: 站点惟一名(Identity)重复: %q", id)
		}
		seen[id] = struct{}{}
	}

	var (
		siteEffectiveIdent []string
		siteEffectiveHosts []string
	)

	for _, cSite := range conf.Site {
		if cSite.Status {
			cSite.Plugin.RPC = maps.Clone(cSite.Plugin.RPC)
			cSite.Plugin.HTTP = maps.Clone(cSite.Plugin.HTTP)
			cSite.Forward = maps.Clone(cSite.Forward)
			// 复制公共  Session 配置
			if cSite.Session.PublicName != "" && !conf.Public.ConfigSiteSession(&cSite.Session, nil) {
				T.errorf("server: %s 站点的私有Session与公共Session合并失败\n", cSite.Identity)
			}
			// 复制公共 Header 配置
			if cSite.Header.PublicName != "" && !conf.Public.ConfigSiteHeader(&cSite.Header, nil) {
				T.errorf("server: %s 站点的私有Header与公共Header合并失败\n", cSite.Identity)
			}

			// 复制公共插件 RPC 配置
			for name, pRPC := range cSite.Plugin.RPC {
				if pRPC.PublicName != "" {
					if conf.Public.Plugin.ConfigSitePluginRPC(&pRPC, nil) {
						cSite.Plugin.RPC[name] = pRPC
						continue
					}
					T.errorf("server: %s 站点的 Plugin RPC %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制公共插件 HTTP 配置
			for name, pHTTP := range cSite.Plugin.HTTP {
				if pHTTP.PublicName != "" {
					if conf.Public.Plugin.ConfigSitePluginHTTP(&pHTTP, nil) {
						cSite.Plugin.HTTP[name] = pHTTP
						continue
					}
					T.errorf("server: %s 站点的 Plugin HTTP %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制公共 Forward 配置
			for name, forward := range cSite.Forward {
				if forward.PublicName != "" {
					// List 的底层数组也与调用方共享，append 可能写进原数组。
					forward.List = slices.Clone(forward.List)
					if conf.Public.ConfigSiteForward(&forward, nil) {
						cSite.Forward[name] = forward
						continue
					}
					T.errorf("server: %s 站点的 Forward %s 合并失败\n", cSite.Identity, name)
				}
			}

			// 复制Dynamic的配置
			if cSite.Dynamic.PublicName != "" && !conf.Public.ConfigSiteDynamic(&cSite.Dynamic, nil) {
				T.errorf("server: %s 站点的私有Dynamic与公共Dynamic合并失败\n", cSite.Identity)
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
	T.updateSitePoolDel(siteEffectiveIdent, siteEffectiveHosts, nil)

	return nil
}

// prepareServer 用 conf 准备一个全新的、尚未监听的服务器。
// 重启时先确认新配置可用，再停止旧服务。
func (T *Group) prepareServer(laddr string, conf config.Listen) (*Server, error) {
	srv := &Server{Addr: laddr}
	if err := srv.ConfigConn(&conf.CC); err != nil {
		return nil, err
	}
	if err := srv.ConfigServer(&conf.CS); err != nil {
		return nil, err
	}
	if err := T.defaultServerConfig(srv); err != nil {
		return nil, err
	}
	return srv, nil
}

// 用同步 net.Listen + Server.Serve 彻底消除 TOCTOU：
// - listenStart/restart 在调用方 goroutine 内拿到 listener，失败可同步返回；
// - 不再需要单独的 probe，且 restart 与 listenStart 行为一致。
func (T *Group) serveWith(laddr string, srv *Server, l net.Listener) {
	defer func() {
		if rec := recover(); rec != nil {
			T.errorf("server: ip(%s) panic: %v\n", laddr, rec)
			_ = srv.Close()
		}
		// 确保 serveWith 是 listener 的最后持有者；
		// Server.Serve 在 prepareServeLocked 失败时不会关闭传入的 l，
		// 不兜底关闭会泄漏文件描述符和端口。
		_ = l.Close()
		if inf, ok := T.srvMan.GetHas(laddr); ok {
			if cur, ok := inf.(*Server); ok && cur == srv {
				T.srvMan.Del(laddr)
			}
		}
	}()

	err := srv.Serve(l)
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		T.errorf("server: ip(%s), %s\n", laddr, err)
	}
}

func (T *Group) listenStop(laddr string) (err error) {
	inf, ok := T.srvMan.GetHas(laddr)
	if !ok {
		return nil
	}

	srv, ok := inf.(*Server)
	if !ok || srv == nil {
		T.srvMan.Del(laddr)
		return nil
	}

	srv.mu.Lock()
	graceful := srv.Server != nil && srv.cServer != nil && srv.cServer.ShutdownConn
	srv.mu.Unlock()

	if graceful {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			_ = srv.Close()
			T.srvMan.Del(laddr)
			return err
		}
		T.srvMan.Del(laddr)
		return nil
	}
	err = srv.Close()
	T.srvMan.Del(laddr)
	return err
}

// bindAndServe 用已准备好的 srv 绑定端口并启动服务 goroutine。
// 调用方必须保证端口已经空闲（restart 中需先 listenStop）。
func (T *Group) bindAndServe(laddr string, srv *Server) error {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return err
	}
	T.srvMan.Set(laddr, srv)
	go T.serveWith(laddr, srv, l)
	return nil
}

// listenStart 启动一个全新的监听（假定 srvMan 中无登记、端口空闲）。
func (T *Group) listenStart(laddr string, conf config.Listen) error {
	srv, err := T.prepareServer(laddr, conf)
	if err != nil {
		return err
	}
	return T.bindAndServe(laddr, srv)
}

// restartServer 用新配置替换一个正在运行的监听。
// 顺序约束：
//  1. 先 prepare（纯计算）—— 新配置无效则不动旧服务；
//  2. 再 listenStop       —— 释放端口；
//  3. 最后 bindAndServe   —— 绑定新 listener 并启动。
func (T *Group) restartServer(laddr string, conf config.Listen) {
	srv, err := T.prepareServer(laddr, conf)
	if err != nil {
		T.errorf("server: %s 新配置无效，继续使用旧配置：%v\n", laddr, err)
		return
	}
	if err := T.listenStop(laddr); err != nil {
		// 停止旧服务失败也继续尝试启动新服务：旧 listener 大概率已处于异常状态，
		// 若 bindAndServe 因端口占用失败，下面会有明确日志。
		T.errorf("server: %s 停止旧服务时出错（继续重启）：%v\n", laddr, err)
	}
	if err := T.bindAndServe(laddr, srv); err != nil {
		T.errorf("server: %s 新配置无法监听端口，服务暂时不可用：%v\n", laddr, err)
	}
}

// updateConfigServers 根据配置同步监听器。
//
// 每个地址有四种归宿：
//  1. 已禁用或本就不在 listens 中 → listenStop（上面已统一处理 removed）；
//  2. 地址不在 srvMan         → listenStart（全新启动，端口空闲）；
//  3. 地址在 srvMan，配置变更   → restartServer（prepare → stop → bind）；
//  4. 地址在 srvMan，配置未变   → 不做任何事（保持现有连接，不因重载断连）。
func (T *Group) updateConfigServers(conf config.Servers) {
	exclude := func(name string, _ reflect.Value, src reflect.Value) bool {
		if name == "TLS" {
			if !src.IsValid() {
				return true
			}
			if src.Kind() == reflect.Pointer {
				return src.IsNil()
			}
			return src.IsZero()
		}
		return false
	}

	listens := maps.Clone(conf.Listen)
	for laddr, cl := range listens {
		if !cl.Status {
			continue
		}
		if cl.CC.PublicName != "" && !conf.Public.ConfigConn(&cl.CC, nil) {
			T.errorf("server: %s 地址的私有CC与公共CC合并失败\n", laddr)
		}
		if cl.CS.PublicName != "" && !conf.Public.ConfigServer(&cl.CS, exclude) {
			T.errorf("server: %s 地址的私有CS与公共CS合并失败\n", laddr)
		}
		listens[laddr] = cl
	}

	// vmap.Map 的 Range 若持有内部锁就会死锁。先收集，再在 Range 外停止。
	var removed []string
	T.srvMan.Range(func(key, _ any) bool {
		if ip, ok := key.(string); ok {
			if _, ok := listens[ip]; !ok {
				removed = append(removed, ip)
			}
		}
		return true
	})
	for _, ip := range removed {
		if err := T.listenStop(ip); err != nil {
			T.errorf("%v", err)
		}
	}

	for laddr, cl := range listens {
		// 已禁用 → 停止监听
		if !cl.Status {
			if err := T.listenStop(laddr); err != nil {
				T.errorf("%v", err)
			}
			continue
		}

		inf, running := T.srvMan.GetHas(laddr)
		if !running {
			// 尚未开启监听 → 全新启动
			if err := T.listenStart(laddr, cl); err != nil {
				T.errorf("server: %s 启动监听失败：%v\n", laddr, err)
			}
			continue
		}

		srv, ok := inf.(*Server)
		if !ok || srv == nil {
			// 内部先 prepare、再 listenStop 清掉脏登记、最后 bind。
			// 原实现直接 listenStart：如果脏登记背后还残留 listener，
			// listenStart 的 net.Listen 会撞上 EADDRINUSE。
			T.restartServer(laddr, cl)
			continue
		}

		// 服务器/连接配置变更才重启；配置未变则保持现有连接（不因重载断连）
		if srvConfigChanged(srv, cl) {
			T.restartServer(laddr, cl)
		}
	}
}

// srvConfigChanged 判断现有 Server 快照与新配置是否变化。
func srvConfigChanged(srv *Server, cl config.Listen) bool {
	cServer, cConn := srv.configSnapshot()
	if cServer != nil {
		if !reflect.DeepEqual(*cServer, cl.CS) {
			return true
		}
	} else if !reflect.DeepEqual(config.Server{}, cl.CS) {
		return true
	}
	if cConn != nil {
		return !reflect.DeepEqual(*cConn, cl.CC)
	}
	return !reflect.DeepEqual(config.Conn{}, cl.CC)
}

// LoadConfigFile 挂载本地配置文件。
//
//	p string        文件路径
//	ok bool			true配置文件被修改过, false没有变动
//	err error       错误
func (T *Group) LoadConfigFile(p string) (ok bool, err error) {
	T.configMu.Lock()
	defer T.configMu.Unlock()

	fileInfo, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	if fileInfo.ModTime().Equal(T.configFileModTime) {
		return false, nil
	}

	var conf config.Config
	if err := conf.ParseFiles(p); err != nil {
		return false, err
	}

	if err := T.updateConfigLocked(&conf); err != nil {
		return false, err
	}

	// 不在解析之前或更新调用之前提交修改时间。
	T.configFileModTime = fileInfo.ModTime()
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
	return T.updateConfigLocked(conf)
}

// updateConfigLocked 的调用方必须持有 configMu。
func (T *Group) updateConfigLocked(conf *config.Config) error {
	T.config = conf
	if !T.run.Load() {
		return nil
	}
	var firstErr error
	if err := T.updateConfigSites(conf.Sites); err != nil {
		firstErr = err
		T.errorf("server: 更新站点配置失败：%v", err)
	}
	T.updateConfigServers(conf.Servers)
	return firstErr
}

// Start 启动服务集群。
func (T *Group) Start() error {
	T.configMu.Lock()

	if T.run.Load() {
		T.configMu.Unlock()
		return fmt.Errorf("server: 服务组已经开启。")
	}

	T.closing.Store(false)
	T.initGroupResources()
	exit := T.exit

	T.run.Store(true)
	if T.config != nil {
		if err := T.updateConfigLocked(T.config); err != nil {
			T.run.Store(false)
			// updateConfigServers 可能已经启动监听； Start 失败必须回收，否则端口仍被占用。
			T.closeServersLocked()
			if T.exit != nil {
				close(T.exit)
				T.exit = nil
			}
			T.configMu.Unlock()
			return err
		}
	}

	T.configMu.Unlock()

	<-exit
	return nil
}

// closeServersLocked 关闭所有服务器。仅用于 Start 失败/Close 的监听回收。
func (T *Group) closeServersLocked() {
	var srvs []*Server
	T.srvMan.Range(func(_, v any) bool {
		if srv, ok := v.(*Server); ok && srv != nil {
			srvs = append(srvs, srv)
		}
		return true
	})
	T.srvMan.Reset()
	for _, srv := range srvs {
		_ = srv.Close()
	}
}

// initGroupResources 幂等初始化 Group 运行期必需的资源。
func (T *Group) initGroupResources() {
	if T.exit == nil {
		T.exit = make(chan bool)
	}
	if T.sitePool == nil {
		T.sitePool = vweb.NewSitePool()
	}
	if T.siteMan == nil {
		T.siteMan = new(vweb.SiteMan)
	}
	if T.route == nil {
		T.route = new(vweb.Route)
	}
	T.route.SetSiteMan(T.siteMan)
	T.route.HandlerError = http.HandlerFunc(T.serveHTTP)
}

// waitTimeout 等待 wg 完成，最多等待 d。返回 true 表示在超时前完成。
func waitTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// Close 关闭服务集群。
func (T *Group) Close() error {
	T.configMu.Lock()
	defer T.configMu.Unlock()

	T.closing.Store(true)
	wasRunning := T.run.Swap(false)

	// 关闭监听
	var srvs []*Server
	T.srvMan.Range(func(_, v any) bool {
		if srv, ok := v.(*Server); ok && srv != nil {
			srvs = append(srvs, srv)
		}
		return true
	})
	T.srvMan.Reset()
	for _, srv := range srvs {
		srv.mu.Lock()
		graceful := srv.Server != nil && srv.cServer != nil && srv.cServer.ShutdownConn
		srv.mu.Unlock()
		// ShutdownConn=true 时先优雅关闭，超时或失败再降级为硬 Close。
		if graceful {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := srv.Shutdown(ctx); err != nil {
				srv.Close()
			}
			cancel()
			continue
		}
		srv.Close()
	}

	// 无论是否 running 都执行资源清理（每步都做 nil 判断与幂等）
	if T.sitePool != nil {
		var wg sync.WaitGroup
		T.updateSitePoolDel(nil, nil, &wg)
		if !waitTimeout(&wg, sessionFreeWaitTimeout) {
			T.errorf("server: 等待会话 Defer 超过 %s，放弃等待\n", sessionFreeWaitTimeout)
		}
		T.sitePool.Close()
		T.sitePool = nil
	}
	T.siteMan = nil

	if T.exit != nil {
		close(T.exit)
		T.exit = nil
	}

	if !wasRunning {
		// 保留"已经关闭过"的可观测语义，但资源已释放
		return fmt.Errorf("server: 服务组已经关闭！")
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
		if ep, ok := errorPage[c]; ok {
			p, err := safeJoinRoot(rootDir, ep)
			if err != nil {
				http.Error(w, e, code)
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				// 回退到默认错误信息并照常写出响应。
				http.Error(w, e, code)
				return err
			}
			// 按文件扩展名设置 Content-Type，原样写出文件内容。
			ct := mime.TypeByExtension(filepath.Ext(p))
			if ct == "" {
				ct = "text/html; charset=utf-8"
			}
			h := w.Header()
			h.Del("Content-Length")
			h.Set("Content-Type", ct)
			h.Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(code)
			_, err = w.Write(b)
			return err
		}
	}
	http.Error(w, e, code)
	return nil
}

// safeJoinRoot 把错误页限制在站点根目录下。
// filepath.Join 遇到绝对路径会丢掉 root，从而读到站点外的文件。
func safeJoinRoot(rootDir, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("error page path escapes root")
	}
	if rootDir == "" {
		rootDir = "."
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return "", err
	}
	p := filepath.Join(rootAbs, filepath.Clean(rel))
	relOut, err := filepath.Rel(rootAbs, p)
	if err != nil {
		return "", err
	}
	if relOut == ".." || strings.HasPrefix(relOut, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("error page path escapes root")
	}

	// 上面只是字符串检查。根目录内的符号链接可以指向站点外文件，
	// 必须解析真实路径后再次校验。根目录本身也可能是符号链接（如 macOS 的 /var），
	// 两边都解析后再比较。
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		rootReal = rootAbs
	}
	realPath, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	relReal, err := filepath.Rel(rootReal, realPath)
	if err != nil || relReal == ".." || strings.HasPrefix(relReal, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("error page path escapes root")
	}
	return realPath, nil
}

// httpTypeByExtension 文件类型扩展, 如果自定义列表不存在扩展类型, 则使用系统默认扩展类型。如果自定义列表扩展类型是空""的类型, 说明是用户设置拒绝访问该类型。
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
