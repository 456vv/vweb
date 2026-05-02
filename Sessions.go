package vweb

import (
	"fmt"
	"net/http"
	"time"

	"github.com/456vv/vmap/v2"
)

type manageSession struct {
	s           Sessioner
	recent      time.Time
	unavailable bool
}

// Sessions 用于管理不同用户的会话。
type Sessions struct {
	Expired      time.Duration // 保存session时间长
	Name         string        // 标识名称。用于Cookie
	Size         int           // 会话ID长度。用于Cookie
	Salt         string        // 加盐，由于计算机随机数是伪随机数。（可默认为空）。用于Cookie
	ActivationID bool          // 为true，保持会话ID。意思就是会话ID过期了，可以激活再次使用。用于Cookie
	ss           vmap.Map      // 集，map[id]*Session
}

// Len 当前Session数量
//
//	int	数量
func (T *Sessions) Len() int {
	return T.ss.Len()
}

func (T *Sessions) InstantDeadAll() {
	T.ss.Range(func(id, mse any) bool {
		ms := mse.(*manageSession)
		// 多线程情况下防止重复检测浪费资源
		if ms.unavailable {
			return true
		}
		ms.unavailable = true
		// 执行Defer
		go ms.s.Free()
		return true
	})
	T.ss.Reset()
}

// CheckDeadAll 定时来处理过期的Session
//
//	[]string	过期的ID名称
func (T *Sessions) CheckDeadAll() []any {
	var expID []any
	if T.Expired != 0 {
		currTime := time.Now()
		T.ss.Range(func(id, mse any) bool {
			ms := mse.(*manageSession)
			// 多线程情况下防止重复检测浪费资源
			if ms.unavailable {
				return true
			}
			recentTime := ms.recent.Add(T.Expired)
			if currTime.After(recentTime) {
				ms.unavailable = true
				// 追加了expId一次性删除
				expID = append(expID, id)
				// 执行Defer
				go ms.s.Free()
			}
			return true
		})
		T.ss.Dels(expID)
	}
	return expID
}

// triggerDeadSession 由用户来触发，并删除已挂载入的Defer
func (T *Sessions) triggerDeadSession(ms *manageSession) (ok bool) {
	if ms.unavailable {
		return true
	}
	if T.Expired != 0 {
		currTime := time.Now()
		recentTime := ms.recent.Add(T.Expired)
		if currTime.After(recentTime) {
			ms.unavailable = true
			go ms.s.Free()
			return true
		}
	}
	return
}

// generateSessionID 生成Session标识符
//
//	string  标识符
func (T *Sessions) generateSessionID() string {
	rnd := make([]byte, T.Size)
	err := GenerateRandomID(rnd)
	if err != nil {
		panic(err)
	}
	if T.Salt != "" {
		return AddSalt(rnd, T.Salt)
	}
	id := fmt.Sprintf("%x", rnd)
	return id[:T.Size]
}

// SessionID 从请求中读取会话标识
//
//	req *http.Request   请求
//	id string           id标识符
//	err error           错误
func (T *Sessions) SessionID(req *http.Request) (id string, err error) {
	c, err := req.Cookie(T.Name)
	if err != nil || c.Value == "" {
		return "", fmt.Errorf("vweb: 该用会话属性（%s）名称，从客户端请求中没有找可用ID值。", T.Name)
	}
	return c.Value, nil
}

// NewSession 新建会话
//
//	id string	id标识符
//	Sessioner   会话
func (T *Sessions) NewSession(id string) Sessioner {
	if s, ok := T.GetSession(id); ok {
		return s
	}
	return T.SetSession(id, new(Session))
}

// GetSession 使用id读取会话
//
//	id string   id标识符
//	Sessioner   会话
//	bool        是否存在
func (T *Sessions) GetSession(id string) (Sessioner, bool) {
	mse, ok := T.ss.GetHas(id)
	if !ok {
		return nil, false
	}

	ms := mse.(*manageSession)
	if ms.unavailable {
		return nil, false
	}

	if T.triggerDeadSession(ms) {
		T.ss.Del(id)
		return nil, false
	}
	ms.recent = time.Now()
	return ms.s, true
}

// SetSession 使用id写入新的会话
//
//	id string   id标识符
//	s Sessioner 新的会话
//	Sessioner   会话
func (T *Sessions) SetSession(id string, s Sessioner) Sessioner {
	if inf, ok := T.ss.GetHas(id); ok {
		ms := inf.(*manageSession)
		// 在执行ProcessDeadAll时候，已经过期了，只是还没有被删除
		// 所以检测有没有过期，否则不能读取返回
		if !ms.unavailable {
			if ms.s.Token() == s.Token() {
				// 已经存在，无法再设置
				return s
			}
			// 设置为不可用,防止被读取
			ms.unavailable = true
			// 替换原有Session，需要清理原有的defer
			go ms.s.Free()
		}
	}
	if t, can := s.(*Session); can {
		// 对应这个id，并保存
		t.id = id
	}
	ms := &manageSession{
		s:      s,
		recent: time.Now(),
	}
	T.ss.Set(id, ms)
	return s
}

// DelSession 使用id删除的会话
//
//	id string   id标识符
func (T *Sessions) DelSession(id string) {
	if mse, ok := T.ss.GetHas(id); ok {
		T.ss.Del(id)
		go mse.(*manageSession).s.Free()
	}
}

// writeToClient 写入到客户端
//
//	rw http.ResponseWriter  响应
//	id string               id标识符
//	Sessioner    			会话
func (T *Sessions) writeToClient(rw http.ResponseWriter) Sessioner {
	wh := rw.Header()

	// 防止重复写入
	for _, c := range readSetCookies(wh) {
		if c.Name == T.Name {
			if ss, ok := T.GetSession(c.Value); ok {
				return ss
			}
		}
	}

	// 客户是第一次请求，没有会话ID
	// 现在生成一个ID给客户端
	id := T.generateRandSessionID()
	cookie := &http.Cookie{
		Name:     T.Name,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
	}
	wh.Add("Set-Cookie", cookie.String())
	return T.SetSession(id, new(Session))
}

func (T *Sessions) generateRandSessionID() string {
	var (
		id      = T.generateSessionID()
		maxWait = time.Second
		wait    time.Duration
	)
	for T.ss.Has(id) {
		wait = delay(wait, maxWait)
		id = T.generateSessionID()
		if wait >= maxWait {
			id += "-temp"
			// ID即将耗尽
			fmt.Printf("vweb: 警告>>会话ID已耗尽，请尽快加大调整ID长度。本次已为用户分配临时ID(%s)\n", id)
		}
	}
	return id
}

// Session 会话
//
//	rw http.ResponseWriter  响应
//	req *http.Request       请求
//	Sessioner               会话接口
func (T *Sessions) Session(rw http.ResponseWriter, req *http.Request) Sessioner {
	// 判断标识名是否存在
	id, err := T.SessionID(req)
	if err != nil {
		return T.writeToClient(rw)
	}

	// 判断Id是否有效
	s, ok := T.GetSession(id)
	if !ok {
		return T.writeToClient(rw)
	}
	return s
}
