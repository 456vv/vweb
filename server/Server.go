package server

import (
	"fmt"
    "reflect"
    "net"
    "net/http"
    "crypto/tls"
    "log"
    "time"
    "io/ioutil"
    "bytes"
    "crypto/x509"
    "path"
    "path/filepath"
    "os"
    "strconv"
    "mime"
    "github.com/456vv/vmap/v2"
    "github.com/456vv/verror"
    "github.com/456vv/vweb/v2"
    "context"
    "sync"
	"sync/atomic"
)

//默认4K
var defaultDataBufioSize int64 = 4096

var Version	string = "Server/2.0.x"

//响应完成设置
type atomicBool int32
func (T *atomicBool) isTrue() bool 	{ return atomic.LoadInt32((*int32)(T)) != 0 }
func (T *atomicBool) isFalse() bool	{ return atomic.LoadInt32((*int32)(T)) != 1 }
func (T *atomicBool) setTrue() bool	{ return !atomic.CompareAndSwapInt32((*int32)(T), 0, 1)}
func (T *atomicBool) setFalse() bool{ return !atomic.CompareAndSwapInt32((*int32)(T), 1, 0)}

type siteExtend struct {
	config 			*ConfigSite
	plugin 			*plugin
	dynamicCache	sync.Map		// 缓存动态文件对象
}

//Server 服务器,使用在 ServerGroup.srvMan 字段。
type Server struct {
    *http.Server                                                                            // http服务器
	Addr				string
    tlsconf				*tls.Config
	l					listener
	status				atomicBool
	cs					*ConfigServer
	cc					*ConfigConn
}

func (T *Server) init(){
	if T.Server == nil {
		T.Server = new(http.Server)
		T.Server.BaseContext = func(l net.Listener) context.Context {
			return context.WithValue(context.Background(), "Listener", l)
		}
		T.Server.ConnContext = func(ctx context.Context, rwc net.Conn) context.Context {
			return context.WithValue(ctx, "Conn", rwc)
		}
	}
}

func (T *Server) Serve(l net.Listener) error {
	T.init()
	T.Addr 	= l.Addr().String()
	T.l.TCPListener = l.(*net.TCPListener)
	return T.Server.Serve(&T.l)
}

func (T *Server) ListenAndServe() error {
	T.init()
	
	if T.Addr == "" {
		T.Addr = ":http"
	}
	l, err := net.Listen("tcp", T.Addr)
	if err != nil {
		return err
	}
	return T.Serve(l)
}

func (T *Server) ConfigConn(cc *ConfigConn) error {
	if cc == nil {
		return verror.TrackError("server: *ConfigConn不可以为nil")
	}
	if T.cc == nil {
		T.cc=&ConfigConn{}
	}
	*T.cc = *cc
	T.l.cc = T.cc
	return nil

}

func (T *Server) ConfigServer(cs *ConfigServer) error {
	if cs == nil {
		return verror.TrackError("server: *ConfigServer不可以为nil")
	}
	if T.cs == nil {
		T.cs=&ConfigServer{}
	}
	*T.cs = *cs
	
	T.init()
	
	//服务器配置
    T.Server.ReadTimeout      	= time.Duration(cs.ReadTimeout) * time.Millisecond
    T.Server.WriteTimeout     	= time.Duration(cs.WriteTimeout) * time.Millisecond
    T.Server.ReadHeaderTimeout	= time.Duration(cs.ReadHeaderTimeout) * time.Millisecond
    T.Server.IdleTimeout		= time.Duration(cs.IdleTimeout) * time.Millisecond
    T.Server.MaxHeaderBytes   	= cs.MaxHeaderBytes
    T.Server.SetKeepAlivesEnabled(cs.KeepAlivesEnabled)
    
    //TLS设置
   	if cs.TLS != nil {
	    if T.l.tlsconf == nil {
	    	T.l.tlsconf = new(tls.Config)
	    }
	    err := configTLSFile(T.l.tlsconf, cs.TLS)
    	if err != nil {
    		return err
    	}
   	}else{
   		T.l.tlsconf = nil
   	}
   	return nil
}

//TLS文件配置
func configTLSFile(c *tls.Config, conf *ConfigServerTLS) error {
    c.NextProtos                	= conf.NextProtos
    c.PreferServerCipherSuites  	= conf.PreferServerCipherSuites
    c.SessionTicketsDisabled    	= conf.SessionTicketsDisabled
    c.MinVersion                	= conf.MinVersion
    c.MaxVersion                	= conf.MaxVersion
   	c.SessionTicketKey				= conf.SessionTicketKey
   	c.DynamicRecordSizingDisabled	= conf.DynamicRecordSizingDisabled

	if len(conf.CipherSuites) >0 {
    	copy(c.CipherSuites, conf.CipherSuites)
	}else{
		//内部判断并使用默认的密码套件
    	c.CipherSuites			= nil
	}

    if len(conf.SetSessionTicketKeys) > 0 {
    	c.SetSessionTicketKeys(conf.SetSessionTicketKeys)
    }

	var errStr string
	//支持双向证书
	if len(conf.ClientCAs) != 0 {
		if c.ClientCAs == nil {
			if certPool, err := x509.SystemCertPool(); err == nil {
				//系统证书
				c.ClientCAs = certPool
			}else{
				//如果读取系统根证书失败，则创建新的证书
				c.ClientCAs = x509.NewCertPool()
			}
		}
		var errClientCA string
		for _, path := range conf.ClientCAs {
			//打开文件
			caData, err := ioutil.ReadFile(path)
			if err != nil {
        		errClientCA = fmt.Sprintf("%s%s: %s\n", errClientCA, path, err.Error())
				continue
			}
			
			ext := filepath.Ext(path)
			if ext == ".cert" {
				certificates, err := x509.ParseCertificates(caData)
				if err != nil {
	        		errClientCA = fmt.Sprintf("%s%s: %s\n", errClientCA, path, err.Error())
	        		continue
				}
				for _, cert := range certificates {
					c.ClientCAs.AddCert(cert)
				}
			}else if ext == ".pem" {
				if !c.ClientCAs.AppendCertsFromPEM(caData) {
	        		errClientCA = fmt.Sprintf("%s%s: %s\n", errClientCA, path, "not is a valid PEM format")
	        		continue
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
        	//日志
        	errServerCert = fmt.Sprintf("%s{CertFile:%q, KeyFile:%q}: %s\n", errServerCert, file.CertFile, file.KeyFile, err.Error())
        	continue
        }
         c.Certificates = append(c.Certificates, cert)
    }
    if errServerCert != "" {
		errStr = errStr + "解析服务端证书发生错误（CS.TLS.RootCAs）: \n" + errServerCert
    }
    
    //多证书
    c.BuildNameToCertificate()
    if errStr != "" {
    	return verror.TrackErrorf("server: %s", errStr)
    }
    return nil
}

type ServerGroup struct {
    ErrorLog			*log.Logger 							// 错误日志文件
    DynamicTemplate		map[string]vweb.DynamicTemplateFunc		// 支持更多动态
    
    // srvMan 存储值类型是 *Server，读取时需要转换类型
    srvMan              vmap.Map            // map[ip:port]*Server	服务器集
    sitePool			*vweb.SitePool		// 站点的池
    siteMan				vweb.SiteMan		// 站点管理
    exit                chan bool			// 退出
	
	run					atomicBool			// 服务器启动了
	
    // 用于 .UpdateConfigFile 方法
    backConfigDate      []byte              // 备份配置数据。如果是相同数据，则不更新
    config				*Config				// 配置
    
}

func NewServerGroup() *ServerGroup {
	return &ServerGroup{
        exit            : make(chan bool),
        ErrorLog		: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile),
    }
}

//增加一个服务器
//	laddr string	监听地址
//	srv *Server		服务器，如果为nil，则删除已存在的记录
func (T *ServerGroup) SetServer(laddr string, srv *Server) error {
	if srv == nil {
		T.srvMan.Del(laddr)
		return nil
	}
    if srv.Handler == nil {
    	srv.Handler = http.HandlerFunc(T.serveHTTP)
	}
	T.srvMan.Set(laddr, srv)
	return nil
}

//读取一个服务器
//	laddr string	监听地址
//	*Server			服务器
//	bool			如果存在服务器，返回true。否则返回false
func (T *ServerGroup) GetServer(laddr string) (*Server, bool) {
	inf, ok := T.srvMan.GetHas(laddr)
	if !ok {
		return nil ,ok
	}
	return inf.(*Server), true
}

//设置一个站点池，如果没有设置，则使用内置全局默认站点池。
//站点池主要是管理会话的过期。
//	pool *vweb.SitePool	池
//	error				错误
func (T *ServerGroup) SetSitePool(pool *vweb.SitePool) error {
	T.sitePool 		= pool
	return nil
}

//serveHTTP 处理HTTP
//	rw http.ResponseWriter	响应
//	r *http.Request			请求
func (T *ServerGroup) serveHTTP(rw http.ResponseWriter, r *http.Request){
	
    //** 检查Host是否存在
    site, ok := T.siteMan.Get(r.Host)
    if !ok {
        //如果在站点集中没有找到存在的Host，则关闭连接。
		hj, ok := rw.(http.Hijacker)
		if !ok {
            //500 服务器遇到了意料不到的情况，不能完成客户的请求。
			http.Error(rw, "Not supported Hijacker", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
            //500 服务器遇到了意料不到的情况，不能完成客户的请求。
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		//直接关闭连接
		defer conn.Close()
        return
    }

    //** 配置
    se, ok := site.Extend.(*siteExtend)
    if !ok || se.config == nil {
        //500 服务器遇到了意料不到的情况，不能完成客户的请求。
        http.Error(rw, "The configuration is nil\n", http.StatusInternalServerError)
        return
    }
    config := se.config
    
    //** 转发URL
    forward := config.Forward
    urlPath	:= r.URL.Path
    if  len(forward) != 0 {
        var forwardC ConfigSiteForward
        derogatoryDomain(r.Host, func(h string) (ok bool){
        	forwardC, ok = forward[h]
            return
        })

        for _, fc := range forwardC.List {
        	if !fc.Status {
        		//跳过禁止的
        		continue
        	}
        	rpath, rewried, err := fc.Rewrite(urlPath)
        	if err != nil {
        		T.ErrorLog.Printf("server: host(%s) 进行重写URL规则发发生错误：%s\n", r.Host, err.Error())
        		continue
        	}
        	if rewried {
            	if fc.RedirectCode != 0 {
            		//重定向,并退出
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
    var(
        rootPath    string = "."
        pagePath    string = urlPath
        err			error
    )
    
    if site.RootDir != nil {
    	rootPath = site.RootDir(pagePath)
    }
    
    if rootPath == "." {
    	//404 无法找到指定位置的资源。这也是一个常用的应答。
        http.Error(rw, "Web root directory is not set?", http.StatusNotFound)
        return
    }
    
    _, pagePath, err = vweb.PagePath(rootPath, pagePath, config.IndexFile)
    if err != nil {
    	//404 无法找到指定位置的资源。这也是一个常用的应答。
        httpError(rw, rootPath, config.ErrorPage, err.Error(), http.StatusNotFound)
        return
    }

    //** 文件后缀支持
    var(
        fileExt         = path.Ext(pagePath)
        header          = config.Header
        contentType     = httpTypeByExtension(fileExt, header.MIME)
    )

    if contentType == "" {
		contentType = "application/octet-stream"
    	//403 资源不可用。服务器解析客户的请求，但拒绝处理它。
        //httpError(rw, config.ErrorPage, "This file suffix type MIME system does not recognize!", http.StatusForbidden)
        //return
    }

	//** 文件固定标头准备
	var (
		buffSize 	= config.Property.BuffSize
	    wh 			= rw.Header()
	    th			ConfigSiteHeaderType
	)
	wh.Set("Content-Type", contentType)
	wh.Set("Server", Version)

	//如果配置默认为0，则使用内置默认缓冲块大小
	if buffSize == 0 {
		buffSize = defaultDataBufioSize
	}

    //** 文件动态静态分离
    if strSliceContains(config.DynamicExt, fileExt) {
		//动态页面
		
	    //读取指定后缀类型的标头内容
	    if header.Dynamic != nil {
	        if h, ok := header.Dynamic[fileExt]; ok {
	        	th = h
	        }else if h, ok := header.Dynamic["*"]; ok {
	        	th = h
			}
		    for k, v := range th.Header {
		    	for _, v1 := range v {
		        	wh.Add(k, v1)
		    	}
		    }
	    }
        
		//处理动态格式
        var handlerDynamic *vweb.ServerHandlerDynamic
        if inf, ok := se.dynamicCache.Load(pagePath); ok && config.DynamicCache {
        	handlerDynamic = inf.(*vweb.ServerHandlerDynamic)
        }else{
        	handlerDynamic = &vweb.ServerHandlerDynamic{
        		PagePath: pagePath,
        		Plus: T.DynamicTemplate,
        	}
        	if config.DynamicCache {
        		se.dynamicCache.Store(pagePath, handlerDynamic)
        	}
        }
        handlerDynamic.RootPath = rootPath
        handlerDynamic.BuffSize = buffSize
       	handlerDynamic.Site 	= site
        handlerDynamic.Context 	= context.WithValue(r.Context(), "Plugin", (Pluginer)(se.plugin))
        handlerDynamic.ServeHTTP(rw, r)
   }else{
    	//静态页面
    	
	    //读取指定后缀类型的标头内容
	    if header.Static != nil {
	        if h, ok := header.Static[fileExt]; ok {
	        	th = h
	        }else if h, ok := header.Static["*"]; ok {
	        	th = h
			}
		    for k, v := range th.Header {
		    	for _, v1 := range v {
		        	wh.Add(k, v1)
		    	}
		    }
	    }
	    
        shs := &vweb.ServerHandlerStatic{
            RootPath        : rootPath,
            PagePath        : pagePath,
		    PageExpired		: th.PageExpired,
			BuffSize		: buffSize,
        }
        shs.ServeHTTP(rw, r)
    }
}


//更新插件
func (T *ServerGroup) updatePluginConn(cSite ConfigSite){
	site := T.sitePool.NewSite(cSite.Identity)
	if site.Extend == nil {
   	   site.Extend = &siteExtend{}
	}
	se := site.Extend.(*siteExtend)
	if se.plugin == nil {
		se.plugin = &plugin{}
	}
	
	var (
		httpNames	[]string
		rpcNames	[]string
	)
	//配置插件
	if cSite.Status {
		
		for name, p := range cSite.Plugin.HTTP {
			if !p.Status {
				continue
			}
			httpNames = append(httpNames, name)
			
			if p.Addr == "" {
				T.ErrorLog.Println(fmt.Sprintf("server: 名称 %s 的HTTP插件配 Addr 字段不可以为空", name))
				continue
			}
			
			var httpC *vweb.PluginHTTPClient
			inf, ok := se.plugin.http.Load(name)
			if ok {
				httpC = inf.(*vweb.PluginHTTPClient)
			}
			if httpC == nil {
				httpC = &vweb.PluginHTTPClient{}
			}
			
			if err := p.ConfigPluginHTTPClient(httpC); err != nil {
				T.ErrorLog.Printf("server: 名称 %s 的HTTP插件配错误，%s", name, err.Error())
				continue
			}
			se.plugin.http.Store(name, httpC)
		}
		for name, p := range cSite.Plugin.RPC {
			if !p.Status {
				continue
			}
			rpcNames = append(rpcNames, name)
			
			if p.Addr == "" {
				T.ErrorLog.Println(fmt.Sprintf("server: 名称 %s 的RPC插件配 Addr 字段不可以为空", name))
				continue
			}
			
			var rpcC *vweb.PluginRPCClient
			inf, ok := se.plugin.rpc.Load(name)
			if ok {
				rpcC = inf.(*vweb.PluginRPCClient)
			}
			if rpcC == nil {
				rpcC = &vweb.PluginRPCClient{}
			}
			if err := p.ConfigPluginRPCClient(rpcC); err != nil {
				T.ErrorLog.Printf("server: 名称 %s 的HTTP插件配错误，%s", name, err.Error())
				continue
			}
			se.plugin.rpc.Store(name, rpcC)
		}
	}else{
		//清除动态文件缓存
		se.dynamicCache.Range(func(ninf, vinf interface{}) bool{
			se.dynamicCache.Delete(ninf)
			return true
		})
	}
	
	//关闭无效的插件
	se.plugin.http.Range(func(ninf, hinf interface{}) bool {
		if !strSliceContains(httpNames, ninf.(string)) {
			se.plugin.http.Delete(ninf)
			
			httpC := hinf.(*vweb.PluginHTTPClient)
			httpC.Tr.CloseIdleConnections()
		}
		return true
	})
	se.plugin.rpc.Range(func(ninf, rinf interface{}) bool {
		if !strSliceContains(rpcNames, ninf.(string)) {
			se.plugin.rpc.Delete(ninf)
			
			rpcC := rinf.(*vweb.PluginRPCClient)
			rpcC.ConnPool.Close()
		}
		return true
	})
}

//updateSitePoolAdd 更新站点池或增加
//	conf ConfigSite     配置
func (T *ServerGroup) updateSitePoolAdd(cSite ConfigSite) {
	
	site := T.sitePool.NewSite(cSite.Identity)
	for _, host := range cSite.Host {
		T.siteMan.Add(host, site)
	}

   	//设置Session
	vweb.CopyStruct(site.Sessions, &cSite.Session, func(name string, dsc, src reflect.Value) bool {
		return name == "Expired"
	})
	
	site.Sessions.Expired = time.Duration(cSite.Session.Expired) * time.Second
	site.RootDir = cSite.Directory.RootDir
	
	if site.Extend == nil {
	   site.Extend = &siteExtend{}
	}
	
	se := site.Extend.(*siteExtend)
	se.config = &cSite

}

//updateSitePoolDel 更新站点池删除，过滤并删除无效的站点池。
//	siteIdent []string      现有的站点列表
func (T *ServerGroup) updateSitePoolDel(siteIdent []string) {
    
    T.sitePool.RangeSite(func(name string, site *vweb.Site) bool{
    	if !strSliceContains(siteIdent, name) {
	    	//从池中删除
			T.sitePool.DelSite(name)
			
	    	//设置过期时间
	    	sec := time.Now().Unix()
	    	site.Sessions.Expired = time.Duration(^sec) * time.Second
	    	site.Sessions.ProcessDeadAll()
	    }
    	return true
    })
}

func (T *ServerGroup) updateConfigSites(conf *ConfigSites) error {
    var (
        siteIdent   []string
        siteHosts  	[]string
    )
    for _, cSite := range conf.Site {
        if cSite.Identity == "" {
            return verror.TrackErrorf("server: 配置中出现站点惟一名(Identity)为 \"\"，需要设定一个名称。")
        }
        
		if cSite.Status {
            
	    	//复制Session的配置
	    	if cSite.Session.PublicName != "" && !conf.Public.ConfigSiteSession(&cSite.Session, nil) {
            	T.ErrorLog.Printf("server: %s 站点的私有Session与公共Session合并失败\n", cSite.Identity)
	    	}
	    	
	    	//复制Header的配置
	    	merge := func(name string, dsc, src reflect.Value) bool{
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
			
			//复制Plugin的配置
	    	merge = func(name string, dsc, src reflect.Value) bool{
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
						cSite.Plugin.RPC[name]=pRPC
            			continue
	            	 }
            		T.ErrorLog.Printf("server: %s 站点的 Plugin RPC %s 合并失败\n", cSite.Identity, name)
            	}
            }
            for name, pHTTP := range cSite.Plugin.HTTP {
            	if pHTTP.PublicName != "" {
	            	 if conf.Public.Plugin.ConfigSitePluginHTTP(&pHTTP, merge) {
						cSite.Plugin.HTTP[name]=pHTTP
            			continue
	            	 }
            		T.ErrorLog.Printf("server: %s 站点的 Plugin HTTP %s 合并失败\n", cSite.Identity, name)
            	}
            }

            //复制Forward的配置
            for name, forward := range cSite.Forward {
            	if forward.PublicName != ""  {
            		if conf.Public.ConfigSiteForward(&forward, nil) {
            			cSite.Forward[name] = forward
            			continue
            		}
            		T.ErrorLog.Printf("server: %s 站点的 Forward %s 合并失败\n", cSite.Identity, name)
            	}
            }
            
            //复制Property的配置
            if cSite.Property.PublicName != "" && !conf.Public.ConfigSiteProperty(&cSite.Property, merge) {
            	T.ErrorLog.Printf("server: %s 站点的私有Property与公共Property合并失败\n", cSite.Identity)
            }
            
            //预选分配池，初始化站点
            T.updateSitePoolAdd(cSite)
            
            //集中名称
            siteIdent = append(siteIdent, cSite.Identity)

            //集中站点Host
            //可能有多个站点绑定了同一个Host，只有最后一个是有效的
            for _, host := range cSite.Host {
                siteHosts = append(siteHosts, host)
            }
        }
		
		//插件不关网站是否开启
		//网站不开启，否关闭插件
		T.updatePluginConn(cSite)
    }

    //更新网站
	T.siteMan.Range(func(host string, site *vweb.Site) bool {
		if !strSliceContains(siteHosts, host) {
			T.siteMan.Add(host, nil)
		}
		return true
	})
	
	//删除池中不存在的配置
    T.updateSitePoolDel(siteIdent)
    
	return nil

}

func (T *ServerGroup) newServer(laddr string) *Server {
    inf, ok := T.srvMan.GetHas(laddr)
    if ok {
    	return inf.(*Server)
    }
    srv := new(Server)
    srv.Addr = laddr
	return srv
}

//listenStart 启动监听端口
func (T *ServerGroup) listenStart(laddr string, conf ConfigListen) error {
    srv := T.newServer(laddr)
    err := srv.ConfigConn(&conf.CC)
    if err != nil {
    	return err
    }
    err = srv.ConfigServer(&conf.CS)
    if err != nil {
    	return err
    }
    if srv.Handler == nil {
    	srv.Handler = http.HandlerFunc(T.serveHTTP)
   }
   	go T.serve(srv)
    return nil
}

//listenStop 关闭监听
func (T *ServerGroup) listenStop(laddr string) (err error) {
   if inf, ok := T.srvMan.GetHas(laddr); ok {
        if srv, ok := inf.(*Server); ok {
		    if srv.Server != nil {
		    	if srv.cs != nil && srv.cs.ShutdownConn {
			    	//不要即时关闭正在下载的连接
			    	return srv.Server.Shutdown(context.Background())
		    	}else{
			    	return srv.Server.Close()
		    	}
		    }
        }
    }
    return nil
}

//监听决定，区分是开启还是关闭监听。
func (T *ServerGroup) updateConfigServers(conf *ConfigServers) {
	var err error

    //如果在新的IP例表中没有找到已经存在的开放监听端口IP，而停止监听此IP
    T.srvMan.Range(func(key, val interface{}) bool{
        ip := key.(string)
        if _, ok := conf.Listen[ip]; !ok {
            err = T.listenStop(ip)
            if err != nil {
            	T.ErrorLog.Println(err.Error())
            }
        }
        return true
    })

    //如果还没开启监听，则启动他
    for laddr, cl := range conf.Listen {
	    if cl.Status {
	    	//复制的配置
			if cl.CC.PublicName != "" && !conf.Public.ConfigConn(&cl.CC, nil) {
            	T.ErrorLog.Printf("server: %s 地址的私有CC与公共CC合并失败\n", laddr)
			}
			
			merge := func(name string, dsc, src reflect.Value) bool {
				switch name {
				case "TLS":
					return !src.Elem().IsValid()
				default:
				}
				return false
			}
			if cl.CS.PublicName != "" && !conf.Public.ConfigServer(&cl.CS, merge) {
            	T.ErrorLog.Printf("server: %s 地址的私有CS与公共CS合并失败\n", laddr)
			}
			
	        err = T.listenStart(laddr, cl)
	        if err != nil {
            	T.ErrorLog.Println(err.Error())
	        }
	    }else{
	    	err = T.listenStop(laddr)
	        if err != nil {
            	T.ErrorLog.Println(err.Error())
	        }
	    }
    }
}

//LoadConfigFile 挂载本地配置文件。
//	p string        文件路径
//	conf *Config	配置
//	ok bool			true配置文件被修改过，false没有变动
//	err error       错误
func (T *ServerGroup) LoadConfigFile(p string)  (conf *Config, ok bool, err error) {
    b, err := ioutil.ReadFile(p)
    if err != nil {
    	return
    }
    //判断文件是否有改动
    if bytes.Equal(b, T.backConfigDate) {
    	return T.config, false, nil
    }
    T.backConfigDate = b

    conf = new(Config)
    r := bytes.NewReader(b)
    //解析配置文件
    err = ConfigDataParse(conf, r)
    if err != nil {
    	return
    }
    //更新配置文件
	err = T.UpdateConfig(conf)
    if err != nil {
    	return
    }
    return conf, true, nil
}

//UpdateConfig 更新配置并把配置分配到各个地方。不检查改动，直接更新。更新配置需要调用 .Start 方法之后才生效。
//	conf *Config        配置
//	error               错误
func (T *ServerGroup) UpdateConfig(conf *Config) error {
	if conf == nil {
    	return verror.TrackErrorf("server: 配置为nil，无法更新。")
	}
	
	T.config = conf
	if T.run.isTrue() {
		//更新网站配置
		if err := T.updateConfigSites(&conf.Sites); err != nil {
			T.ErrorLog.Println(err.Error())
		}
		//更新服务器配置
		T.updateConfigServers(&conf.Servers)
	}
	return nil
}

//serve 启动服务器
func (T *ServerGroup) serve(srv *Server) {
	if srv.status.setTrue() {
		return
	}
	T.srvMan.Set(srv.Addr, srv)
	defer T.srvMan.Del(srv.Addr)
    err := srv.ListenAndServe()		//阻塞
	srv.status.setFalse()			//退出
    if err != nil {
        T.ErrorLog.Printf("server: ip(%s), %s\n", srv.Addr, err.Error())
    }
}

//Start 启动服务集群
//	error   错误
func (T *ServerGroup) Start() error {
	if T.run.setTrue() {
		return verror.TrackErrorf("server: 服务组已经开启。")
	}

	//站点池
	if T.sitePool == nil {
		pool := vweb.DefaultSitePool
		err := pool.Start()
		if err == nil {
			defer pool.Close()
		}
		T.sitePool = pool
	}
	
	//刷新配置
	if T.config != nil {
		T.UpdateConfig(T.config)
	}
	
	//等待退出
	<-T.exit
    return nil
}

//Close 关闭服务集群
//	error   错误
func (T *ServerGroup) Close() error {
    if T.run.setFalse() {
		return verror.TrackErrorf("server: 服务组已经关闭！")
    }
    
    //关闭监听
    T.srvMan.Range(func(k, v interface{})bool{
        if srv, ok := v.(*Server); ok{
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
    	return true
    })
    
	T.sitePool = nil
   	T.exit <- true
    return nil
}

//httpError 返回错误到客户端
//	w http.ResponseWriter           响应
//	rootDir string					根目录
//	errorPage map[string]string     错误页地址
//	e string                        错误内容，如果错误页不存在，将使用内容
//	code int                        错误代码
func httpError(w http.ResponseWriter, rootDir string, errorPage map[string]string, e string, code int) error {
    if errorPage != nil {
        c := strconv.Itoa(code)
        ep, ok := errorPage[c]
        if ok {
        	p := filepath.Join(rootDir, ep)
        	b, err := ioutil.ReadFile(p)
            if err != nil {
            	return err
            }else{
            	http.Error(w, string(b), code)
                return nil
            }
        }
    }
    http.Error(w, e, code)
    return nil
}

//httpTypeByExtension 文件类型扩展，如果自定义列表不存在扩展类型，则使用系统默认扩展类型。如果自定义列表扩展类型是空“”的类型，说明是用户设置拒绝访问该类型。
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

