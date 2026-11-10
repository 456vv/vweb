package vweb

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/456vv/vmap/v2"
)

// sitePoolCloseFreeWaitTimeout SitePool.Close 等待所有 Defer 完成的超时上限。
//
// InstantDeadAll 内部 Free 为异步；Close 作为“关闭站点池”的最终边界，
// 需要保证用户 Defer 已执行或已放弃等待。给一个有限超时，避免个别 Defer 卡死
// 拖垮整个 Close。
const sitePoolCloseFreeWaitTimeout = 3 * time.Second

// SitePool 网站池：按名称管理多个 *Site，并定期回收过期会话。
//
// 生命周期：
//
//	sp := NewSitePool()
//	site := sp.NewSite("default")
//	// ... 使用 site.Sessions() / site.Global()
//	sp.Close() // 停止 ticker、清理会话与全局数据
type SitePool struct {
	pool   sync.Map     // map[host]*Site
	tick   *time.Ticker // 定时回收会话
	exit   chan bool    // 退出信号
	closed atomic.Bool  // 是否已关闭
}

// NewSitePool 创建站点池并启动后台回收协程（默认每秒一次）。
func NewSitePool() *SitePool {
	sp := &SitePool{
		exit: make(chan bool),
		tick: time.NewTicker(time.Second),
	}
	go sp.start()
	return sp
}

// NewSite 创建一个站点；若 name 已存在则返回已有实例。
// Sessions 使用默认：Expired=20min, Name="VID", Size=64。
//
// 参数 name：站点标识（池内唯一）
// 返回：*Site（永不 nil）
//
// 示例：
//
//	site := pool.NewSite("blog")
func (T *SitePool) NewSite(name string) *Site {
	site := &Site{
		identity: name,
		sessions: &Sessions{
			Expired: time.Minute * 20,
			Name:    "VID",
			Size:    64,
		},
		global: &Global{
			Map: vmap.NewMap(),
		},
	}
	actual, loaded := T.pool.LoadOrStore(name, site)
	if T.closed.Load() {
		if !loaded {
			T.pool.Delete(name)
		}
		panic("vweb: The website pool has been closed")
	}
	if loaded {
		return actual.(*Site)
	}
	return site
}

// DelSite 从池中删除指定名称的站点（不主动 Free 会话，建议先 Close 或手动清理）。
func (T *SitePool) DelSite(name string) {
	T.pool.Delete(name)
}

// RangeSite 遍历池中所有站点；f 返回 false 时停止。
//
// 示例：
//
//	pool.RangeSite(func(name string, site *Site) bool {
//	    fmt.Println(name, site.Sessions().Len())
//	    return true
//	})
func (T *SitePool) RangeSite(f func(name string, site *Site) bool) {
	T.pool.Range(func(key, value any) bool {
		return f(key.(string), value.(*Site))
	})
}

// SetRecoverSession 设置回收无效会话的间隔。默认为 1 秒。
// d <= 0 时强制为 1 秒，防止 ticker 高频空转打满 CPU。
func (T *SitePool) SetRecoverSession(d time.Duration) {
	if T.closed.Load() {
		return
	}
	if d <= 0 {
		d = time.Second // 防止 <=0 导致 ticker 高频空转打满 CPU
	}
	if T.tick != nil {
		T.tick.Reset(d)
	}
}

// start 后台循环：定时对每个站点执行 sessions.CheckDeadAll。
func (T *SitePool) start() {
L:
	for {
		select {
		case <-T.tick.C:
			// 关闭后跳过，避免 Close 与 ticker 竞态再执行一次 CheckDeadAll
			if T.closed.Load() {
				continue
			}
			T.pool.Range(func(host, inf any) bool {
				if site, ok := inf.(*Site); ok {
					site.sessions.CheckDeadAll()
				}
				return true
			})
		case <-T.exit:
			break L
		}
	}
}

// Close 关闭池：停止 ticker、清理全局/扩展数据、异步 Free 所有会话并等待最多 3 秒。
// 已关闭则返回 "SitePool is closed"。
func (T *SitePool) Close() error {
	if !T.closed.Swap(true) {
		if T.tick != nil {
			T.tick.Stop()
		}
		if T.exit != nil {
			close(T.exit)
		}

		var sites []*Site
		T.pool.Range(func(key, value any) bool {
			if site, ok := value.(*Site); ok {
				sites = append(sites, site)
			}
			return true
		})
		T.pool.Clear()

		if len(sites) > 0 {
			var wg sync.WaitGroup
			for _, s := range sites {
				s.sessions.InstantDeadAllInto(&wg)
			}

			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			ctx, cancel := context.WithTimeout(context.Background(), sitePoolCloseFreeWaitTimeout)
			select {
			case <-done:
			case <-ctx.Done():
			}
			cancel()

			for _, site := range sites {
				// 清除全局数据
				site.global.ExpireAll()
				// 清除网站扩展数据
				site.Extend.ExpireAll()
			}
		}

		return nil
	}
	return errors.New("SitePool is closed")
}

// Site 站点数据存储：会话集、全局 Map、根目录函数、扩展数据。
type Site struct {
	sessions *Sessions                // 会话集
	global   *Global                  // 站点级全局数据
	RootDir  func(path string) string // 网站的根目录解析
	Extend   vmap.Map                 // 用户自定义扩展数据
	identity string
}

// PoolName 返回站点在池中的名称（Identity）。
func (T *Site) PoolName() string {
	return T.identity
}

// Sessions 返回本站点的会话管理器。
func (T *Site) Sessions() *Sessions {
	return T.sessions
}

// Global 返回站点全局数据接口。
func (T *Site) Global() Globaler {
	return T.global
}

// SiteMan 按 Host 管理 *Site 的映射，支持贬域名（通配符）匹配。
type SiteMan struct {
	site sync.Map // map[host]*Site
}

// Add 设置一个站点；site==nil 时删除该 host。
func (T *SiteMan) Add(host string, site *Site) {
	if site == nil {
		T.site.Delete(host)
		return
	}
	T.site.Store(host, site)
}

// Get 读取站点；精确匹配失败时尝试贬域名（如 *.example.com）。
//
// 返回：(*Site, true) 或 (nil, false)
func (T *SiteMan) Get(host string) (*Site, bool) {
	if inf, ok := T.site.Load(host); ok {
		return inf.(*Site), ok
	}
	return T.derogatoryDomain(host)
}

// Range 遍历所有精确注册的站点。
func (T *SiteMan) Range(f func(host string, site *Site) bool) {
	T.site.Range(func(key, value any) bool {
		return f(key.(string), value.(*Site))
	})
}

// derogatoryDomain 从左向右将标签替换为 * 做通配匹配。
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
