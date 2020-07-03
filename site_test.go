package vweb

import (
	"testing"
    "time"
    "net/http"
    "net/http/httptest"
)



func Test_Site_SetSite(t *testing.T) {
	sitePool := NewSitePool()
    site := sitePool.NewSite("A")
    
    siteMan := SiteMan{}
    siteMan.Add("host", site)
    if _, ok := siteMan.Get("host"); !ok {
    	t.Fatal("无法往池中增加站点")
    }
    
    //写入池
    siteMan.Add("host", nil) //删除
    if _, ok := siteMan.Get("host"); ok {
    	t.Fatal("无法从池中删除站点")
    }
	
    siteMan.Add("host", site)
    if _, ok := siteMan.Get("host"); !ok {
    	t.Fatal("无法往池中增加站点")
    }
}

func Test_Site_GetSite(t *testing.T) {
	sitePool := NewSitePool()
    site := sitePool.NewSite("A")
	
    siteMan := SiteMan{}
    siteMan.Add("*.vweb.com:80", site)
    
    if _, ok := siteMan.Get("aaaaa.vweb.com:80"); !ok {
    	t.Fatal("无法往池中增加站点")
    }
    
    //写入池
    siteMan.Add("*.vweb.com:80", nil) //删除
    if _, ok := siteMan.Get("aaaaa.vweb.com:80"); ok {
    	t.Fatal("无法从池中删除站点")
    }
	
    siteMan.Add("*.vweb.com:80", site)
    if _, ok := siteMan.Get("bbbbbb.vweb.com:80"); !ok {
    	t.Fatal("无法往池中增加站点")
    }
}


func Test_Site_Start(t *testing.T) {
	//创建池并设置刷新时间
	sitePool := NewSitePool()
    sitePool.SetRecoverSession(time.Second*2)
	sitePool.Start()
	defer sitePool.Close()
    site := sitePool.NewSite("A")
    site.Sessions.Expired = time.Second
    
    //生成会话
    rw := httptest.NewRecorder()
    r := &http.Request{}
    site.Sessions.Session(rw, r)
	
    if site.Sessions.Len() != 1 {
        t.Fatal("无法增加会话")
    }
    time.Sleep(time.Second*4)
    if site.Sessions.Len() != 0 {
        t.Fatal("无法删除过期会话")
    }
}

func Test_Site_SetRecoverSession(t *testing.T) {
	//创建池并设置刷新时间
	sitePool := NewSitePool()
	sitePool.Start()
	defer sitePool.Close()
	
    site := sitePool.NewSite("A")
    site.Sessions.Expired = time.Second*2
    
    //生成会话
    rw := httptest.NewRecorder()
    r := &http.Request{}
    site.Sessions.Session(rw, r).Defer(func(){
    	t.Log("ok")
    })
	
    if site.Sessions.Len() != 1 {
        t.Fatal("无法增加会话")
    }
    time.Sleep(time.Second*4)
    if site.Sessions.Len() != 0 {
        t.Fatal("无法删除过期会话")
    }
}
