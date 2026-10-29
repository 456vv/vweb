package vweb

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/456vv/vmap/v2"
)

type manageSession struct {
	s           Sessioner
	recent      atomic.Int64
	unavailable atomic.Bool
}

// Sessions 用于管理不同用户的会话。
//
// 注意：Sessions 内部含 sync.WaitGroup 与 vmap.Map，**不可按值复制**。
type Sessions struct {
	Expired time.Duration // 保存session时间长
	Name    string        // 标识名称。用于Cookie
	Size    int           // 会话ID长度。用于Cookie。（默认64位）
	Salt    string        // 加盐，由于计算机随机数是伪随机数。（可默认为空）。用于Cookie
	ss      vmap.Map      // 集，map[id]*Session
}

// Len 当前Session数量
//
//	int	数量
func (T *Sessions) Len() int {
	return T.ss.Len()
}

// InstantDeadAll 立即清理全部会话，并异步执行各自的 Defer。
func (T *Sessions) InstantDeadAll() {
	T.instantDeadAll(nil)
}

// InstantDeadAllInto 与 InstantDeadAll 行为一致，并把每个异步 Free 注册到调用方提供的 wg。
// wg 不能为 nil（否则与 InstantDeadAll 等价，无必要调用本方法）。
//
// 使用约定：
//   - 调用方保证 wg 的 Wait 在本方法返回之后才发起；
//   - 调用方自行控制超时（本方法不阻塞）；
//   - 多个 Sessions 可共享同一个 wg（SitePool.Close 正是如此）。
func (T *Sessions) InstantDeadAllInto(wg *sync.WaitGroup) {
	if wg == nil {
		// 防御：nil 等价于无跟踪调用，避免调用方误传
		T.instantDeadAll(nil)
		return
	}
	T.instantDeadAll(wg)
}

// instantDeadAll 是 InstantDeadAll / InstantDeadAllInto 的共同实现。
func (T *Sessions) instantDeadAll(wg *sync.WaitGroup) {
	var frees []Sessioner
	T.ss.Range(func(id, mse any) bool {
		ms := mse.(*manageSession)
		if ms.unavailable.Swap(true) {
			return true
		}
		frees = append(frees, ms.s)
		return true
	})
	T.ss.Reset()

	if len(frees) == 0 {
		return
	}
	if wg != nil {
		wg.Add(len(frees))
	}
	for _, s := range frees {
		go func(s Sessioner) {
			if wg != nil {
				defer wg.Done()
			}
			s.Free()
		}(s)
	}
}

// CheckDeadAll 定时来处理过期的Session
func (T *Sessions) CheckDeadAll() []any {
	if T.Expired == 0 {
		return nil
	}

	type expiredEntry struct {
		id any
		ms *manageSession
	}
	var expired []expiredEntry

	currTime := time.Now()
	T.ss.Range(func(id, mse any) bool {
		ms := mse.(*manageSession)
		if ms.unavailable.Load() {
			return true
		}
		recentTime := time.Unix(0, ms.recent.Load()).Add(T.Expired)
		if currTime.After(recentTime) {
			ms.unavailable.Store(true)
			expired = append(expired, expiredEntry{id: id, ms: ms})
			go ms.s.Free()
		}
		return true
	})

	if len(expired) == 0 {
		return nil
	}

	expID := make([]any, 0, len(expired))
	for _, e := range expired {
		if T.deleteIfSame(e.id, e.ms) {
			expID = append(expID, e.id)
		}
	}
	return expID
}

// deleteIfSame 仅当 id 当前映射到指定的 manageSession 时才从会话表中删除。
// 调用约定：调用方应已对 ms 执行 unavailable.Store(true)。
// GetHas → Del 两步之间存在窗口；SetSession 并发替换后 Del 会误删新条目。
func (T *Sessions) deleteIfSame(id any, ms *manageSession) bool {
	cur, ok := T.ss.GetHas(id)
	if !ok {
		return false
	}
	curMS, ok := cur.(*manageSession)
	if !ok || curMS != ms {
		return false
	}
	T.ss.Del(id)
	return true
}

// triggerDeadSession 由用户来触发，并删除已挂载入的Defer
func (T *Sessions) triggerDeadSession(ms *manageSession) (ok bool) {
	if ms.unavailable.Load() {
		return true
	}
	if T.Expired != 0 {
		currTime := time.Now()
		recentTime := time.Unix(0, ms.recent.Load()).Add(T.Expired)
		if currTime.After(recentTime) {
			ms.unavailable.Store(true)
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
	// 零值/负值回退到 64 字节，保证生成的 ID 有足够熵并避免 panic。
	size := T.Size
	if size <= 0 {
		size = 64
	}
	rnd := make([]byte, size)
	err := GenerateRandomID(rnd)
	if err != nil {
		panic(err)
	}
	if T.Salt != "" {
		return AddSalt(rnd, T.Salt)
	}
	id := fmt.Sprintf("%x", rnd)
	return id[:size]
}

// SessionID 从请求中读取会话标识
func (T *Sessions) SessionID(req *http.Request) (id string, err error) {
	if req == nil {
		return "", fmt.Errorf("vweb: 请求为空，无法读取会话ID")
	}
	if T.Name == "" {
		return "", fmt.Errorf("vweb: 会话名称(Cookie名)不能为空")
	}
	c, err := req.Cookie(T.Name)
	if err != nil || c == nil || c.Value == "" {
		return "", fmt.Errorf("vweb: 该用会话属性（%s）名称，从客户端请求中没有找可用ID值。", T.Name)
	}
	return c.Value, nil
}

// NewSession 新建会话
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

	ms, ok := mse.(*manageSession)
	if !ok || ms == nil {
		return nil, false
	}
	if ms.unavailable.Load() {
		return nil, false
	}

	if T.triggerDeadSession(ms) {
		// triggerDeadSession 已标记 unavailable 并 go Free，这里只负责身份校验删除
		T.deleteIfSame(id, ms)
		return nil, false
	}
	ms.recent.Store(time.Now().UnixNano())
	return ms.s, true
}

// SetSession 使用id写入新的会话
func (T *Sessions) SetSession(id string, s Sessioner) Sessioner {
	if s == nil {
		return nil
	}
	if inf, ok := T.ss.GetHas(id); ok {
		if ms, ok := inf.(*manageSession); ok && ms != nil && ms.s != nil {
			// 在执行ProcessDeadAll时候，已经过期了，只是还没有被删除
			// 所以检测有没有过期，否则不能读取返回
			if !ms.unavailable.Load() {
				if ms.s.Token() == s.Token() {
					return ms.s
				}
				// 设置为不可用,防止被读取
				ms.unavailable.Store(true)
				// 替换原有Session，需要清理原有的defer
				go ms.s.Free()
			}
		}
	}
	if t, can := s.(*Session); can {
		// 对应这个id，并保存
		t.id = id
	}
	ms := &manageSession{s: s}
	ms.recent.Store(time.Now().UnixNano())
	T.ss.Set(id, ms)
	return s
}

// DelSession 使用id删除的会话
//
//	id string   id标识符
func (T *Sessions) DelSession(id string) {
	inf, ok := T.ss.GetHas(id)
	if !ok {
		return
	}
	ms, ok := inf.(*manageSession)
	if !ok || ms == nil {
		return
	}

	// 先标记不可用：阻断 GetSession 继续读到该条目，也告知 SetSession 无需再 Free
	ms.unavailable.Store(true)

	// 仅当 id 仍指向 ms 时才删除，避免误删并发 SetSession 写入的新会话
	T.deleteIfSame(id, ms)

	// Free 幂等；若 SetSession 已 Free 过旧条目，这里也会立即返回
	go ms.s.Free()
}

// writeToClient 写入到客户端
func (T *Sessions) writeToClient(rw http.ResponseWriter, req *http.Request) Sessioner {
	if T.Name == "" {
		return nil
	}
	wh := rw.Header()

	// 1. 请求 Cookie 里已有有效会话：复用，并清理响应头里会覆盖它的同名 Set-Cookie
	if req != nil {
		if id, err := T.SessionID(req); err == nil {
			if s, ok := T.GetSession(id); ok {
				stripSetCookieByName(wh, T.Name)
				return s
			}
		}
	}

	// 2. 响应头里已有有效的同名 Set-Cookie（兼容原行为）
	for _, c := range readSetCookies(wh) {
		if c.Name != T.Name {
			continue
		}
		if s, ok := T.GetSession(c.Value); ok {
			return s
		}
	}

	// 3. 生成新会话
	id := T.generateRandSessionID()
	cookie := &http.Cookie{Name: T.Name, Value: id, Path: "/", HttpOnly: true}
	if req != nil && req.TLS != nil {
		cookie.Secure = true
	}
	T.replaceSetCookie(wh, cookie.String())
	return T.SetSession(id, new(Session))
}

func stripSetCookieByName(wh http.Header, name string) {
	vals := wh.Values("Set-Cookie")
	wh.Del("Set-Cookie")
	for _, v := range vals {
		if cookieNameFromSetCookie(v) == name {
			continue
		}
		wh.Add("Set-Cookie", v)
	}
}

// replaceSetCookie 移除 header 中所有与 newCookie 同名的 Set-Cookie 后追加 newCookie。
// 用于避免同一响应中重复下发同名 Cookie。
func (T *Sessions) replaceSetCookie(wh http.Header, newCookie string) {
	vals := wh.Values("Set-Cookie")
	wh.Del("Set-Cookie")
	for _, v := range vals {
		if cookieNameFromSetCookie(v) == T.Name {
			continue
		}
		wh.Add("Set-Cookie", v)
	}
	wh.Add("Set-Cookie", newCookie)
}

// cookieNameFromSetCookie 从 Set-Cookie 头值中提取 Cookie 名称。
// 提取失败时返回空字符串（不匹配任何合法名称，因而不会被误删）。
func cookieNameFromSetCookie(v string) string {
	if semi := strings.IndexByte(v, ';'); semi >= 0 {
		v = v[:semi]
	}
	eq := strings.IndexByte(v, '=')
	if eq <= 0 {
		return ""
	}
	return strings.TrimSpace(v[:eq])
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
func (T *Sessions) Session(rw http.ResponseWriter, req *http.Request) Sessioner {
	return T.writeToClient(rw, req)
}
