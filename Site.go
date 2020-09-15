package vweb

import (
	"time"
    "github.com/456vv/verror"
    "github.com/456vv/vmap/v2"
    "sync"
)

var DefaultSitePool    = NewSitePool()                                                      // 网站池（默认）


//SitePool 网站池
type SitePool struct {
	pool					sync.Map                                                        // map[host]*Site
    recoverSessionTick      time.Duration                                             	 	// 回收无效会话(默认1秒)
    setTick					chan bool
    exit                    chan bool                                                       // 退出
    run						atomicBool														// 已经启动
}

func NewSitePool() *SitePool {
   	sp := &SitePool{
        recoverSessionTick  : time.Second,
        setTick             : make(chan bool,1),
        exit                : make(chan bool),
    }
    return sp
}


//NewSite 创建一个站点，如果存在返回已经存在的。Sessions 使用默认的设置，你需要修改它。
//	name string		站点name
//	*Site			站点
func (T *SitePool) NewSite(name string) *Site {
	if inf, ok := T.pool.Load(name); ok {
		return inf.(*Site)
	}
	site := &Site{
		identity:	name,
		Sessions:	&Sessions{
			Expired: 	time.Minute*20,
			Name:		"VID",
			Size:		64,
		},
		Global: 	vmap.NewMap(),
	}
	T.pool.Store(name, site)
	return site
}

//DelSite 删除站点
//	name string		站点name
func (T *SitePool) DelSite(name string) {
	T.pool.Delete(name)
}

//RangeSite 迭举站点
//	f func(name string, site *Site) bool
func (T *SitePool) RangeSite(f func(name string, site *Site) bool){
	T.pool.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*Site))
	})
}

//SetRecoverSession 设置回收无效的会话。默认为1秒
//	d time.Duration     回收时间隔，不可以是0
func (T *SitePool) SetRecoverSession(d time.Duration) {
	if d == 0 {
		return
	}
    T.recoverSessionTick = d
    select{
    case <-T.setTick:
    default:
    }
   	T.setTick <- true
}

//Start 启动池，用于读取处理过期的会话
//	error   错误
func (T *SitePool) Start() error {
	if T.run.setTrue() {
		return verror.TrackErrorf("vweb: 网站池已经启动!")
	}
	//处理Session的过期
	go T.start()
    return nil
}

func (T *SitePool) start() {
	//处理Session的过期
	rst 	:= T.recoverSessionTick
    tick 	:= time.NewTicker(rst)
    L:for {
    	select {
		case <-T.setTick:
	        //判断过期时间是否有变动
	        if T.recoverSessionTick != rst {
        		rst = T.recoverSessionTick
				tick.Reset(rst)
	        }
		case <-tick.C:
            T.pool.Range(func(host, inf interface{}) bool {
            	if site, ok := inf.(*Site); ok {
                	go site.Sessions.ProcessDeadAll()
            	}
                return true
            })
	    case <-T.exit:
            tick.Stop()
            break L
    	}
    }
}

//Close 关闭池
//	error   错误
func (T *SitePool) Close() error {
	if !T.run.setFalse() {
    	T.exit <- true
	}
    return nil
}


//Site 站点数据存储
type Site struct {
    Sessions	*Sessions                                                           // 会话集
    Global		Globaler                                                            // Global
    RootDir		func(path string) string											// 网站的根目录
    Extend		interface{}															// 接口类型，可以自己存在任何类型
	identity	string
}

// PoolName 网站池名称
//	string	名称
func (T *Site) PoolName() string {
	return T.identity
}


type SiteMan struct {
	site	sync.Map
}

//SetSite 设置一个站点
//	host string		站点host
//	*Site			站点
func (T *SiteMan) Add(host string, site *Site) {
	if site == nil {
		T.site.Delete(host)
		return
	}
	T.site.Store(host, site)
}

//GetSite 读取一个站点
//	host string		站点host
//	*Site			站点
//	bool			true存在，否则没有
func (T *SiteMan) Get(host string) (*Site, bool){
	if inf, ok := T.site.Load(host); ok {
		return inf.(*Site), ok
	}
	
	return T.derogatoryDomain(host)
}

//Range 迭举站点
//	f func(host string, site *Site) bool
func (T *SiteMan) Range(f func(host string, site *Site) bool){
	T.site.Range(func(key, value interface{}) bool {
		return f(key.(string), value.(*Site))
	})
}

//读出站点，支持贬域名。
func (T *SiteMan) derogatoryDomain(host string) (s *Site, ok bool) {
    var inf interface{}
	derogatoryDomain(host, func(domain string) bool {
        inf, ok = T.site.Load(domain)
        return ok
	});
	if ok {
        return inf.(*Site), ok
    }
    return nil, false
}


