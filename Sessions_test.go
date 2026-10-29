package vweb

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func Test_Sessions_processDeadAll(t *testing.T) {
	var nss Sessions
	nss.Expired = time.Second

	ns := new(Session)
	nss.SetSession("A", ns)

	cleanup := make(chan bool, 1)
	err := ns.Defer(func(b bool) { cleanup <- b }, true)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 2)

	nss.SetSession("B", new(Session))
	nss.CheckDeadAll()

	if nss.ss.Has("A") {
		t.Fatal("无法删除过期Session条目")
	}
	if !nss.ss.Has("B") {
		t.Fatal("误删除未过期Session条目")
	}
	<-cleanup
}

func Test_Sessions_triggerDeadSession(t *testing.T) {
	var nss Sessions
	nss.Expired = time.Second

	ns := new(Session)
	nss.SetSession("A", ns)
	cleanup := make(chan bool, 1)
	err := ns.Defer(func(b bool) { cleanup <- b }, true)
	if err != nil {
		t.Fatal(err)
	}

	mse := nss.ss.Get("A").(*manageSession)
	if nss.triggerDeadSession(mse) {
		t.Fatal("错误的手工判断会话已经过期。")
	}
	time.Sleep(time.Second * 1)
	if !nss.triggerDeadSession(mse) {
		t.Fatal("无法手工判断会话是否已经过期。")
	}
	<-cleanup
}

func Test_Sessions_generateSessionIdNoSalt(t *testing.T) {
	var nss Sessions
	nss.Size = 64
	for range 100 {
		s := nss.generateSessionID()
		if len(s) != nss.Size {
			t.Fatalf("长度非（%d）位", nss.Size)
		}
	}
}

func Test_Session_generateRandSessionId(t *testing.T) {
	var nss Sessions
	nss.Size = 64
	for range 100 {
		id := nss.generateRandSessionID()
		nss.ss.Set(id, id)
	}
	if nss.ss.Len() != 100 {
		t.Fatalf("错误长度不足")
	}
}

func Test_Sessions_SessionID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		header  http.Header
		nameErr bool
		idErr   bool
	}{
		{name: "A", id: "a", header: http.Header{"Cookie": []string{"A=a", "B=b"}}},
		{name: "B", id: "b", header: http.Header{"Cookie": {"A=b"}}, nameErr: true},
		{name: "C", id: "c", header: http.Header{"Cookie": {"C=b"}}, idErr: true},
	}
	for i, test := range tests {
		var ss Sessions
		ss.Name = test.name
		ss.SetSession(test.id, new(Session))
		req := &http.Request{Header: test.header}
		id, err := ss.SessionID(req)
		if (err == nil) == test.nameErr {
			t.Fatalf("error in %d, name: %s, error: %v", i, test.name, err)
		}
		if !test.nameErr && (test.id == id) == test.idErr {
			t.Fatalf("error in %d, name: %s", i, test.name)
		}
	}
}

func Test_Sessions_writeToClient(t *testing.T) {
	var ss Sessions
	ss.Name = "VID"
	ss.Size = 10
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "https://localhost/", nil)
	session := ss.writeToClient(recorder, req)
	header := recorder.Header()

	cook, ok := header["Set-Cookie"]
	if !ok || len(cook) == 0 {
		t.Fatal("Cookie写入不成功")
	}
	if _, ok := ss.GetSession(session.Token()); !ok {
		t.Fatal("Session无法存储")
	}
}

func Test_Sessions_Session(t *testing.T) {
	var ss Sessions
	ss.Name = "VID"
	ss.Size = 10
	recorder := httptest.NewRecorder()
	req := &http.Request{Header: http.Header{"Cookie": {"VID=A3"}}}
	ss.Session(recorder, req)
	if cook, ok := recorder.Header()["Set-Cookie"]; !ok || len(cook) == 0 {
		t.Fatal("Cookie写入不成功")
	}
}

func TestSessions_SetSession(t *testing.T) {
	ss := Sessions{Size: 10, Name: "vid"}
	a := new(Session)
	cleanup := make(chan bool, 1)
	a.Defer(func() { cleanup <- true })
	ss.SetSession("id", a)

	b := new(Session)
	bSession := ss.SetSession("id", b)
	<-cleanup
	if unsafe.Pointer(bSession.(*Session)) != unsafe.Pointer(b) {
		t.Fatal("没有覆盖之前的会话")
	}
}

func TestSessions_DelSession_SetSession_Race(t *testing.T) {
	s := &Sessions{Expired: time.Hour, Name: "VID", Size: 16}
	s.SetSession("id1", new(Session))
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.DelSession("id1") }()
		go func() { defer wg.Done(); s.SetSession("id1", new(Session)) }()
		wg.Wait()
	}
}

func TestSessions_Get_Get_Del(t *testing.T) {
	ss := Sessions{Size: 10, Name: "vid"}
	ss.SetSession("id", new(Session))
	if _, ok := ss.GetSession("id"); !ok {
		t.Fatal("不存在该id")
	}
	ss.DelSession("id")
	if _, ok := ss.GetSession("id"); ok {
		t.Fatal("该id无法删除")
	}
}

func TestSessions_SessionID_Errors(t *testing.T) {
	var ss Sessions
	ss.Name = "VID"
	if _, err := ss.SessionID(nil); err == nil {
		t.Fatal("nil req 应报错")
	}
	ss2 := Sessions{}
	if _, err := ss2.SessionID(&http.Request{}); err == nil {
		t.Fatal("空 Name 应报错")
	}
	ss3 := Sessions{Name: "VID"}
	if _, err := ss3.SessionID(&http.Request{Header: http.Header{}}); err == nil {
		t.Fatal("无 cookie 应报错")
	}
	ss4 := Sessions{Name: "VID"}
	req := &http.Request{Header: http.Header{"Cookie": {"VID="}}}
	if _, err := ss4.SessionID(req); err == nil {
		t.Fatal("空 cookie 值应报错")
	}
}

func TestSessions_SetSession_Nil(t *testing.T) {
	ss := &Sessions{Name: "VID", Size: 8}
	if got := ss.SetSession("id", nil); got != nil {
		t.Fatalf("nil session 应返回 nil，得到 %v", got)
	}
	if _, ok := ss.GetSession("id"); ok {
		t.Fatal("nil session 不应写入")
	}
}

func TestSessions_NewSession_ReturnsExisting(t *testing.T) {
	ss := &Sessions{Name: "VID", Size: 8}
	a := ss.NewSession("id1")
	a.Set("k", "v")
	b := ss.NewSession("id1")
	if a != b || b.Get("k") != "v" {
		t.Fatal("已存在 id 应返回原 session 且数据保留")
	}
}

func TestSessions_InstantDeadAll_Empty(t *testing.T) {
	ss := &Sessions{}
	ss.InstantDeadAll()
	ss.InstantDeadAllInto(nil)
	if ss.Len() != 0 {
		t.Fatal("应为空")
	}
}

func TestSessions_CheckDeadAll_NoExpired(t *testing.T) {
	ss := &Sessions{Expired: time.Hour}
	ss.SetSession("id", new(Session))
	if got := ss.CheckDeadAll(); len(got) != 0 {
		t.Fatalf("不应有过期：%v", got)
	}
	if ss.Len() != 1 {
		t.Fatal("会话应保留")
	}
}

func TestSessions_CheckDeadAll_ExpiredZero(t *testing.T) {
	ss := &Sessions{Expired: 0}
	ss.SetSession("id", new(Session))
	if got := ss.CheckDeadAll(); got != nil {
		t.Fatalf("Expired=0 应跳过，得到 %v", got)
	}
}

func TestSessions_GetExpiredSession_Removes(t *testing.T) {
	ss := &Sessions{Expired: 10 * time.Millisecond, Name: "VID", Size: 8}
	ss.SetSession("id", new(Session))
	time.Sleep(30 * time.Millisecond)
	if _, ok := ss.GetSession("id"); ok {
		t.Fatal("过期会话应不可读")
	}
	time.Sleep(50 * time.Millisecond)
	if ss.Len() != 0 {
		t.Fatalf("过期会话应被删除，剩余 %d", ss.Len())
	}
}

func TestCookieNameFromSetCookie(t *testing.T) {
	cases := []struct {
		in, want string
	}{
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

func TestSessions_generateSessionID_ZeroSize(t *testing.T) {
	ss := &Sessions{}
	if id := ss.generateSessionID(); len(id) != 64 {
		t.Fatalf("Size=0 应回退 64，得到 %d", len(id))
	}
}

// TestSessions_Boundary 过期语义、同名 Cookie 去重、零尺寸回退。
func TestSessions_Boundary(t *testing.T) {
	t.Run("Expired=0 不回收", func(t *testing.T) {
		ss := &Sessions{Expired: 0}
		ss.SetSession("id", new(Session))
		if got := ss.CheckDeadAll(); got != nil {
			t.Fatalf("Expired=0 应跳过，实得 %v", got)
		}
		if ss.Len() != 1 {
			t.Fatal("会话应保留")
		}
	})

	t.Run("过期后 GetSession 清理", func(t *testing.T) {
		ss := &Sessions{Expired: 10 * time.Millisecond, Name: "VID", Size: 8}
		ss.SetSession("id", new(Session))
		time.Sleep(30 * time.Millisecond)
		if _, ok := ss.GetSession("id"); ok {
			t.Fatal("过期会话应不可读")
		}
	})

	t.Run("writeToClient 同名 Cookie 去重", func(t *testing.T) {
		ss := &Sessions{Name: "VID", Size: 8}
		s := ss.SetSession("valid-id", new(Session))

		rw := httptest.NewRecorder()
		rw.Header().Add("Set-Cookie", "VID=other; Path=/")

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Cookie", "VID=valid-id")

		if got := ss.writeToClient(rw, req); got != s {
			t.Fatal("应返回已有 session")
		}
		for _, v := range rw.Header().Values("Set-Cookie") {
			if strings.HasPrefix(v, "VID=other") {
				t.Fatalf("同名 Cookie 应被清理：%q", v)
			}
		}
	})

	t.Run("Size=0 回退 64", func(t *testing.T) {
		var ss Sessions
		if id := ss.generateSessionID(); len(id) != 64 {
			t.Fatalf("回退长度应 64，实得 %d", len(id))
		}
	})
}
