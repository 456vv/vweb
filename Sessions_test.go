package vweb


import (
	"testing"
    "time"
    "net/http"
   "net/http/httptest"
)



func Test_Sessions_processDeadAll(t *testing.T){
    nss := Sessions{}
    nss.Expired = time.Second

    ns := &Session{}
    err := ns.Defer(t.Log, "1", "2", []string{}, "看到这里，表示Session.Defer 成功执行")
    if err != nil {
    	t.Fatal(err)
    }
    nss.SetSession("A", ns)
    time.Sleep(time.Second*2)
    
    ns1 := &Session{}
    nss.SetSession("B", ns1)
    nss.ProcessDeadAll()
    
    if nss.ss.Has("A") {
    	t.Fatal("无法删除过期Session条目")
    }
    if !nss.ss.Has("B") {
    	t.Fatal("误删除未过期Session条目")
    }
    time.Sleep(time.Second)
}


func Test_Sessions_triggerDeadSession(t *testing.T){

    nss := Sessions{}
    nss.Expired=time.Second

    ns := &Session{}
    err := ns.Defer(t.Log, "1", "2", []string{}, "看到这里，表示Session.Defer 成功执行")
    if err != nil {
    	t.Fatal(err)
    }
    nss.SetSession("A", ns)
	mse := nss.ss.Get("A").(*manageSession)
    ok := nss.triggerDeadSession(mse)
    if ok {
    	t.Fatal("错误的手工判断会话已经过期。")
    }

    time.Sleep(time.Second*1)

    ok = nss.triggerDeadSession(mse)
    if !ok {
    	t.Fatal("无法手工判断会话是否已经过期。")
    }
    time.Sleep(time.Second)

}

func Test_Sessions_generateSessionIdSalt_1(t *testing.T){
    nss := Sessions{}
    nss.Size=64
    nss.Salt="1234567890qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM"
    for i:=0;i<1000;i++ {
        s := nss.generateSessionIdSalt()
        if len(s) != nss.Size {
            t.Fatalf("长度非（%d）位", nss.Size)
        }
    }
}

func Test_Sessions_generateSessionIdSalt_2(t *testing.T){
	ss := Sessions{}
	salt := ss.generateSessionIdSalt()
	if salt != "" {
		t.Fatalf("发生错误，预定为(), 返回为(%s)", salt)
	}
}



func Test_Sessions_generateSessionIdNoSalt_1(t *testing.T){
    nss := Sessions{}
    nss.Size=64
    for i:=0;i<1000;i++ {
        s := nss.generateSessionId()
        if len(s) != nss.Size {
            t.Fatalf("长度非（%d）位", nss.Size)
        }
    }
}

func Test_Sessions_generateSessionIdNoSalt_2(t *testing.T){
	ss := Sessions{}
	salt := ss.generateSessionId()
	if salt != "" {
		t.Fatalf("发生错误，预定为(), 返回为(%s)", salt)
	}
}

func Test_Session_generateRandSessionId(t *testing.T){
	nss := Sessions{}
	nss.Size=64
	nss.Salt=""
	for i:=0;i<100;i++{
		id := nss.generateRandSessionId()
		nss.ss.Set(id, id)
	}
	if nss.ss.Len() != 100 {
		t.Fatalf("错误长度不足")
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
        ss := Sessions{}
        ss.Name=test.name
        ss.SetSession(test.id, &Session{})
        req := &http.Request{
            Header: test.header,
        }
        id, err := ss.SessionId(req)
        if err != nil && !test.err {
        	t.Fatal(err)
        }
        _, ok := ss.GetSession(id)
        if !ok && !test.err {
        	t.Fatal("Error")
        }
    }
}

func Test_Sessions_writeToClient(t *testing.T){

    ss := Sessions{}
    ss.Name = "VID"
    recorder := httptest.NewRecorder()
    ss.writeToClient(recorder, "A")
    header := recorder.Header()
    cook, ok := header["Set-Cookie"]
    if !ok || len(cook) == 0 {
    	t.Fatal("Cookie写入不成功")
    }
    _, ok = ss.GetSession("A")
    if !ok {
    	t.Fatal("Session无法存储")
    }
}
func Test_Sessions_Session(t *testing.T){
    ss := Sessions{}
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






