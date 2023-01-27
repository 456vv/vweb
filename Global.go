package vweb

import (
	"time"
)
type Globaler interface {
    Set(key, val any)                                                                   // 设置
    Has(key any) bool                                                                   // 检查
    Get(key any) any                                                            // 读取
    Del(key any)                                                                        // 删除
	SetExpired(key any, d time.Duration)  												// 设置KEY有效期，过期会自动删除
	SetExpiredCall(key any, d time.Duration, f func(any))    					// 设置KEY有效期，过期会自动删除，并调用函数
    Reset()                                                                                     // 重置
}

