package vweb

import (
	"time"

	"github.com/456vv/vmap/v2"
)

// Globaler 站点级全局键值存储接口（与 Session 类似，但作用于整个站点，非单用户）。
type Globaler interface {
	Set(key, val any)                                     // 设置
	Has(key any) bool                                     // 判断
	Get(key any) any                                      // 读取
	GetHas(key any) (val any, ok bool)                    // 读取判断
	Del(key any)                                          // 删除
	SetExpired(key any, d time.Duration)                  // 过期
	SetExpiredCall(key any, d time.Duration, f func(any)) // 过期调用
	TTL(key any) (time.Duration, bool)                    // 某个KEY过期时间
	Range(f func(key, value any) bool)                    // 遍历
	Reset()                                               // 重置
}

// Global 站点全局数据，内嵌 vmap.Map。
type Global struct {
	*vmap.Map
}

// 实现说明：Global 通过内嵌 *vmap.Map 满足 Globaler 的大部分方法；
// 具体方法由 vmap 提供，此处无需再包装。
func (T *Global) Reset() {
	T.Map.ExpireAll()
}
