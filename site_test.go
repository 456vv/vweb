package vweb

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/issue9/assert/v4"
)

func TestNewSitePool(t *testing.T) {
	at := assert.New(t, true)

	sp := NewSitePool()
	defer sp.Close()

	siteA := sp.NewSite("A")
	at.Equal(siteA.PoolName(), "A")

	siteB := sp.NewSite("B")
	at.Equal(siteB.PoolName(), "B")

	sp.DelSite("A")             // 这里删除了
	siteANew := sp.NewSite("A") // 创建新的
	at.NotSame(siteANew, siteA)
}

func Test_Site_SetSite(t *testing.T) {
	sitePool := NewSitePool()
	defer sitePool.Close()

	site := sitePool.NewSite("A")

	siteMan := SiteMan{}
	siteMan.Add("host", site)
	if _, ok := siteMan.Get("host"); !ok {
		t.Fatal("无法往池中增加站点")
	}

	// 写入池
	siteMan.Add("host", nil) // 删除
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
	defer sitePool.Close()
	site := sitePool.NewSite("A")

	siteMan := SiteMan{}
	siteMan.Add("*.x.com:80", site)

	if _, ok := siteMan.Get("aaaaa.x.com:80"); !ok {
		t.Fatal("无法往池中增加站点")
	}

	// 写入池
	siteMan.Add("*.x.com:80", nil) // 删除
	if _, ok := siteMan.Get("aaaaa.x.com:80"); ok {
		t.Fatal("无法从池中删除站点")
	}

	siteMan.Add("*.x.com:80", site)
	if _, ok := siteMan.Get("bbbbbb.x.com:80"); !ok {
		t.Fatal("无法往池中增加站点")
	}
}

func Test_Site_Start(t *testing.T) {
	// 创建池并设置刷新时间
	sitePool := NewSitePool()
	defer sitePool.Close()
	sitePool.SetRecoverSession(time.Second * 2)

	site := sitePool.NewSite("A")
	site.sessions.Expired = time.Second

	// 生成会话
	rw := httptest.NewRecorder()
	r := new(http.Request)
	site.sessions.Session(rw, r)

	if site.sessions.Len() != 1 {
		t.Fatal("无法增加会话")
	}
	time.Sleep(time.Second * 4)
	if site.sessions.Len() != 0 {
		t.Fatal("无法删除过期会话")
	}
}

func Test_Site_SetRecoverSession(t *testing.T) {
	// 创建池并设置刷新时间
	sitePool := NewSitePool()
	defer sitePool.Close()

	site := sitePool.NewSite("A")
	site.sessions.Expired = time.Second * 2

	// 生成会话
	rw := httptest.NewRecorder()
	r := new(http.Request)
	ok := false
	site.sessions.Session(rw, r).Defer(func() {
		ok = true
	})

	if site.sessions.Len() != 1 {
		t.Fatal("无法增加会话")
	}
	time.Sleep(time.Second * 4)
	if site.sessions.Len() != 0 {
		t.Fatal("无法删除过期会话")
	}
	if !ok {
		t.Fatal("会话过期没有调用清除函数")
	}
}

func TestSitePool_RangeSite(t *testing.T) {
	names := []string{"A", "B", "C", "D"}
	sp := NewSitePool()
	defer sp.Close()
	for _, name := range names {
		sp.NewSite(name)
	}
	sp.RangeSite(func(name string, site *Site) bool {
		if !slices.Contains(names, name) {
			t.Logf("%s 不存在", name)
			return false
		}
		return true
	})
}

func TestSitePool_Start(t *testing.T) {
	sp := NewSitePool()
	sp.SetRecoverSession(time.Second) // 默认1秒，循环检查会话
	defer sp.Close()

	site := sp.NewSite("A")
	site.sessions.Expired = time.Second * 2 // 设置2秒过期
	session := site.sessions.NewSession("a-session")
	session.Set("key", "val")

	time.Sleep(time.Second * 3) // 3秒后会话过期了
	if _, ok := site.sessions.GetSession("a-session"); ok {
		t.Fatal("error")
	}
}

func TestSiteMan(t *testing.T) {
	tests := []struct {
		host1 string
		host2 string
		want  bool
	}{
		{host1: "*.x.com", host2: "y.x.com", want: true},
		{host1: "*.x.com:443", host2: "y.x.com:443", want: true},
		{host1: "*.x.com", host2: "x.com", want: false},
		{host1: "x.com", host2: "x.com", want: true},
		{host1: "x.com", host2: "y.com", want: false},
		{host1: "x.com", host2: "x.com:80", want: false},
		{host1: "x.com:80", host2: "x.com", want: false},
	}
	sp := NewSitePool()

	for _, test := range tests {
		var sm SiteMan
		site := sp.NewSite(test.host1)
		sm.Add(test.host1, site)
		if _, ok := sm.Get(test.host2); ok != test.want {
			t.Fatalf("error int %s", test.host1)
		}
	}
}
