package vweb

import (
	"errors"
	"sync"
	"time"

	"github.com/456vv/vmap/v2"
)

// SitePool 网站池
type SitePool struct {
	pool   sync.Map     // map[host]*Site
	tick   *time.Ticker // 定时器
	exit   chan bool    // 退出
	closed atomicBool   // 已经启动
}

func NewSitePool() *SitePool {
	sp := &SitePool{
		exit: make(chan bool),
		tick: time.NewTicker(time.Second),
	}
	go sp.start()
	return sp
}

// NewSite 创建一个站点，如果存在返回已经存在的。Sessions 使用默认的设置，你需要修改它。
//
//	name string		站点name
//	*Site			站点
func (T *SitePool) NewSite(name string) *Site {
	if inf, ok := T.pool.Load(name); ok {
		return inf.(*Site)
	}
	site := &Site{
		identity: name,
		sessions: &Sessions{
			Expired: time.Minute * 20,
			Name:    "VID",
			Size:    64,
		},
		global: vmap.NewMap(),
	}
	T.pool.Store(name, site)
	return site
}

// DelSite 删除站点
//
//	name string		站点name
func (T *SitePool) DelSite(name string) {
	T.pool.Delete(name)
}

// RangeSite 迭举站点
//
//	f func(name string, site *Site) bool
func (T *SitePool) RangeSite(f func(name string, site *Site) bool) {
	T.pool.Range(func(key, value any) bool {
		return f(key.(string), value.(*Site))
	})
}

// SetRecoverSession 设置回收无效的会话间隔。默认为1秒
//
//	d time.Duration     回收时间隔，不可以等于或小于0,否则CPU爆增（警告）
func (T *SitePool) SetRecoverSession(d time.Duration) {
	T.tick.Reset(d)
}

func (T *SitePool) start() {
L:
	for {
		select {
		case <-T.tick.C:
			T.pool.Range(func(host, inf any) bool {
				if site, ok := inf.(*Site); ok {
					go site.sessions.CheckDeadAll()
				}
				return true
			})
		case <-T.exit:
			break L
		}
	}
}

// Close 关闭池,关闭前将遍历池中的站点出来清理会话和全局数据。如果已经关闭则返回错误。
//
//	error   错误
func (T *SitePool) Close() error {
	if !T.closed.setTrue() {
		T.exit <- true
		T.tick.Stop()
		T.pool.Range(func(key, value any) bool {
			if site, ok := value.(*Site); ok {
				// 清除Session
				site.Sessions().InstantDeadAll()
				// 清除全局数据
				site.Global().Reset()
				// 清除网站扩展数据
				site.Extend.Reset()
			}
			return true
		})
		return nil
	}

	return errors.New("SitePool is closed")
}

// Site 站点数据存储
type Site struct {
	sessions *Sessions                // 会话集
	global   Globaler                 // Global
	RootDir  func(path string) string // 网站的根目录z
	Extend   vmap.Map                 // 接口类型，可以自己存在任何类型
	identity string
}

// PoolName 网站池名称
//
//	string	名称
func (T *Site) PoolName() string {
	return T.identity
}

func (T *Site) Sessions() *Sessions {
	return T.sessions
}

func (T *Site) Global() Globaler {
	return T.global
}

type SiteMan struct {
	site sync.Map
}

// Add 设置一个站点
//
//	host string		站点host
//	*Site			站点
func (T *SiteMan) Add(host string, site *Site) {
	if site == nil {
		T.site.Delete(host)
		return
	}
	T.site.Store(host, site)
}

// Get 读取一个站点
//
//	host string		站点host
//	*Site			站点
//	bool			true存在，否则没有
func (T *SiteMan) Get(host string) (*Site, bool) {
	if inf, ok := T.site.Load(host); ok {
		return inf.(*Site), ok
	}

	return T.derogatoryDomain(host)
}

// Range 迭举站点
//
//	f func(host string, site *Site) bool
func (T *SiteMan) Range(f func(host string, site *Site) bool) {
	T.site.Range(func(key, value any) bool {
		return f(key.(string), value.(*Site))
	})
}

// 读出站点，支持贬域名。
func (T *SiteMan) derogatoryDomain(host string) (s *Site, ok bool) {
	var inf any
	derogatoryDomain(host, func(domain string) bool {
		inf, ok = T.site.Load(domain)
		return ok
	})
	if ok {
		return inf.(*Site), ok
	}
	return nil, false
}
