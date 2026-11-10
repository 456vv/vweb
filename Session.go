package vweb

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/456vv/vmap/v2"
)

// Sessioner 用户独立的内存存储接口。
// 每个会话对应一个浏览器/客户端，可在请求间保存任意键值，并支持过期与退出回调。
//
// 典型用法：
//
//	sess := site.Sessions().Session(w, r)
//	sess.Set("user_id", 42)
//	if v, ok := sess.GetHas("user_id"); ok { ... }
type Sessioner interface {
	Token() string                                        // 会话唯一编号（Cookie 值）
	Set(key, val any)                                     // 设置键值
	Has(key any) bool                                     // 是否存在键
	Get(key any) any                                      // 读取（不存在返回 nil）
	GetHas(key any) (val any, ok bool)                    // 读取并判断是否存在
	Del(key any)                                          // 删除单键
	Dels(keys []any)                                      // 批量删除
	SetExpired(key any, d time.Duration)                  // 为键设置过期时间
	SetExpiredCall(key any, d time.Duration, f func(any)) // 过期时调用回调
	TTL(key any) (time.Duration, bool)                    // 查询键剩余存活时间
	Reset()                                               // 清空所有键（触发过期清理）
	Defer(call any, args ...any) error                    // 会话销毁时执行的延迟回调
}

// Session 会话用于用户保存数据。
// 内嵌 vmap.Map 提供线程安全的键值存储；id 由 Sessions 管理；exitCall 在 Free 时执行。
type Session struct {
	vmap.Map          // 数据，用户存储的数据
	id       string   // id，给 Sessions 使用的
	exitCall ExitCall // 退出时调用的函数队列
	freed    atomic.Bool
}

// Reset 清空会话内所有键值（等价于 ExpireAll）。
// 示例：sess.Reset()
func (T *Session) Reset() {
	T.Map.ExpireAll()
}

// Token 读取当前的令牌（会话 ID）。
//
//	返回：如 "a1b2c3d4..." 的十六进制或加盐字符串
func (T *Session) Token() string {
	return T.id
}

// Defer 在用户会话时间过期或被销毁后将被调用。
//
// 参数：
//   - call: 函数（任意可调用对象）
//   - args: 传给该函数的参数，或后续更多函数
//
// 返回：若会话已 Free 则返回 "has execute freed"
//
// 示例：
//
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (T *Session) Defer(call any, args ...any) error {
	if T.freed.Load() {
		return errors.New("has execute freed")
	}
	return T.exitCall.Defer(call, args...)
}

// Free 执行结束 Defer
// 幂等：多次调用安全；设置 freed 后禁止再 Defer。
func (T *Session) Free() {
	T.freed.Store(true)
	// 执行退出函数
	T.exitCall.Free()
}
