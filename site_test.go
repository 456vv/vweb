package vweb

import (
	"testing"
    "time"
    "net/http"
    "net/http/httptest"
)


func Test_Site_Start(t *testing.T) {
	//创建池并设置刷新时间
	sitePool := NewSitePool()
    sitePool.SetRecoverSession(time.Second*3)

    //新建一个站点并设置会话过期时间
    site := NewSite()
    site.Sessions.Expired = time.Second*2

    //生成会话
    rw := httptest.NewRecorder()
    r := &http.Request{}
    site.Sessions.Session(rw, r)

    //写入池
    sitePool.Pool.Set("A", site)

    if site.Sessions.sessions.Len() == 0 {
        t.Fatal("无法增加会话")
    }
    
    go func(){
        time.Sleep(time.Second)
        //更新池判断会话过期时间
   		sitePool.SetRecoverSession(time.Second*4)

        time.Sleep(time.Second*5)
        sitePool.Close()
    }()
    err := sitePool.Start()
    if err != nil {
    	t.Fatal(err)
    }
    if site.Sessions.sessions.Len() != 0 {
        t.Fatal("无法删除过期的会话")
    }
}
