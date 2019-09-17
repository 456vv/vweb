package vweb

import (
    "github.com/456vv/vmap/v2"
    "time"
)

//Sessioner 用户独立的内存存储接口
type Sessioner interface {
	Set(key, val interface{})
    Has(key interface{}) bool
    Get(key interface{}) interface{}
    GetHas(key interface{}) (val interface{}, ok bool)
    Del(key interface{})
    SetExpired(key interface{}, d time.Duration)
    SetExpiredCall(key interface{}, d time.Duration, f func(interface{}))
    Reset()
    Defer(call interface{}, args ... interface{}) error
    Free()
}



//Session 会话用于用户保存数据
type Session struct{
    *vmap.Map                                                                               // 数据，用户存储的数据
	id			string																		// id，给Sessions使用的
    exitCall	exitCall																	// 退回调用函数
}

func NewSession() *Session {
	return &Session{
        Map : vmap.NewMap(),
    }
}


// Defer 在用户会话时间过期后，将被调用。
//	call interface{}            函数
//	args ... interface{}        参数或更多个函数是函数的参数
//	error                       错误
//  例：
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (s *Session) Defer(call interface{}, args ... interface{}) error {
	return s.exitCall.Defer(call, args...)
}


//Free 执行结束Defer和键值有效期
func (s *Session) Free() {
	//执行退出函数
	s.exitCall.Free()
}
