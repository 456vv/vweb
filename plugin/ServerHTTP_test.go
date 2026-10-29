package plugin

import (
	"bytes"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func Test_Server_HTTP_1(t *testing.T) {
	sendTest := []byte("test")
	sh := NewServerHTTP()
	defer sh.Close()
	sh.Route.HandleFunc("^/(\\d+)$", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})
	sh.Addr = "127.0.0.1:0"
	go sh.ListenAndServe()

	time.Sleep(time.Second * 2)

	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://" + sh.l.Addr().String() + "/123")
	if err != nil {
		t.Fatalf("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatalf("读出数据出错，错误：%v", err)
	}
	if !bytes.Equal(sendTest, b) {
		t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
	}
}

func Test_Server_HTTP_2(t *testing.T) {
	sendTest := []byte("test")

	netListener, err := net.Listen("tcp", "127.0.0.1:5003")
	if err != nil {
		t.Fatalf("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()

	sh := NewServerHTTP()
	sh.Route.HandleFunc("/(\\d+)$", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})
	go sh.Serve(netListener)

	time.Sleep(time.Second * 2)

	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://" + sh.l.Addr().String() + "/123")
	if err != nil {
		t.Fatalf("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatalf("读出数据出错，错误：%v", err)
	}
	if !bytes.Equal(sendTest, b) {
		t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
	}
}

func Test_Server_HTTP_3(t *testing.T) {
	sendTest := []byte("test")

	netListener, err := net.Listen("tcp", "127.0.0.1:5002")
	if err != nil {
		t.Fatalf("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()
	serverHTTP := NewServerHTTP()
	serverHTTP.Route.HandleFunc("/123", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})
	go serverHTTP.Serve(netListener)

	time.Sleep(time.Second * 2)

	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://" + serverHTTP.l.Addr().String() + "/123")
	if err != nil {
		t.Fatalf("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatalf("读出数据出错，错误：%v", err)
	}
	if !bytes.Equal(sendTest, b) {
		t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
	}
}

func Test_Server_HTTP_4(t *testing.T) {
	sendTest := []byte("test")

	netListener, err := net.Listen("tcp", "127.0.0.1:5001")
	if err != nil {
		t.Fatalf("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()
	serverHTTP := NewServerHTTP()
	serverHTTP.Route.HandleFunc("/123", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})

	err = serverHTTP.LoadTLS("test/Cer/Cert-test.pem", "test/Cer/Cert-test.key")
	if err != nil {
		t.Fatalf("错误：%v", err)
	}

	go serverHTTP.Serve(netListener)

	time.Sleep(time.Second * 2)
	addr := serverHTTP.l.Addr().String()
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Scheme: "https", Host: addr, Path: "/123"},
	}
	httpResponse, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := io.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatalf("读出数据出错，错误：%v", err)
	}
	if !bytes.Equal(sendTest, b) {
		t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
	}
}

type nonTCPListener struct{ net.Listener }

func (l *nonTCPListener) Accept() (net.Conn, error) { return l.Listener.Accept() }

func TestServerHTTP_Serve_NonTCPListener(t *testing.T) {
	sh := NewServerHTTP()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := sh.Serve(&nonTCPListener{Listener: ln}); err == nil {
		t.Fatal("非 TCP listener 应报错")
	}
}

func TestServerHTTP_CloseWithoutServe(t *testing.T) {
	sh := NewServerHTTP()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Close 未 Serve 前不应 panic：%v", r)
		}
	}()
	_ = sh.Close()
}

func TestServerHTTP_LoadTLS_InvalidFiles(t *testing.T) {
	sh := NewServerHTTP()
	if err := sh.LoadTLS("/nonexistent/cert.pem", "/nonexistent/key.pem"); err == nil {
		t.Fatal("不存在的证书应报错")
	}
}

// TestPluginServer_NonTCP_HTTP 非 TCP listener 拒绝。
func TestPluginServer_NonTCP_HTTP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := NewServerHTTP().Serve(&nonTCPListener{Listener: ln}); err == nil {
		t.Fatal("非 TCP 应报错")
	}
}
