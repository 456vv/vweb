package vweb

import (
    "github.com/456vv/vmap/v2"
    "time"
    "sync"
)

//Sessioner 用户独立的内存存储接口
type Sessioner interface {
	Set(key, val interface{})
    Has(key interface{}) bool
    Get(key interface{}) interface{}
    GetHas(key interface{}) (val interface{}, ok bool)
    Del(key interface{})
    SetExpired(key interface{}, d time.Duration)
    Reset()
    Defer(call interface{}, args ... interface{}) error
    Free()
}



//Session 会话用于用户保存数据
type Session struct{
    *vmap.Map                                                                               // 数据，用户存储的数据
	id			string																		// id，给Sessions使用的
    exitCall	exitCall																	// 退回调用函数
    expired		map[interface{}]*time.Timer													// 有效期
    m			sync.Mutex																	// 锁
}

func NewSession() *Session {
	return &Session{
        Map : vmap.NewMap(),
        expired: make(map[interface{}]*time.Timer),
    }
}

// SetExpired 单个键值的有效期
//	key interface{}		键名
//	d time.Duration		时间
func (s *Session) SetExpired(key interface{}, d time.Duration){
	s.m.Lock()
	defer s.m.Unlock()
	
	//如果该Key不存在，则退出
	if !s.Has(key) {
		return
	}
	
	//存在定时，使用定时。如果过期，则创建新的定时
	if timer, ok := s.expired[key]; ok {
		if timer.Reset(d) {
			return
		}
		timer.Stop()
	}
	
	s.expired[key]=time.AfterFunc(d, func(){
		s.m.Lock()
		defer s.m.Unlock()
		delete(s.expired, key)
		s.Del(key)
	})
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


//Free 执行结束Defer
func (s *Session) Free() {
	s.exitCall.Free()
}
