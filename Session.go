package vweb

import (
    "github.com/456vv/vmap/v2"
    "time"
)

//Sessioner 用户独立的内存存储接口
type Sessioner interface {
    Token() string																									// 编号
    Set(key, val interface{})																						// 设置
    Has(key interface{}) bool																						// 判断
    Get(key interface{}) interface{}																				// 读取
    GetHas(key interface{}) (val interface{}, ok bool)																// 读取判断
    Del(key interface{})																							// 删除
    SetExpired(key interface{}, d time.Duration)																	// 过期
    SetExpiredCall(key interface{}, d time.Duration, f func(interface{}))											// 过期调用
    Reset()																											// 重置
    Defer(call interface{}, args ...interface{}) error																// 退出调用
    Free()																											// 释放调用
}

//Session 会话用于用户保存数据
type Session struct{
    vmap.Map                                                                                // 数据，用户存储的数据
	id			string																		// id，给Sessions使用的
    exitCall	exitCall																	// 退回调用函数
}



// Token 读取当前的令牌
//	string	令牌
func (T *Session) Token() string {
	return T.id
}

// Defer 在用户会话时间过期后，将被调用。
//	call interface{}            函数
//	args ... interface{}        参数或更多个函数是函数的参数
//	error                       错误
//  例：
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (T *Session) Defer(call interface{}, args ... interface{}) error {
	return T.exitCall.Defer(call, args...)
}


//Free 执行结束Defer和键值有效期
func (T *Session) Free() {
	//执行退出函数
	T.exitCall.Free()
}
