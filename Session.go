package vweb

import (
    "github.com/456vv/vmap/v2"
    "time"
)

//Sessioner 用户独立的内存存储接口
type Sessioner interface {
    Token() string																									// 编号
    Set(key, val any)																						// 设置
    Has(key any) bool																						// 判断
    Get(key any) any																				// 读取
    GetHas(key any) (val any, ok bool)																// 读取判断
    Del(key any)																							// 删除
    SetExpired(key any, d time.Duration)																	// 过期
    SetExpiredCall(key any, d time.Duration, f func(any))											// 过期调用
    Reset()																											// 重置
    Defer(call any, args ...any) error																// 退出调用
    Free()																											// 释放调用
}

//Session 会话用于用户保存数据
type Session struct{
    vmap.Map                                                                                // 数据，用户存储的数据
	id			string																		// id，给Sessions使用的
    ExitCall	ExitCall																	// 退回调用函数
}



// Token 读取当前的令牌
//	string	令牌
func (T *Session) Token() string {
	return T.id
}

// Defer 在用户会话时间过期后，将被调用。
//	call any            函数
//	args ... any        参数或更多个函数是函数的参数
//	error                       错误
//  例：
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (T *Session) Defer(call any, args ... any) error {
	return T.ExitCall.Defer(call, args...)
}


//Free 执行结束Defer和键值有效期
func (T *Session) Free() {
	//执行退出函数
	T.ExitCall.Free()
}
