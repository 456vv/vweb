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

// manageSession 包装单个 Session，记录最近访问时间与是否已标记不可用。
type manageSession struct {
	s           *Session
	recent      atomic.Int64 // UnixNano，最近访问
	unavailable atomic.Bool  // 已过期/删除，禁止再 Get
}

// Sessions 管理不同用户的会话。
//
// 注意：内部含 sync.WaitGroup 与 vmap.Map，**不可按值复制**。
//
// 配置字段：
//   - Expired: 空闲过期时长（0 表示不过期）
//   - Name: Cookie 名称，如 "VID"
//   - Size: 会话 ID 字节长度（默认 64）
//   - Salt: 可选加盐，增加伪随机熵
//
// 示例：
//
//	ss := &Sessions{Expired: 20 * time.Minute, Name: "VID", Size: 64}
//	sess := ss.Session(w, r) // 自动读 Cookie 或新建并 Set-Cookie
type Sessions struct {
	Expired time.Duration // 保存 session 时间长
	Name    string        // Cookie 名
	Size    int           // 会话 ID 长度
	Salt    string        // 加盐
	ss      vmap.Map      // map[id]*manageSession
	mu      sync.RWMutex
	lastSizeWarn atomic.Int64 // 限流警告时间戳
}

// Configure 在写锁下更新配置（避免与 Get/Check 竞态）。
//
// 示例：ss.Configure(func(s *Sessions) { s.Expired = time.Hour })
func (T *Sessions) Configure(f func(*Sessions)) {
	T.mu.Lock()
	defer T.mu.Unlock()
	f(T)
}

// Len 当前会话数量。
func (T *Sessions) Len() int {
	return T.ss.Len()
}

// InstantDeadAll 立即清理全部会话，并异步执行各自的 Defer。
func (T *Sessions) InstantDeadAll() {
	T.instantDeadAll(nil)
}

// InstantDeadAllInto 与 InstantDeadAll 相同，但把每个异步 Free 注册到调用方 wg。
// SitePool.Close 用此等待最多 3 秒。
func (T *Sessions) InstantDeadAllInto(wg *sync.WaitGroup) {
	T.instantDeadAll(wg)
}

// freeWorkerLimit 限制并发执行用户 Defer 的 goroutine 上限，防止会话爆炸时 goroutine 过多。
const freeWorkerLimit = 64

// instantDeadAll 收集所有 Session，清空表，再限制并发数执行 Reset+Free。
func (T *Sessions) instantDeadAll(wg *sync.WaitGroup) {
	T.mu.Lock()
	var frees []*Session
	T.ss.Range(func(id, value any) bool {
		ms, ok := value.(*manageSession)
		if !ok || ms == nil || ms.s == nil {
			return true
		}

		if !ms.unavailable.Swap(true) {
			frees = append(frees, ms.s)
		}
		return true
	})
	T.ss.Reset()
	T.mu.Unlock()

	if len(frees) == 0 {
		return
	}

	workers := min(freeWorkerLimit, len(frees))
	ch := make(chan *Session, len(frees))
	for _, s := range frees {
		ch <- s
	}
	close(ch)

	if wg != nil {
		wg.Add(workers)
	}
	for i := 0; i < workers; i++ {
		go func() {
			if wg != nil {
				defer wg.Done()
			}
			for s := range ch {
				s.Reset()
				// freed=true 表示已禁止继续使用；
				// Free 仍负责执行 ExitCall，不能在此省略。
				s.Free()
			}
		}()
	}
}

// CheckDeadAll 定时检查并清理过期会话；返回被删除的 id 列表。
// 在 Range 内只标记，Range 外再 Free，避免用户 Defer 回调死锁。
func (T *Sessions) CheckDeadAll() []any {
	T.mu.RLock()
	expiredAfter := T.Expired
	T.mu.RUnlock()
	if expiredAfter == 0 {
		return nil
	}

	type expiredEntry struct {
		id any
		ms *manageSession
	}
	var (
		expired []expiredEntry
		toFree  []*Session
	)
	currTime := time.Now()

	// Range 内只做标记与收集，不执行用户代码。
	// 用户在 Defer 里若回调 Sessions 方法会与 vmap.Map.Range 内部锁或后续 Lock 争用，
	// 且会延长 Range 持锁时间，扩大延迟。
	T.ss.Range(func(id, mse any) bool {
		ms := mse.(*manageSession)
		if ms.unavailable.Load() {
			expired = append(expired, expiredEntry{id: id, ms: ms})
			return true
		}
		if currTime.After(time.Unix(0, ms.recent.Load()).Add(expiredAfter)) {
			if !ms.unavailable.Swap(true) {
				toFree = append(toFree, ms.s)
			}
			expired = append(expired, expiredEntry{id: id, ms: ms})
		}
		return true
	})

	// Range 结束后统一 Free。
	for _, s := range toFree {
		s.Reset()
		s.Free()
	}

	if len(expired) == 0 {
		return nil
	}

	T.mu.Lock()
	defer T.mu.Unlock()

	expID := make([]any, 0, len(expired))
	for _, e := range expired {
		if T.deleteIfSame(e.id, e.ms) {
			expID = append(expID, e.id)
		}
	}
	return expID
}

// deleteIfSame 仅当 id 仍指向同一 manageSession 时删除，避免误删并发 Set 的新会话。
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

// generateSessionID 生成会话标识符（crypto 随机 + 可选 Salt）。
// Size<=0 时回退 64。
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

// SessionID 从请求 Cookie 中读取会话标识。
//
// 返回：id 字符串，或错误（请求为空、Cookie 名未配置、客户端无该 Cookie）
func (T *Sessions) SessionID(req *http.Request) (id string, err error) {
	if req == nil {
		return "", fmt.Errorf("vweb: 请求为空，无法读取会话ID")
	}

	T.mu.RLock()
	defer T.mu.RUnlock()

	if T.Name == "" {
		return "", fmt.Errorf("vweb: 会话名称(Cookie名)不能为空")
	}
	c, err := req.Cookie(T.Name)
	if err != nil || c == nil || c.Value == "" {
		return "", fmt.Errorf("vweb: 该用会话属性（%s）名称，从客户端请求中没有找可用ID值。", T.Name)
	}
	return c.Value, nil
}

// NewSession 按 id 新建或返回已有会话。
func (T *Sessions) NewSession(id string) *Session {
	T.mu.Lock()
	if s, ok := T.getSessionLocked(id); ok {
		T.mu.Unlock()
		return s
	}
	s, toFree := T.setSessionLocked(id, new(Session))
	T.mu.Unlock()
	if toFree != nil {
		toFree.Reset()
		toFree.Free()
	}
	return s
}

// GetSession 使用 id 读取会话；过期则标记并异步 Free，返回 (nil, false)。
// 成功时会刷新 recent 时间戳。
func (T *Sessions) GetSession(id string) (*Session, bool) {
	T.mu.RLock()
	defer T.mu.RUnlock()

	return T.getSessionLocked(id)
}

func (T *Sessions) getSessionLocked(id string) (*Session, bool) {
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

	if T.Expired != 0 {
		dead, toFree := T.triggerDeadSession(ms, T.Expired)
		if dead {
			if toFree != nil {
				go func(s *Session) {
					s.Reset()
					s.Free()
				}(toFree)
			}
			return nil, false
		}
		ms.recent.Store(time.Now().UnixNano())
	}
	return ms.s, true
}

func (T *Sessions) triggerDeadSession(ms *manageSession, expired time.Duration) (dead bool, toFree *Session) {
	if ms.unavailable.Load() {
		return true, nil
	}
	if time.Now().Before(time.Unix(0, ms.recent.Load()).Add(expired)) {
		return false, nil
	}
	if !ms.unavailable.Swap(true) {
		return true, ms.s
	}
	return true, nil
}

// SetSession 写入会话；若同 id 已有不同 Token 的旧会话，则标记旧会话待 Free。
func (T *Sessions) SetSession(id string, s *Session) *Session {
	T.mu.Lock()
	result, toFree := T.setSessionLocked(id, s)
	T.mu.Unlock()
	if toFree != nil {
		toFree.Reset()
		toFree.Free()
	}
	return result
}

func (T *Sessions) setSessionLocked(id string, s *Session) (result, toFree *Session) {
	if s == nil {
		return nil, nil
	}

	if inf, ok := T.ss.GetHas(id); ok {
		if ms, ok := inf.(*manageSession); ok && ms != nil && ms.s != nil {
			if !ms.unavailable.Load() {
				if ms.s.Token() == s.Token() {
					// 同 token 复用
					return ms.s, nil
				}
				// 不同 token：旧会话标记不可用，延迟到锁外 Free
				if !ms.unavailable.Swap(true) {
					toFree = ms.s
				}
			}
		}
	}

	s.id = id
	ms := &manageSession{s: s}
	ms.recent.Store(time.Now().UnixNano())
	T.ss.Set(id, ms)
	return s, toFree
}

// DelSession 删除指定 id 的会话并 Free。
func (T *Sessions) DelSession(id string) {
	T.mu.Lock()
	defer T.mu.Unlock()

	inf, ok := T.ss.GetHas(id)
	if !ok {
		return
	}
	ms, ok := inf.(*manageSession)
	if !ok || ms == nil {
		return
	}

	// 先标记不可用：阻断 GetSession 继续读到该条目，也告知 SetSession 无需再 Free
	if !ms.unavailable.Swap(true) {
		// 仅当 id 仍指向 ms 时才删除，避免误删并发 SetSession 写入的新会话
		T.deleteIfSame(id, ms)

		// Free 幂等；若 SetSession 已 Free 过旧条目，这里也会立即返回
		ms.s.Reset()
		ms.s.Free()
	}
}

// Session 是面向 Handler 的便捷入口：
// 1. 请求 Cookie 有效 → 复用；
// 2. 响应头已有同名 Set-Cookie → 复用；
// 3. 否则新建随机会话并写入 Set-Cookie。
//
// 返回 Sessioner；Name 为空时返回 nil。
func (T *Sessions) Session(rw http.ResponseWriter, req *http.Request) Sessioner {
	T.mu.RLock()
	name := T.Name
	T.mu.RUnlock()

	if name == "" {
		return nil
	}
	wh := rw.Header()

	// 1. 请求 Cookie 里已有有效会话：复用，并清理响应头里会覆盖它的同名 Set-Cookie
	if req != nil {
		if id, err := T.SessionID(req); err == nil {
			if s, ok := T.GetSession(id); ok {
				stripSetCookieByName(wh, name)
				return s
			}
		}
	}

	// 2. 响应头里已有有效的同名 Set-Cookie（兼容原行为）
	for _, c := range readSetCookies(wh) {
		if c.Name != name {
			continue
		}
		if s, ok := T.GetSession(c.Value); ok {
			return s
		}
	}

	// 3. 生成新会话
	s := T.newRandomSession()
	cookie := &http.Cookie{Name: name, Value: s.Token(), Path: "/", HttpOnly: true}
	if req != nil && req.TLS != nil {
		cookie.Secure = true
	}
	replaceSessionCookie(wh, name, cookie.String())
	return s
}

// newRandomSession 生成不冲突的会话 ID（最多重试 8 次，失败则用时间戳兜底并限流警告）。
func (T *Sessions) newRandomSession() *Session {
	const maxRetry = 8
	for i := 0; i < maxRetry; i++ {
		T.mu.Lock()
		id := T.generateSessionID()
		if T.ss.Has(id) {
			T.mu.Unlock()
			time.Sleep(time.Millisecond)
			continue
		}
		s, toFree := T.setSessionLocked(id, new(Session))
		T.mu.Unlock()
		if toFree != nil {
			toFree.Reset()
			toFree.Free()
		}
		return s
	}

	// 重试耗尽：用纳秒时间戳 + 逐次追加保证唯一。
	T.mu.Lock()
	id := fmt.Sprintf("%s-%x", T.generateSessionID(), time.Now().UnixNano())
	for T.ss.Has(id) {
		id += "-x"
	}
	size := T.Size
	s, toFree := T.setSessionLocked(id, new(Session))
	T.mu.Unlock()
	if toFree != nil {
		toFree.Reset()
		toFree.Free()
	}
	// 限流：首次立刻打印，之后每 1 分钟最多一次。
	now := time.Now().UnixNano()
	const warnInterval = int64(time.Minute)
	if last := T.lastSizeWarn.Load(); now-last >= warnInterval {
		if T.lastSizeWarn.CompareAndSwap(last, now) {
			fmt.Printf("vweb: 警告>>会话ID空间耗尽，请加大 Size（当前 %d），已分配临时ID(%s)\n", size, id)
		}
	}
	return s
}

func stripSetCookieByName(wh http.Header, name string) {
	vals := wh.Values("Set-Cookie")
	if len(vals) == 0 {
		return
	}
	// 仅保留同名 Cookie 的最后一个值，避免重复设置。
	// 既避免无谓的 Header 重排和内存分配，也不改变原有的 Header 顺序。
	need := false
	for _, v := range vals {
		if cookieNameFromSetCookie(v) == name {
			need = true
			break
		}
	}
	if !need {
		return
	}
	wh.Del("Set-Cookie")
	for _, v := range vals {
		if cookieNameFromSetCookie(v) == name {
			continue
		}
		wh.Add("Set-Cookie", v)
	}
}

func replaceSessionCookie(wh http.Header, name, value string) {
	// 复用已有过滤实现，避免维护两套同名 Cookie 清理代码。
	stripSetCookieByName(wh, name)
	wh.Add("Set-Cookie", value)
}

// cookieNameFromSetCookie 从 Set-Cookie 头值中提取 Cookie 名称。
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
