package plugin

import (
	"net/http"
	"testing"

	//"os"
	"bytes"
	"crypto/tls"
	"io/ioutil"
	"net"
	"net/url"
	"time"
)

func Test_Server_HTTP_1(t *testing.T) {
	sendTest := []byte("test")
	// 监听
	sh := NewServerHTTP()
	defer sh.Close()
	// 路径绑定一个函数，路径支持正则格式
	sh.Route.HandleFunc("^/(\\d+)$", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})
	sh.Addr = "127.0.0.1:0"
	go sh.ListenAndServe()

	time.Sleep(time.Second * 2)

	// 请求一个连接
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

func Test_Server_HTTP_2(t *testing.T) {
	sendTest := []byte("test")

	// 监听
	netListener, err := net.Listen("tcp", "127.0.0.1:5003")
	if err != nil {
		t.Fatalf("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()

	sh := NewServerHTTP()
	// 路径绑定一个函数，路径支持正则格式
	sh.Route.HandleFunc("/(\\d+)$", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})
	go sh.Serve(netListener)

	time.Sleep(time.Second * 2)

	// 请求一个连接
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

	// 监听
	netListener, err := net.Listen("tcp", "127.0.0.1:5002")
	if err != nil {
		t.Fatalf("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()
	serverHTTP := NewServerHTTP()
	// 路径绑定一个函数，路径支持正则格式
	serverHTTP.Route.HandleFunc("/123", func(rw http.ResponseWriter, r *http.Request) {
		rw.Write(sendTest)
	})
	go serverHTTP.Serve(netListener)

	time.Sleep(time.Second * 2)

	// 请求一个连接
	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://" + serverHTTP.l.Addr().String() + "/123")
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

func Test_Server_HTTP_4(t *testing.T) {
	sendTest := []byte("test")

	// 监听
	netListener, err := net.Listen("tcp", "127.0.0.1:5001")
	if err != nil {
		t.Fatalf("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()
	serverHTTP := NewServerHTTP()
	// 路径绑定一个函数，路径支持正则格式
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
	// 请求一个连接
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
	b, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatalf("读出数据出错，错误：%v", err)
	}
	if !bytes.Equal(sendTest, b) {
		t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
	}
}
