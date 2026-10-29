package vweb

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPluginHTTPClient_NilTr(t *testing.T) {
	c := &PluginHTTPClient{}
	if _, err := c.Connection(); err == nil {
		t.Fatal("Tr 为 nil 应返回错误")
	}
}

func TestPluginHTTPClient_AddrEmptyDoesNotConsumeOnce(t *testing.T) {
	c := &PluginHTTPClient{Tr: &http.Transport{}}
	if _, err := c.Connection(); err != nil {
		t.Fatal(err)
	}
	if c.Tr.DialContext != nil {
		t.Fatal("Addr 为空时不应设置 DialContext")
	}
	c.Addr = "127.0.0.1:1"
	if _, err := c.Connection(); err != nil {
		t.Fatal(err)
	}
	if c.Tr.DialContext == nil {
		t.Fatal("补齐 Addr 后应设置 DialContext")
	}
}

func TestPluginHTTP_EndToEnd(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "ok")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hello-proxy"))
	}))
	defer backend.Close()

	addr := strings.TrimPrefix(backend.URL, "http://")
	client := &PluginHTTPClient{
		Tr:     &http.Transport{},
		Addr:   addr,
		Scheme: "http",
		Host:   addr,
	}
	ph, err := client.Connection()
	if err != nil {
		t.Fatal(err)
	}
	if ph.Type() != PluginTypeHTTP {
		t.Fatalf("Type=%v", ph.Type())
	}

	rw := httptest.NewRecorder()
	ph.ServeHTTP(rw, httptest.NewRequest("GET", "/x", nil))
	if rw.Code != http.StatusCreated || rw.Body.String() != "hello-proxy" {
		t.Fatalf("code=%d body=%q", rw.Code, rw.Body.String())
	}
	if rw.Header().Get("X-Backend") != "ok" {
		t.Fatal("响应头未透传")
	}
	if rw.Header().Get("Server") != "" {
		t.Fatalf("不应透传 Server 头：%q", rw.Header().Get("Server"))
	}
	ph.CloseIdleConnections()
}

func TestPluginHTTP_RoundTrip_FillCompleteURL(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Host)
	}))
	defer backend.Close()

	addr := strings.TrimPrefix(backend.URL, "http://")
	client := &PluginHTTPClient{Tr: &http.Transport{}, Addr: addr, Scheme: "http", Host: addr}
	ph, _ := client.Connection()

	req := httptest.NewRequest("GET", "/", nil)
	req.Host = ""
	req.URL.Host = ""
	req.URL.Scheme = ""
	resp, err := ph.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if string(b) != addr {
		t.Fatalf("backend 收到 Host=%q 期望 %q", b, addr)
	}
}

func TestPluginHTTP_ServeHTTP_UpstreamError504(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()

	client := &PluginHTTPClient{Tr: &http.Transport{}, Addr: addr, Scheme: "http", Host: addr}
	ph, _ := client.Connection()
	rw := httptest.NewRecorder()
	ph.ServeHTTP(rw, httptest.NewRequest("GET", "/x", nil))
	if rw.Code != http.StatusGatewayTimeout {
		t.Fatalf("期望 504，得到 %d", rw.Code)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPluginHTTP_RegisterProtocol(t *testing.T) {
	client := &PluginHTTPClient{Tr: &http.Transport{}, Addr: "127.0.0.1:1"}
	ph, _ := client.Connection()
	called := false
	ph.RegisterProtocol("http", roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.URL.Scheme = "http"
	ph.RoundTrip(req)
	if !called {
		t.Fatal("自定义 RoundTripper 未被调用")
	}
}

func TestPluginHTTP_CancelRequestNoPanic(t *testing.T) {
	client := &PluginHTTPClient{Tr: &http.Transport{}, Addr: "127.0.0.1:1"}
	ph, _ := client.Connection()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CancelRequest panic: %v", r)
		}
	}()
	ph.CancelRequest(httptest.NewRequest("GET", "/", nil))
}

func TestPluginHTTP_Connection_Concurrent(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer backend.Close()
	addr := strings.TrimPrefix(backend.URL, "http://")

	client := &PluginHTTPClient{Tr: &http.Transport{}, Addr: addr, Scheme: "http", Host: addr}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.Connection(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

// TestPluginHTTP_Boundary 空 Addr 不消耗 once、上游失败 504。
func TestPluginHTTP_Boundary(t *testing.T) {
	t.Run("Tr nil 报错", func(t *testing.T) {
		if _, err := (&PluginHTTPClient{}).Connection(); err == nil {
			t.Fatal("Tr nil 应报错")
		}
	})

	t.Run("Addr 空不消耗 once", func(t *testing.T) {
		c := &PluginHTTPClient{Tr: &http.Transport{}}
		c.Connection()
		if c.Tr.DialContext != nil {
			t.Fatal("Addr 空时不应设置 DialContext")
		}
		c.Addr = "127.0.0.1:1"
		c.Connection()
		if c.Tr.DialContext == nil {
			t.Fatal("补齐 Addr 后应设置")
		}
	})

	t.Run("上游失败 504", func(t *testing.T) {
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		addr := ln.Addr().String()
		ln.Close()
		c := &PluginHTTPClient{Tr: &http.Transport{}, Addr: addr, Scheme: "http", Host: addr}
		ph, _ := c.Connection()
		rw := httptest.NewRecorder()
		ph.ServeHTTP(rw, httptest.NewRequest("GET", "/x", nil))
		if rw.Code != http.StatusGatewayTimeout {
			t.Fatalf("应 504，实得 %d", rw.Code)
		}
	})
}

func TestPluginHTTP_FillCompleteURL(t *testing.T) {
	ph := &pluginHTTP{host: "backend:80", scheme: "http"}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Host = ""
	r.URL.Host = ""
	r.URL.Scheme = ""
	ph.fillCompleteURL(r)
	if r.Host != "backend:80" {
		t.Fatalf("Host=%q", r.Host)
	}
	if r.Header.Get("Host") != "backend:80" {
		t.Fatalf("Header.Host=%q", r.Header.Get("Host"))
	}
	if r.URL.Host != "backend:80" {
		t.Fatalf("URL.Host=%q", r.URL.Host)
	}
	if r.URL.Scheme != "http" {
		t.Fatalf("URL.Scheme=%q", r.URL.Scheme)
	}
}

func TestPluginHTTP_FillCompleteURL_Partial(t *testing.T) {
	ph := &pluginHTTP{host: "b:80", scheme: "http"}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Host = "orig:80"
	r.URL.Scheme = "https"
	ph.fillCompleteURL(r)
	if r.Host != "orig:80" {
		t.Fatalf("Host 不应被覆盖：%q", r.Host)
	}
	if r.URL.Host != "orig:80" {
		t.Fatalf("URL.Host=%q", r.URL.Host)
	}
	if r.URL.Scheme != "https" {
		t.Fatalf("Scheme 不应被覆盖：%q", r.URL.Scheme)
	}
}
