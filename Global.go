package vweb

import (
	"time"
)
type Globaler interface {
    Set(key, val interface{})                                                                   // 设置
    Has(key interface{}) bool                                                                   // 检查
    Get(key interface{}) interface{}                                                            // 读取
    Del(key interface{})                                                                        // 删除
	SetExpired(key interface{}, d time.Duration)  												// 设置KEY有效期，过期会自动删除
	SetExpiredCall(key interface{}, d time.Duration, f func(interface{}))    					// 设置KEY有效期，过期会自动删除，并调用函数
    Reset()                                                                                     // 重置
}

