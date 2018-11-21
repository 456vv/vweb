package vweb


import (
	"testing"
    "time"
    "net/http"
//    "fmt"
    "net/http/httptest"
)

func Test_Sessions_processDeadAll(t *testing.T){
    nss := newSessions()
    nss.Expired=time.Second*3

    ns := NewSession()
    err := ns.Defer(t.Log, "1", "2", []string{}, "看到这里，表示Session.Defer 成功执行")
    if err != nil {
    	t.Fatal(err)
    }
    nss.SetSession("A", ns)

    time.Sleep(time.Second*5)

    ns1 := NewSession()
    nss.SetSession("B", ns1)
    nss.ProcessDeadAll()

    if nss.sessions.Has("A") {
    	t.Fatal("无法删除过期Session条目")
    }
    if !nss.sessions.Has("B") {
    	t.Fatal("误删除未过期Session条目")
    }
}


func Test_Sessions_triggerDeadSession(t *testing.T){

    nss := newSessions()
    nss.Expired=time.Second*3

    ns := NewSession()
    err := ns.Defer(t.Log, "1", "2", []string{}, "看到这里，表示Session.Defer 成功执行")
    if err != nil {
    	t.Fatal(err)
    }
    nss.SetSession("A", ns)
	mse := nss.sessions.Get("A").(*manageSession)
    ok := nss.triggerDeadSession(mse)
    if ok {
    	t.Fatal("错误的手工判断会话已经过期。")
    }

    time.Sleep(time.Second*5)

    ok = nss.triggerDeadSession(mse)
    if !ok {
    	t.Fatal("无法手工判断会话是否已经过期。")
    }

}

func Test_Sessions_generateSessionIdSalt(t *testing.T){
    nss := newSessions()
    nss.Size=64
    nss.Salt="1234567890qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM"
    for i:=0;i<1000;i++ {
        s := nss.GenerateSessionIdSalt()
        if len(s) != nss.Size {
            t.Fatalf("长度非（%d）位", nss.Size)
        }
    }
}

func Test_Sessions_generateSessionIdNoSalt(t *testing.T){
    nss := newSessions()
    nss.Size=64
    for i:=0;i<1000;i++ {
        s := nss.GenerateSessionId()
        if len(s) != nss.Size {
            t.Fatalf("长度非（%d）位", nss.Size)
        }
    }
}

func Test_Sessions_SessionID(t *testing.T){
    tests := []struct {
    	name    string
        id      string
        header    http.Header
        err     bool
    }{
        {name:"BW", id:"A", header:http.Header{"Cookie":[]string{"A=a;","B=b"}}, err: true},
        {name:"BW1", id:"A1", header:http.Header{"Cookie":{"BW1=A1"}}},
        {name:"BW2", id:"A2", header:http.Header{"Cookie":{"BW1=A1"}}, err: true},
        {name:"BW3", id:"A3", header:http.Header{"Cookie":{"BW3=A3;BW3=A4"}}},
        {name:"BW4", id:"A4", header:http.Header{"Cookie":{"BW4=A3;BW4=A4"}}, err: true},
    }

    for _, test := range tests {
        ss := newSessions()
        ss.Name=test.name
        ss.SetSession(test.id, NewSession())
        req := &http.Request{
            Header: test.header,
        }
        id, err := ss.SessionId(req)
        if err != nil && !test.err {
        	t.Fatal(err)
        }
        _, err = ss.GetSession(id)
        if err != nil && !test.err {
        	t.Fatal(err)
        }
    }
}

func Test_Sessions_writeToClient(t *testing.T){

    ss := newSessions()
    recorder := httptest.NewRecorder()
    ss.writeToClient(recorder, "A")
    header := recorder.Header()
    cook, ok := header["Set-Cookie"]
    if !ok || len(cook) == 0 {
    	t.Fatal("Cookie写入不成功")
    }
    _, err := ss.GetSession("A")
    if err != nil {
    	t.Fatal("Session无法存储")
    }
}
func Test_Sessions_Session(t *testing.T){
    ss := newSessions()
    ss.Name="VID"
    recorder := httptest.NewRecorder()
    req := &http.Request{
        Header: http.Header{"Cookie":{"VID=A3"}},
    }

    ss.Session(recorder, req)

    header := recorder.Header()
    cook, ok := header["Set-Cookie"]
    if !ok || len(cook) == 0 {
    	t.Fatal("Cookie写入不成功")
    }
}






