package vweb
import (
    "testing"
    "net/http"
    "net/http/httptest"
)

func Test_pluginHTTP1(t *testing.T){
 	config := ConfigSitePlugin{
        Path        : "",
        Addr        : "www.baidu.com:80",
    	Host        : "www.baidu.com",
    	Scheme		: "http",
    }
    c, err := configHTTPClient(nil, config)
    if err != nil {
    	t.Fatal(err)
    }
    phttp, err := c.Connection()
    if err != nil {
    	t.Fatal(err)
    }
    w := httptest.NewRecorder()
    r, err := http.NewRequest("GET", "http://www.baidu.com/", nil)
    if err != nil {
    	t.Fatal(err)
    }
    phttp.ServeHTTP(w, r)
    if d := w.Code; d != 200 {
    	t.Fatalf("网络不通或返回状态码不正确：%d", d)
    }
    if !w.Flushed {
    	t.Fatalf("从服务器上没有获得数据")
    }
}

func Test_pluginHTTP2(t *testing.T){
 	config := ConfigSitePlugin{
        Path        : "",
        LocalAddr	: ":0",
        Addr        : "www.baidu.com:443",
    	Host        : "www.baidu.com",
    	Scheme		: "https",
    	TLS			: &ConfigSitePluginTLS{
			ServerName:"www.baidu.com",
			InsecureSkipVerify:true,
    	},
    }
    c, err := configHTTPClient(nil, config)
    if err != nil {
    	t.Fatal(err)
    }
    phttp, err := c.Connection()
    if err != nil {
    	t.Fatal(err)
    }
    w := httptest.NewRecorder()
    r, err := http.NewRequest("GET", "https://www.baidu.com/", nil)
    if err != nil {
    	t.Fatal(err)
    }
    phttp.ServeHTTP(w, r)
    if d := w.Code; d != 200 {
    	t.Fatalf("网络不通或返回状态码不正确：%d", d)
    }
    if !w.Flushed {
    	t.Fatalf("从服务器上没有获得数据")
    }
}