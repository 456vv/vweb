package vweb

import (
	"net/http"
	"net/http/httptest"
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
	err := ns.Defer(func(b bool) {
		cleanup <- b
	}, true)
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
	err := ns.Defer(func(b bool) {
		cleanup <- b
	}, true)
	if err != nil {
		t.Fatal(err)
	}

	mse := nss.ss.Get("A").(*manageSession)
	ok := nss.triggerDeadSession(mse)
	if ok {
		t.Fatal("错误的手工判断会话已经过期。")
	}

	time.Sleep(time.Second * 1)

	ok = nss.triggerDeadSession(mse)
	if !ok {
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

		req := &http.Request{
			Header: test.header,
		}
		id, err := ss.SessionID(req)
		// 检测name
		if (err == nil) == test.nameErr {
			t.Fatalf("error in %d, name: %s, error: %v", i, test.name, err)
		}
		// 检测id
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
	session := ss.writeToClient(recorder)
	header := recorder.Header()

	cook, ok := header["Set-Cookie"]
	if !ok || len(cook) == 0 {
		t.Fatal("Cookie写入不成功")
	}

	_, ok = ss.GetSession(session.Token())
	if !ok {
		t.Fatal("Session无法存储")
	}
}

func Test_Sessions_Session(t *testing.T) {
	var ss Sessions
	ss.Name = "VID"
	ss.Size = 10
	recorder := httptest.NewRecorder()
	req := &http.Request{
		Header: http.Header{"Cookie": {"VID=A3"}},
	}

	ss.Session(recorder, req)

	header := recorder.Header()
	cook, ok := header["Set-Cookie"]
	if !ok || len(cook) == 0 {
		t.Fatal("Cookie写入不成功")
	}
}

func TestSessions_SetSession(t *testing.T) {
	ss := Sessions{
		Size: 10,
		Name: "vid",
	}
	a := new(Session)
	cleanup := make(chan bool, 1)
	a.Defer(func() {
		cleanup <- true
	})
	ss.SetSession("id", a)

	// 覆盖之前的
	b := new(Session)
	bSession := ss.SetSession("id", b)
	<-cleanup
	if unsafe.Pointer(bSession.(*Session)) != unsafe.Pointer(b) {
		t.Fatal("没有覆盖之前的会话")
	}
}

func TestSessions_Get_Get_Del(t *testing.T) {
	ss := Sessions{
		Size: 10,
		Name: "vid",
	}
	ss.SetSession("id", new(Session))
	if _, ok := ss.GetSession("id"); !ok {
		t.Fatal("不存在该id")
	}
	ss.DelSession("id")
	if _, ok := ss.GetSession("id"); ok {
		t.Fatal("该id无法删除")
	}
}
