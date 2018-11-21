package vweb

import (
	"fmt"
    //"reflect"
    "net"
    "net/http"
    "crypto/tls"
    "log"
    "time"
    "io/ioutil"
    "bytes"
    "strings"
    "regexp"
    //"net/url"
    "path"
    "path/filepath"
    "os"
    "strconv"
    "mime"
    "github.com/456vv/vmap/v2"
    "context"

)



//Server 服务器,使用在 ServerGroup.srvMan 字段。
type Server struct {
    *http.Server                                                                            // http服务器
	Listener            net.Listener                                                        // 监听
	status				atomicBool															// 已经监听
	cc					*ConfigConn															// 连接配置
	cs					*ConfigServer														// 服务器配置
}

//配置监听
//	laddr string	监听地址
//	CC *ConfigConn	配置
func (T *Server) ConfigListener(laddr string, CC *ConfigConn) error {
	var err error
	T.cc = CC
	if T.Listener == nil {
		T.Listener, err = net.Listen("tcp", laddr)
		if err != nil {
			return err
		}
		//连接配置
	    T.Listener = &tcpKeepAliveListener{
	        TCPListener : T.Listener.(*net.TCPListener),
	        cc			: T.cc,
	    }
	}
	return nil
}

//配置服务器
//	CS *ConfigServer	配置
func (T *Server) ConfigServer(CS *ConfigServer) error {
	T.cs  = CS
	if T.Listener == nil {
		return fmt.Errorf("vweb: 服务器监听对象为nil，需要先调用 .ConfigListener 方法配置！")
	}
	if T.Server == nil {
		T.Server 		= new(http.Server)
	    T.Server.Addr	= T.Listener.Addr().String()
	    if CS.TLS != nil {
	    	T.Server.TLSConfig = new(tls.Config)
	        T.Listener = tls.NewListener(T.Listener, T.Server.TLSConfig)
	    }
   	}
	//服务器配置
    T.Server.ReadTimeout      	= time.Duration(CS.ReadTimeout) * time.Millisecond
    T.Server.WriteTimeout     	= time.Duration(CS.WriteTimeout) * time.Millisecond
    T.Server.ReadHeaderTimeout	= time.Duration(CS.ReadHeaderTimeout) * time.Millisecond
    T.Server.IdleTimeout		= time.Duration(CS.IdleTimeout) * time.Millisecond
    T.Server.MaxHeaderBytes   	= CS.MaxHeaderBytes
    T.Server.SetKeepAlivesEnabled(CS.KeepAlivesEnabled)
    
   	if CS.TLS != nil {
    	err := configTLSFile(T.Server.TLSConfig, CS.TLS)
    	if err != nil {
    		return err
    	}
   	}
   	return nil
}
//TLS文件配置
func configTLSFile(c *tls.Config, conf *ConfigServerTLS) error {
	if conf == nil {
    	return fmt.Errorf("vweb: TLS配置无效，因为没有配置 ConfigServers.CS.TLS")
	}
    c.NextProtos                = conf.NextProtos
    c.PreferServerCipherSuites  = conf.PreferServerCipherSuites
    c.SessionTicketsDisabled    = conf.SessionTicketsDisabled
    c.MinVersion                = conf.MinVersion
    c.MaxVersion                = conf.MaxVersion
   	c.SessionTicketKey			= conf.SessionTicketKey

	if len(conf.CipherSuites) >0 {
    	c.CipherSuites			= conf.CipherSuites
	}else{
		//内部判断并使用默认的密码套件
    	c.CipherSuites			= nil
	}
    if !strSliceContains(c.NextProtos, "http/1.1") {
        c.NextProtos = append(c.NextProtos, "http/1.1")
    }

    if len(conf.SetSessionTicketKeys) > 0 {
    	c.SetSessionTicketKeys(conf.SetSessionTicketKeys)
    }

	var errStr string
    c.Certificates = nil
    for _, file := range conf.File {
	    cert, err := tls.LoadX509KeyPair(file.CertFile, file.KeyFile)
        if err == nil {
            c.Certificates = append(c.Certificates, cert)
        }else{
        	//日志
        	errStr = fmt.Sprintf("%s>>ConfigServers.CS.TLS.File{CertFile:%q, KeyFile:%q}无法配置: %s", errStr, file.CertFile, file.KeyFile, err.Error())
        }
    }
    //多证书
    c.BuildNameToCertificate()
    if errStr != "" {
    	return fmt.Errorf("vweb: %s", errStr)
    }
    return nil
}

type ServerGroup struct {
    ErrorLog			*log.Logger 		// 错误日志文件
    // srvMan 存储值类型是 *Server，读取时需要转换类型
    srvMan              *vmap.Map           // map[ip:port]*Server	服务器集
    sitePool			*SitePool			// 站点的池
    sitePooled			bool				// 有设置池
    sites				*Sites				// 站点集
    exit                chan bool			// 退出

	run					atomicBool			// 服务器启动了

    // 用于 .UpdateConfigFile 方法
    backConfigDate      []byte              // 备份配置数据。如果是相同数据，则不更新
    config				*Config				// 配置
}

func NewServerGroup() *ServerGroup {
	return &ServerGroup{
        srvMan          : vmap.NewMap(),
        sitePool		: DefaultSitePool,
        sites			: DefaultSites,
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
//	pool *SitePool	池
//	error			错误
func (T *ServerGroup) SetSitePool(pool *SitePool) error {
	T.sitePool 		= pool
	T.sitePooled 	= true
	return nil
}

//设置站点集，如果没有设置，则使用内置全局默认站点集。
//站点集主要是记录配置，方便每个Host读取对应的配置。
//	sites *Sites	集
//	error			错误
func (T *ServerGroup) SetSites(sites *Sites) error {
	T.sites 		= sites
	return nil
}

//serveHTTP 处理HTTP
//	rw http.ResponseWriter	响应
//	r *http.Request			请求
func (T *ServerGroup) serveHTTP(rw http.ResponseWriter, r *http.Request){
	
    //** 检查Host是否存在
    site, ok := T.sites.Site(r.Host)
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


    //** 转发URL
    forward := site.Config.Forward
    urlPath	:= r.URL.Path
    if forward != nil {
        var forwardC []ConfigSiteForward
        derogatoryDomain(r.Host, func(h string) (ok bool){
        	forwardC, ok = forward[h]
            return
        })

        for _, fc := range forwardC {
        	if !fc.Status {
        		//跳过禁止的
        		continue
        	}
        	
            //满足路径
            var okContain bool
            var okExclude bool
            var regExp *regexp.Regexp
            var err error

            //排除路径
        	for index, ep := range fc.ExcludePath {
            	//非正则
                if okExclude = (ep == urlPath); okExclude {
                	break
                }

            	//正则
            	regExp, err = regexp.Compile(ep)
                if err != nil {
                    //日志
                    if T.ErrorLog != nil {
						T.ErrorLog.Printf("vweb: ConfigSite.Forward.[%s][%d].ExcludePath(%s) 数组的正则是错误(%s)\r\n", r.Host, index, ep, err.Error())
                    }
                	continue
                }

                _, complete := regExp.LiteralPrefix()
                if !complete {
                	regExp.Longest()
                	okExclude = regExp.MatchString(urlPath)
	            	if okExclude {
	            		break
	            	}
                }
            }

            if !okExclude {
            	//包含路径
           		regExp = nil
	            for index, p := range fc.Path {
	            	//非正则
	                if okContain = (p == urlPath); okContain {
	                	break
	                }
	            	//正则
	            	regExp, err = regexp.Compile(p)
	                if err != nil {
	                    //日志
	                    if T.ErrorLog != nil {
                   			T.ErrorLog.Printf("vweb: ConfigSite.Forward.[%s][%d].Path(%s) 数组的正则是错误(%s)\r\n", r.Host, index, p, err.Error())
	                    }
	                	continue
	                }
	                _, complete := regExp.LiteralPrefix()
	                if !complete {
	                	regExp.Longest()
	                	okContain = regExp.MatchString(urlPath)
	                	if okContain {
	                		break
	                	}
	                }
	                regExp = nil
	            }

	            //修改路径地址
	            if okContain {
	            	if regExp != nil {
						var findAllSubmatch [][]string = regExp.FindAllStringSubmatch(urlPath, 1)
	            		urlPath = fc.RePath
						if len(findAllSubmatch) != 0 {
							//使用第一个匹配
							submatch := findAllSubmatch[0]
							for i, match := range submatch {
								urlPath = strings.Replace(urlPath, "$"+strconv.Itoa(i), match, -1)
							}
						}
	            	}else{
	            		urlPath = strings.Replace(fc.RePath, "$0", urlPath, -1)
	            	}

	            	if fc.RedirectCode != 0 {
	            		//重定向,并退出
	            		http.Redirect(rw, r, urlPath, fc.RedirectCode)
	            		return
	            	}

	            	if fc.End {
	            		//跳出
	            		break
	            	}
	            }
            }

        }
    }


    //** 文件存在
    var(
        config      = site.Config
        rootPath    = T.httpRootPath(&config.Directory, r)
        pagePath    string
    )

    if rootPath == "." {
    	//404 无法找到指定位置的资源。这也是一个常用的应答。
        http.Error(rw, "Web root directory is not set?", http.StatusNotFound)
        return
    }
    _, pagePath, err := PagePath(rootPath, urlPath, config.IndexFile)
    if err != nil {
    	//404 无法找到指定位置的资源。这也是一个常用的应答。
        httpError(rw, config.ErrorPage, err.Error(), http.StatusNotFound)
        return
    }

    //** 文件后缀支持
    var(
        fileExt         = path.Ext(pagePath)
        header          = config.Header
        contentType     = T.httpTypeByExtension(fileExt, header.MIME)
    )

    if contentType == "" {
    	//403 资源不可用。服务器解析客户的请求，但拒绝处理它。
        httpError(rw, config.ErrorPage, "This file suffix type MIME system does not recognize!", http.StatusForbidden)
        return
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
    	buffSize=defaultDataBufioSize
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
        shd := &ServerHandlerDynamic{
            RootPath        : rootPath,
            PagePath        : pagePath,
			BuffSize		: buffSize,
            Site            : site,
        }
        shd.ServeHTTP(rw, r)
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
	    
        shs := &ServerHandlerStatic{
            RootPath        : rootPath,
            PagePath        : pagePath,
		    PageExpired		: th.PageExpired,
			BuffSize		: buffSize,
        }
        shs.ServeHTTP(rw, r)
    }
}

//httpTypeByExtension 文件类型扩展，如果自定义列表不存在扩展类型，则使用系统默认扩展类型。如果自定义列表扩展类型是空“”的类型，说明是用户设置拒绝访问该类型。
//	ext string              文件后缀
//	me map[string]string    自定义扩展列表
//	string                  扩展类型
//	string                  文件类型
func (T *ServerGroup) httpTypeByExtension(ext string, me map[string]string) string {
	if me != nil {
	   if extType, ok := me[ext]; ok {
	      return extType
	   }
	}
	return mime.TypeByExtension(ext)
}

//httpRootPath	根目录
//	dir *ConfigSiteDirectory    目录
//	r *http.Request	    		请求
//	string			    		根目录路径
func (T *ServerGroup) httpRootPath(dir *ConfigSiteDirectory, r *http.Request) string {
    var (
        p		= filepath.Clean(r.URL.Path)
        root    = filepath.Clean(dir.Root)
    )

    for _, v := range dir.Virtual {
        if v == ""{
        	continue
        }else if !filepath.IsAbs(v) {
            var err error
        	v, err = filepath.Abs(v)
            if err != nil {
            	continue
            }
        }
    	v = filepath.Clean(v)
        pos	:= strings.LastIndex(v, "\\")
        if strings.HasPrefix(p+"\\", "\\"+v[pos+1:]+"\\") {
			root = v[:pos]
			break
		}
    }
    return root
}

//更新插件
func (T *ServerGroup) updatePluginConn(cSite *ConfigSite, site *Site){
	var (
		addrs 		= make(map[string]bool) //记录正在使用的IP地址
		names 		= make(map[string]bool) //记录正在使用的插件名称
		pmaphttp 	= site.Plugin.GetNewMap("HTTP")
		pmaprpc 	= site.Plugin.GetNewMap("RPC")
	)
	if cSite != nil {
		//所有类型
		for ptype, pvalue := range cSite.Plugin {
			//所有插件
			for pname, pconfig := range pvalue {
				if pconfig.Addr == "" {
	            	//日志
		             T.ErrorLog.Println(fmt.Sprintf("vweb: 名称为 %s 插件配置的 ConfigSitePlugin.Addr 字段是空的", pname))
	            	continue
				}
				switch {
				case ptype=="HTTP" :
					var httpC *PluginHTTPClient
					inf, ok := pmaphttp.GetHas(pname)
					if ok {
						httpC = inf.(*PluginHTTPClient)
					}
	                httpCC, err := configHTTPClient(httpC, pconfig)
	                if err != nil {
	                	//日志
	                	T.ErrorLog.Println(err.Error())
	                	continue
	                }
	        		pmaphttp.Set(pname, httpCC)
				case ptype=="RPC" :
				    if pconfig.Path == "" {
	        			//日志
	                	T.ErrorLog.Println(fmt.Sprintf("vweb: 名称为 %s 插件配置的 ConfigSitePlugin.Path 字段是空的", pname))
				   	    continue
				    }
					var rpcC *PluginRPCClient
					inf, ok := pmaprpc.GetHas(pname)
					if ok {
						rpcC = inf.(*PluginRPCClient)
					}
			    	rpcCC, err := configRPCClient(rpcC, pconfig)
	                if err != nil {
	                	//日志
	            		T.ErrorLog.Println(err.Error())
	                	continue
	                }
	        		pmaprpc.Set(pname, rpcCC)
				}//switch
				names[pname]=true
				addrs[pconfig.Addr]=true;
			}//for
		}//for
	}//if
	
    //关闭插件HTTP连接池
    var dels []interface{}
    pmaphttp.Range(func(k, v interface{}) bool {
        p := v.(*PluginHTTPClient)
        //IP地址不存在，关闭他
        if _, ok := addrs[p.Addr]; !ok {
        	p.Tr.CloseIdleConnections()
        }
        //插件名称不存在，删除他
        if _, ok := names[k.(string)]; ! ok {
        	dels = append(dels, k)
        }
        return true
    })
    pmaphttp.Dels(dels)

    //关闭插件RPC连接池
    dels = nil
    pmaprpc.Range(func(k, v interface{}) bool {
        p := v.(*PluginRPCClient)
        if _, ok := addrs[p.Addr]; !ok {
       		p.ConnPool.Close()
        }
        //插件名称不存在，删除他
        if _, ok := names[k.(string)]; ! ok {
        	dels = append(dels, k)
        }
        return true
    })
    pmaprpc.Dels(dels)
}

//updateSiteConfig 更新站点配置
//	hosts *Map      每个host都带着站点池名称，这样就可以从站点池中读出匹配。map[host]stieName
func (T *ServerGroup) updateSiteConfig(hosts *vmap.Map) {
    //删除
    var delhost []interface{}
    T.sites.Host.Range(func(host, name interface{})bool{
        if !hosts.Has(host) {
        	delhost = append(delhost, host)
        }
        return true
    })
    T.sites.Host.Dels(delhost)

    //增加
    hosts.Range(func(host, name interface{})bool{
        inf, ok := T.sitePool.Pool.GetHas(name)
        if ok {
            T.sites.Host.Set(host, inf)
        }
        return true
    })
}

//updateSitePoolAdd 更新站点池或增加
//	conf ConfigSite     配置
func (T *ServerGroup) updateSitePoolAdd(cSite *ConfigSite) {
    //从站点池里读出站点配置。如果不存在，则创建一个池
    var site    *Site
    inf, ok  := T.sitePool.Pool.GetHas(cSite.Name)
    if !ok {
        site = NewSite()
        T.sitePool.Pool.Set(cSite.Name, site)
    }else{
    	site = inf.(*Site)
    }

    site.Config=cSite
    site.Sessions.update(cSite.Property.Session)

    //更新插件连接
	T.updatePluginConn(cSite, site)
}

//updateSitePoolDel 更新站点池删除，过滤并删除无效的站点池。
//	names []string      现有的站点列表
func (T *ServerGroup) updateSitePoolDel(names []string) {
    var dels []interface{}
    T.sitePool.Pool.Range(func(n, v interface{})bool{
        var ok bool
        for _, name := range names {
        	if n.(string) == name {
        		ok = true
                break
        	}
        }
        if !ok {
            dels = append(dels, n)
            
		    //删除过期插件连接
			T.updatePluginConn(nil, v.(*Site))
        }
        
        return true
    })
    T.sitePool.Pool.Dels(dels)
}

//getServer 增加或读取Server
func (T *ServerGroup) getServer(laddr string) *Server {
    inf, ok := T.srvMan.GetHas(laddr)
    if ok {
    	return inf.(*Server)
    }
	return new(Server)
}

//listenStart 启动监听端口
func (T *ServerGroup) listenStart(laddr string, conf ConfigServers) error {
    srv := T.getServer(laddr)
    err := srv.ConfigListener(laddr, &conf.CC)
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
   	go T.serve(laddr, srv)
    return nil
}

//listenStop 关闭监听
func (T *ServerGroup) listenStop(laddr string) (err error) {
   if sm, ok := T.srvMan.GetHas(laddr); ok {
        if srv, ok := sm.(*Server); ok {
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
func (T *ServerGroup) updateConfigServers(conf map[string]ConfigServers) {
	var err error

    //如果在新的IP例表中没有找到已经存在的开放监听端口IP，而停止监听此IP
    T.srvMan.Range(func(key, val interface{}) bool{
        ip := key.(string)
        if _, ok := conf[ip]; !ok {
            err = T.listenStop(ip)
            if err != nil  && T.ErrorLog != nil {
                //日志
            	T.ErrorLog.Println(err.Error())
            }
        }
        return true
    })

    //如果还没开启监听，则启动他
    for laddr, css := range conf {
	    if css.Status {
	        err = T.listenStart(laddr, css)
	        if err != nil  && T.ErrorLog != nil {
	            //日志
            	T.ErrorLog.Println(err.Error())
	        }
	    }else{
	    	err = T.listenStop(laddr)
	        if err != nil  && T.ErrorLog != nil {
	            //日志
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
	T.config = conf
	if T.run.isTrue() {
		//更新网站配置
		if err := T.updateConfigSites(&conf.Sites); err != nil && T.ErrorLog != nil{
			//日志
			T.ErrorLog.Println(err.Error())
		}
		//更新服务器配置
		T.updateConfigServers(conf.Servers)
		return nil
	}
    return fmt.Errorf("vweb.ServerGroup.UpdateConfig: 配置被保存，还没生效，需要调用 .Start() 方法启动。")
}


func (T *ServerGroup) updateConfigSites(cSites *ConfigSites) error {
    var (
        newHost   	= vmap.NewMap()	//map[host]siteName
        siteName    []string 	//所有可用的
    )

    for i, cSite := range cSites.Site {
        if cSite.Status {
            if cSite.Name == "" {
                return fmt.Errorf("vweb: 配置中出现站点名称(Name)为 \"\"，需要设定一个名称。")
            }
            
            //预选分配池，初始化站点
            T.updateSitePoolAdd(&cSites.Site[i])
            
            //集中名称
            siteName = append(siteName, cSite.Name)

            //集中站点Host
            //可能有多个站点绑定了同一个Host，只有最后一个是有效的
            for _, host := range cSite.Host {
                newHost.Set(host, cSite.Name)
            }
        }
    }
    //可能想为什么不在for中处理这些？
    //由于配置文件是可以变动的，和在内存中的不匹配。无法索引出来删除。
    //只能筛选内存上没用的出来删除掉。

    //删除池中不存在的配置
    T.updateSitePoolDel(siteName)

    //更新网站配置
    T.updateSiteConfig(newHost)
    return nil

}

//serve 启动服务器
func (T *ServerGroup) serve(laddr string, srv *Server) error {
	if srv.status.setTrue() {
		return fmt.Errorf("vweb: 该服务器处于监听状态，无需再监听！")
	}
	T.srvMan.Set(laddr, srv)
	defer T.srvMan.Del(laddr)
    err := srv.Serve(srv.Listener)	//阻塞
	srv.status.setFalse()			//退回
    if err != nil && T.ErrorLog != nil {
    	//日志
        T.ErrorLog.Printf("vweb: ip(%s), %s\r\n", laddr, err.Error())
        return err
    }
    return err
}


//Start 启动服务集群
//	error   错误
func (T *ServerGroup) Start() error {
	if T.sitePool == nil {
		return fmt.Errorf("vweb.ServerGroup.Start: 请使用 .SetSitePool 方法设置有效的站点池。")
	}
	if T.sites == nil {
		return fmt.Errorf("vweb.ServerGroup.Start: 请使用 .SetSites 方法设置有效的站点集。")
	}
	if T.run.setTrue() {
		return fmt.Errorf("vweb.ServerGroup.Start: 不需要重复的调用 .Start() 方法。")
	}
	
		
	//启动内置池
	//false 表示使用内部默认池
	if !T.sitePooled {
		defer T.sitePool.Close()
		go T.sitePool.Start()
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

    //关闭监听
    T.srvMan.Range(func(k, v interface{})bool{
        if srv, ok := v.(*Server); ok{
        	if srv.Server != nil {
        		//srv.Server.Shutdown(context.Background())
        		srv.Server.Close()
        	}
        	if srv.Listener != nil {
        		srv.Listener.Close()
        	}
        }
        return true
    })
    T.srvMan.Reset()
	
	//如果服务启动，否则退出并设置为false。
    if T.run.isTrue() {
    	T.exit <- true
    	T.run.setFalse()
    }
    return nil
}

//strSliceContains 从切片中查找匹配的字符串
//	ss []string     切片
//	T string        需要从切片中查找的字符
func strSliceContains(ss []string, T string) bool {
	for _, v := range ss {
		if v == T {
			return true
		}
	}
	return false
}


//httpError 返回错误到客户端
//	w http.ResponseWriter           响应
//	errorPage map[string]string     错误页地址
//	e string                        错误内容，如果错误页不存在，将使用内容
//	code int                        错误代码
func httpError(w http.ResponseWriter, errorPage map[string]string, e string, code int){
    if errorPage != nil {
        c := strconv.Itoa(code)
        ep, ok := errorPage[c]
        if ok {
        	b, err := ioutil.ReadFile(ep)
            if err != nil {
            	//日志
            }else{
            	http.Error(w, string(b), code)
                return
            }
        }
    }
    http.Error(w, e, code)
}
