package vweb

import (
	"time"
    "github.com/456vv/vmap/v2"
    "fmt"
)

var DefaultSitePool    = NewSitePool()                                                      // 网站池（默认）

//SitePool 网站池
type SitePool struct {
	Pool					*vmap.Map                                                       // map[池名]*Site
    recoverSessionTick      time.Duration                                             	 	// 回收无效会话(默认20会分钟回收一次)
    setTick					chan bool
    exit                    chan bool                                                       // 退出
    run						atomicBool														// 已经启动
}

func NewSitePool() *SitePool {
    return &SitePool{
        Pool                : vmap.NewMap(),
        recoverSessionTick  : time.Minute*20,
        setTick             : make(chan bool,1),
        exit                : make(chan bool),
    }
}

//SetRecoverSession 设置回收无效的会话。
//  参：
//      d time.Duration     回收时间隔
func (T *SitePool) SetRecoverSession(d time.Duration) {
    T.recoverSessionTick = d
    select {
    case <-T.setTick:
    default:
    }
   	T.setTick <- true
}

//Start 启动池
//  返：
//      error   错误
func (T *SitePool) Start() error {
	if T.run.setTrue() {
		return fmt.Errorf("vweb.SitePool.Start: 已经启动，无需重复调用！")
	}
	
	//处理Session的过期
	rst 	:= T.recoverSessionTick
    tick 	:= time.NewTicker(rst)
    L:for {
    	select {
    		case <-T.setTick:
		        //判断过期时间是否有变动
		        if T.recoverSessionTick != rst {
    				tick.Stop()
		        	rst 	= T.recoverSessionTick
		        	tick 	= time.NewTicker(rst)
		        }
    		case <-tick.C:
                T.Pool.Range(func(name, vsite interface{}) bool {
                	site, ok := vsite.(*Site)
                	if ok {
                    	go site.Sessions.ProcessDeadAll()
                	}
                    return true
                })
    	    case <-T.exit:
                tick.Stop()
                break L
    	}
    }
    return nil
}

//Close 关闭池
//  返：
//      error   错误
func (T *SitePool) Close() error {
	if T.run.setFalse() {
		return fmt.Errorf("vweb.SitePool.Close: 已经关闭，无需重复调用！")
	}
    T.exit <- true
    return nil
}


//站点数据存储
type Site struct {
    Sessions            *Sessions                                                           // 会话集
    Global              Globaler                                                            // Global
    Config              interface{}                                                         // Config
    Plugin				*vmap.Map															// 插件map[type]map[name]interface{}
}

func NewSite() *Site {
    return &Site{
        Sessions	: newSessions(),
        Global		: vmap.NewMap(),
        Plugin		: vmap.NewMap(),
    }
}

