package vweb

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// waitClosed 用超时等待 closed，避免 Defer 未执行导致测试永久挂起。
func waitClosed(t *testing.T, closed <-chan struct{}, d time.Duration, msg string) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(d):
		t.Fatal(msg)
	}
}

// TestSessions_SessionID 表格驱动：覆盖正常读取、名称不匹配、无会话、nil req、空 Name、无 Cookie、空 Cookie。
func TestSessions_SessionID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		sessName string
		setID    string
		req      *http.Request
		wantID   string
		wantErr  bool
	}{
		{name: "Cookie 名不匹配", sessName: "B", setID: "b", req: &http.Request{Header: http.Header{"Cookie": {"A=b"}}}, wantErr: true},
		{name: "Cookie 名匹配但无会话", sessName: "C", setID: "c", req: &http.Request{Header: http.Header{"Cookie": {"C=b"}}}, wantID: "b", wantErr: false},
		{name: "无会话", sessName: "A", req: &http.Request{Header: http.Header{"Cookie": {"A=a"}}}, wantID: "a", wantErr: false},
		{name: "正常读取", sessName: "A", setID: "a", req: &http.Request{Header: http.Header{"Cookie": {"A=a"}}}, wantID: "a", wantErr: false},
		{name: "空 Cookie 值", sessName: "VID", wantID: "", req: &http.Request{Header: http.Header{"Cookie": {"VID="}}}, wantErr: true},
		{name: "无 Cookie", sessName: "VID", req: &http.Request{Header: http.Header{}}, wantErr: true},
		{name: "nil 请求", sessName: "VID", req: nil, wantErr: true},
		{name: "空 Name", sessName: "", wantID: "", req: &http.Request{}, wantErr: true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ss := &Sessions{Name: c.sessName, Size: 8}
			if c.setID != "" {
				ss.SetSession(c.setID, new(Session))
			}
			id, err := ss.SessionID(c.req)
			if (err != nil) != c.wantErr {
				t.Fatalf("name=%s, err=%v, wantErr=%v", c.name, err, c.wantErr)
			}
			if !c.wantErr && id != c.wantID {
				t.Fatalf("name=%s, id=%q, want %q", c.name, id, c.wantID)
			}
		})
	}
}

// TestSessions_triggerDeadSession 合并原两个触发过期用例：首次返回 toFree，二次不再返回。
func TestSessions_triggerDeadSession(t *testing.T) {
	t.Parallel()
	ss := &Sessions{Expired: 10 * time.Millisecond, Name: "VID", Size: 8}
	ss.NewSession("id")
	ms := ss.ss.Get("id").(*manageSession)

	if dead, toFree := ss.triggerDeadSession(ms, ss.Expired); dead || toFree != nil {
		t.Fatalf("未过期时死=%v toFree=%v", dead, toFree)
	}

	time.Sleep(20 * time.Millisecond)

	dead, toFree := ss.triggerDeadSession(ms, ss.Expired)
	if !dead || toFree == nil {
		t.Fatalf("首次过期应返回 toFree，得到 dead=%v toFree=%v", dead, toFree)
	}
	if dead, toFree = ss.triggerDeadSession(ms, ss.Expired); !dead || toFree != nil {
		t.Fatalf("二次调用不应重复返回 toFree，得到 dead=%v toFree=%v", dead, toFree)
	}
}

// TestSessions_generateSessionID 合并长度、唯一性、Size=0 回退、newRandomSession。
func TestSessions_generateSessionID(t *testing.T) {
	t.Parallel()
	t.Run("长度与唯一性", func(t *testing.T) {
		ss := &Sessions{Size: 64}
		seen := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			id := ss.generateSessionID()
			if len(id) != ss.Size {
				t.Fatalf("长度 %d != %d", len(id), ss.Size)
			}
			if _, ok := seen[id]; ok {
				t.Fatalf("重复 ID: %s", id)
			}
			seen[id] = struct{}{}
		}
	})
	t.Run("Size=0 回退 64", func(t *testing.T) {
		ss := &Sessions{}
		if id := ss.generateSessionID(); len(id) != 64 {
			t.Fatalf("回退长度 64，实得 %d", len(id))
		}
	})
	t.Run("newRandomSession 唯一性", func(t *testing.T) {
		ss := &Sessions{Size: 8}
		seen := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			id := ss.newRandomSession().Token()
			if _, ok := seen[id]; ok {
				t.Fatalf("重复 ID: %s", id)
			}
			seen[id] = struct{}{}
		}
		if ss.Len() != 100 {
			t.Fatalf("会话数不足：%d", ss.Len())
		}
	})
}

// TestSessions_DelGet 合并 Get/Get/Del 与 NewSession 复用。
func TestSessions_DelGet(t *testing.T) {
	t.Parallel()
	ss := &Sessions{Name: "VID", Size: 8}
	a := ss.NewSession("id")
	a.Set("k", "v")
	if b := ss.NewSession("id"); a != b || b.Get("k") != "v" {
		t.Fatal("已存在 id 应返回原 session 且数据保留")
	}
	if _, ok := ss.GetSession("id"); !ok {
		t.Fatal("应能读到")
	}
	ss.DelSession("id")
	if _, ok := ss.GetSession("id"); ok {
		t.Fatal("删除后应读不到")
	}
}

// TestSessions_CheckDeadAll 表格驱动：无过期、Expired=0、正常过期（含 Free 锁外执行）。
func TestSessions_CheckDeadAll(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		expired    time.Duration
		sleep      time.Duration
		wantRemain int
		wantFree   bool
	}{
		{name: "无过期", expired: time.Hour, wantRemain: 1},
		{name: "Expired=0 跳过", expired: 0, wantRemain: 1},
		{name: "过期被清理", expired: 10 * time.Millisecond, sleep: 30 * time.Millisecond, wantRemain: 0, wantFree: true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ss := &Sessions{Expired: c.expired, Name: "VID", Size: 8}
			s := ss.NewSession("id")
			var freed atomic.Bool
			_ = s.Defer(func() { freed.Store(true) })
			if c.sleep > 0 {
				time.Sleep(c.sleep)
			}
			ss.CheckDeadAll()
			if ss.Len() != c.wantRemain {
				t.Fatalf("Len=%d, want %d", ss.Len(), c.wantRemain)
			}
			if c.wantFree {
				deadline := time.Now().Add(time.Second)
				for !freed.Load() && time.Now().Before(deadline) {
					time.Sleep(5 * time.Millisecond)
				}
				if !freed.Load() {
					t.Fatal("过期会话的 Defer 未被调用")
				}
			}
		})
	}
	t.Run("过期后 GetSession 立即不可读", func(t *testing.T) {
		t.Parallel()
		ss := &Sessions{Expired: 10 * time.Millisecond, Name: "VID", Size: 8}
		ss.SetSession("id", new(Session))
		time.Sleep(30 * time.Millisecond)
		if _, ok := ss.GetSession("id"); ok {
			t.Fatal("过期会话应不可读")
		}
	})
}

// TestSessions_WriteToClient 合并新会话写 Cookie 与同名 Set-Cookie 去重。
func TestSessions_WriteToClient(t *testing.T) {
	t.Parallel()
	t.Run("新会话写入 Cookie", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 10}
		rw := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "https://localhost/", nil)
		s := ss.Session(rw, req)
		if cook, ok := rw.Header()["Set-Cookie"]; !ok || len(cook) == 0 {
			t.Fatal("Cookie 写入不成功")
		}
		if _, ok := ss.GetSession(s.Token()); !ok {
			t.Fatal("Session 无法存储")
		}
	})
	t.Run("同名 Set-Cookie 去重", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		s := ss.SetSession("valid-id", new(Session))
		rw := httptest.NewRecorder()
		rw.Header().Add("Set-Cookie", "VID=other; Path=/")
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Cookie", "VID=valid-id")
		if got := ss.Session(rw, req); got != s {
			t.Fatal("应返回已有 session")
		}
		for _, v := range rw.Header().Values("Set-Cookie") {
			if strings.HasPrefix(v, "VID=other") {
				t.Fatalf("同名 Cookie 应被清理：%q", v)
			}
		}
	})
}

// TestSessions_InstantDeadAll 合并空池与大批量。
func TestSessions_InstantDeadAll(t *testing.T) {
	t.Parallel()
	t.Run("空池", func(t *testing.T) {
		ss := &Sessions{}
		ss.InstantDeadAll()
		ss.InstantDeadAllInto(nil)
		if ss.Len() != 0 {
			t.Fatal("应为空")
		}
	})
	t.Run("大批量", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		const n = 1000
		var count atomic.Int32
		for i := 0; i < n; i++ {
			s := ss.NewSession(fmt.Sprintf("id-%d", i))
			if err := s.Defer(func() { count.Add(1) }); err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		ss.InstantDeadAllInto(&wg)
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("InstantDeadAll 超时")
		}
		if got := count.Load(); got != int32(n) {
			t.Fatalf("应执行 %d 次 Defer，实际 %d", n, got)
		}
		if ss.Len() != 0 {
			t.Fatalf("会话应全部清空，实际 %d", ss.Len())
		}
	})
}

// TestSessions_FreeOutsideLock 验证 Defer 回调 Sessions 方法不死锁（Free 必须在锁外执行）。
func TestSessions_FreeOutsideLock(t *testing.T) {
	t.Parallel()
	t.Run("过期触发", func(t *testing.T) {
		ss := &Sessions{Expired: 10 * time.Millisecond, Name: "VID", Size: 8}
		s := ss.NewSession("id")
		done := make(chan struct{})
		if err := s.Defer(func() { ss.DelSession("id"); close(done) }); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		if _, ok := ss.GetSession("id"); ok {
			t.Fatal("过期会话应不可读")
		}
		waitClosed(t, done, 2*time.Second, "过期路径 Defer 在锁内执行，导致死锁")
	})
	t.Run("SetSession 替换", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		a := ss.NewSession("id")
		done := make(chan struct{})
		if err := a.Defer(func() { ss.DelSession("other"); close(done) }); err != nil {
			t.Fatal(err)
		}
		ss.SetSession("id", new(Session))
		waitClosed(t, done, 2*time.Second, "SetSession 中 Free 在锁内执行，导致死锁")
	})
	t.Run("CheckDeadAll 不在 Range 内 Free", func(t *testing.T) {
		ss := &Sessions{Expired: 10 * time.Millisecond}
		const n = 10
		var count atomic.Int32
		for i := 0; i < n; i++ {
			s := ss.NewSession(fmt.Sprintf("id-%d", i))
			if err := s.Defer(func() { ss.Len(); count.Add(1) }); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(30 * time.Millisecond)
		if ids := ss.CheckDeadAll(); len(ids) != n {
			t.Fatalf("应删除 %d 条，实际 %d", n, len(ids))
		}
		if got := count.Load(); got != int32(n) {
			t.Fatalf("应执行 %d 次 Defer，实际 %d", n, got)
		}
	})
}

// TestSessions_newRandomSession_Conflict 合并冲突下唯一性与兜底触发。
func TestSessions_newRandomSession_Conflict(t *testing.T) {
	t.Parallel()
	ss := &Sessions{Size: 1} // 只产生 64 种 ID，冲突概率极高
	const n = 200
	seen := make(map[string]struct{}, n)
	base := make(map[string]struct{}, len(encodeStd))
	for i := 0; i < len(encodeStd); i++ {
		base[string(encodeStd[i])] = struct{}{}
	}
	for i := 0; i < n; i++ {
		id := ss.newRandomSession().Token()
		if _, ok := seen[id]; ok {
			t.Fatalf("重复 ID: %s", id)
		}
		seen[id] = struct{}{}
	}
	// 预占满所有基础 ID，强制走兜底
	for id := range base {
		ss.ss.Set(id, &manageSession{s: &Session{id: id}})
	}
	a := ss.newRandomSession().Token()
	b := ss.newRandomSession().Token()
	if a == b {
		t.Fatalf("兜底 ID 不唯一：%s", a)
	}
	if _, isBase := base[a]; isBase {
		t.Fatalf("a=%q 命中基础 ID，未走兜底", a)
	}
	if !strings.Contains(a, "-") {
		t.Fatalf("兜底 ID 应带后缀，实际 %q", a)
	}
}

// TestSessions_SetSession 合并替换语义、setSessionLocked 返回 toFree。
func TestSessions_SetSession(t *testing.T) {
	t.Parallel()
	t.Run("替换旧会话并同步 Free", func(t *testing.T) {
		ss := &Sessions{Size: 10, Name: "vid"}
		a := new(Session)
		cleanup := make(chan struct{})
		if err := a.Defer(func() { close(cleanup) }); err != nil {
			t.Fatal(err)
		}
		ss.SetSession("id", a)

		b := new(Session)
		if got := ss.SetSession("id", b); got != b {
			t.Fatal("没有覆盖之前的会话")
		}
		waitClosed(t, cleanup, 2*time.Second, "旧会话 Defer 未执行（Free 可能未调用）")
	})
	t.Run("setSessionLocked 返回 toFree 供锁外释放", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		old := new(Session)
		ss.SetSession("id", old)
		newS := new(Session)
		result, toFree := func() (*Session, *Session) {
			ss.mu.Lock()
			defer ss.mu.Unlock()
			return ss.setSessionLocked("id", newS)
		}()
		if result != newS {
			t.Fatal("应返回新会话")
		}
		if toFree != old {
			t.Fatal("替换旧会话时应返回 toFree=旧会话")
		}
		toFree.Reset()
		toFree.Free()
	})
	t.Run("nil session 不写入", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		if got := ss.SetSession("id", nil); got != nil {
			t.Fatalf("nil session 应返回 nil，得到 %v", got)
		}
		if _, ok := ss.GetSession("id"); ok {
			t.Fatal("nil session 不应写入")
		}
	})
}

// TestSessions_ConfigureRace 合并 Name 与 Expired 的 Configure 并发读写。
func TestSessions_ConfigureRace(t *testing.T) {
	t.Parallel()
	t.Run("Name", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					ss.Configure(func(s *Sessions) { s.Name = "VID" })
				}
			}
		}()
		for i := 0; i < 100; i++ {
			rw := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			ss.Session(rw, req)
		}
		close(stop)
		wg.Wait()
	})
	t.Run("Expired", func(t *testing.T) {
		ss := &Sessions{Expired: time.Millisecond}
		ss.NewSession("id")
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					ss.Configure(func(s *Sessions) { s.Expired = time.Millisecond })
				}
			}
		}()
		for i := 0; i < 100; i++ {
			ss.CheckDeadAll()
		}
		close(stop)
		wg.Wait()
	})
}

// TestSessions_DelSetRace 综合并发：Del/Set 交替，200 轮。
// 说明：wg.Wait() 在循环内使每轮近似串行，只验证组合不泄漏 toFree、不死锁；
// 更细粒度交错需要并行 goroutine + 同步屏障，此测试不承担该职责。
func TestSessions_DelSetRace(t *testing.T) {
	t.Parallel()
	s := &Sessions{Expired: time.Hour, Name: "VID", Size: 16}
	s.SetSession("id1", new(Session))
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.DelSession("id1") }()
		go func() { defer wg.Done(); s.SetSession("id1", new(Session)) }()
		wg.Wait()
	}
}

// TestSessions_processDeadAll 端到端：过期被清理、未过期保留、Defer 被调用。
func TestSessions_processDeadAll(t *testing.T) {
	t.Parallel()
	var nss Sessions
	nss.Expired = time.Second
	ns := new(Session)
	nss.SetSession("A", ns)
	cleanup := make(chan struct{})
	if err := ns.Defer(func() { close(cleanup) }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	nss.SetSession("B", new(Session))
	nss.CheckDeadAll()
	if nss.ss.Has("A") {
		t.Fatal("无法删除过期Session条目")
	}
	if !nss.ss.Has("B") {
		t.Fatal("误删除未过期Session条目")
	}
	waitClosed(t, cleanup, 2*time.Second, "过期会话 Defer 未执行")
}

// TestCookieNameFromSetCookie 表格驱动。
func TestCookieNameFromSetCookie(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"VID=abc; Path=/", "VID"},
		{"VID=abc", "VID"},
		{"VID=", "VID"},
		{"", ""},
		{"novalue", ""},
		{"  VID=abc ; Path=/", "VID"},
	}
	for _, c := range cases {
		if got := cookieNameFromSetCookie(c.in); got != c.want {
			t.Errorf("cookieNameFromSetCookie(%q)=%q 期望 %q", c.in, got, c.want)
		}
	}
}

// TestStripSetCookieByName_NoMatch 无同名时不改变 Header 内容与顺序。
func TestStripSetCookieByName_NoMatch(t *testing.T) {
	t.Parallel()
	wh := http.Header{}
	wh.Add("Set-Cookie", "A=1; Path=/")
	wh.Add("Set-Cookie", "B=2; Path=/")
	before := wh.Values("Set-Cookie")
	stripSetCookieByName(wh, "VID")
	after := wh.Values("Set-Cookie")
	if len(before) != len(after) {
		t.Fatalf("无同名 Cookie 时不应改变 Header：before=%v after=%v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("顺序不应改变：before=%v after=%v", before, after)
		}
	}
}
