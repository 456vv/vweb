package vweb

import (
	"time"

	"github.com/456vv/vmap/v2"
)

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

type Global struct {
	*vmap.Map
}

func (T *Global) Reset() {
	T.Map.ExpireAll()
}
