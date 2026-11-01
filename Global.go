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
	Range(f func(key, value any) bool)                    // 遍历
	GetNewMaps(keys ...any) *vmap.Map                     // 更多
}
