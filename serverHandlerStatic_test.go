﻿package vweb

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_serverHandlerStaticHeader(t *testing.T) {
	tempFile, err := os.CreateTemp("", "T")
	if err != nil {
		t.Fatalf("打开文件错误：%v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	tempFile.Write([]byte("123456"))
	fi, err := tempFile.Stat()
	if err != nil {
		t.Fatalf("读取文件(%s)信息错误：%v", tempFile.Name(), err)
	}

	wh := make(http.Header)
	rh := http.Header{}
	shsh := serverHandlerStaticHeader{
		fileInfo: fi,
		wh:       wh,
	}

	shsh.setLastModified()
	if shsh.lastModified() != wh.Get("Last-Modified") {
		t.Fatalf("返回的修改时间和设置的修改时间不一致")
	}

	shsh.setDate()
	if time.Now().Format(http.TimeFormat) != wh.Get("Date") {
		t.Fatalf("返回的系统时间和设置的系统时间不一致")
	}

	shsh.setContentLength()
	if shsh.contentLength() != wh.Get("Content-Length") {
		t.Fatalf("返回的文件大小和设置的文件大小不一致")
	}

	shsh.setETag()
	if shsh.etag() != wh.Get("ETag") {
		t.Fatalf("返回的ETag和设置的ETag不一致")
	}

	test_Range := []struct {
		h   string
		n   int64
		err bool
	}{
		{h: "bytes=1-2", n: 2},
		{h: "bytes=0-2", n: 3},
		{h: "bytes=-2", n: 2}, //-2 表示后面两位字节
		{h: "bytes=0-", n: fi.Size()},
		{h: "bytes=60-80", n: 0}, // 长度超出
		{h: "bytes=2-1", n: 0},   // 错误的格式
		{h: "bytes=1-1", n: 1},
		{h: "bytes=-1-", n: 0, err: true}, // 错误的格式
		{h: "bytes=0-,1-2,0-2", n: fi.Size() + 2 + 3},
		{h: "bytes=0-,1-2,-1-2", n: 12, err: true},    //-1-2 是错误的，忽略
		{h: "bytes=0-,1-2,1-4", n: fi.Size() + 2 + 4}, // 1-4 超出长度
	}
	for _, v := range test_Range {
		rh.Set("Range", v.h)
		_, n, err := shsh.ranges(v.h)
		if err != nil && !v.err {
			t.Fatalf("数据长度：%d, 标头 Raange: %s，是不正确的，错误：%v\r\n", fi.Size(), v.h, err)
		}
		if n != v.n && !v.err {
			t.Fatalf("标头 Raange: %s， 数据长度：%d, 返回长度：%d\r\n", v.h, fi.Size(), n)
		}
	}
}

func Test_ServerHandlerStatic_header(t *testing.T) {
	tempFile, err := os.CreateTemp("", "T")
	if err != nil {
		t.Fatalf("打开文件错误：%v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	tempFile.Write([]byte("123456"))
	fi, err := tempFile.Stat()
	if err != nil {
		t.Fatalf("读取文件(%s)信息错误：%v", tempFile.Name(), err)
	}

	test_Range := []struct {
		h   string
		err bool
	}{
		{h: "bytes=1-2"},
		{h: "bytes=0-2,1-2,0-3"},
		{h: "bytes=-2"}, //-2 表示后面两位字节
		{h: "bytes=0-"},
		{h: "bytes=60-80"}, // 长度超出
		{h: "bytes=2-1"},   // 错误的格式
		{h: "bytes=1-1"},
		{h: "bytes=-1-", err: true}, // 错误的格式
		{h: "bytes=0-,1-2,0-2"},
		{h: "bytes=0-,1-2,-1-2", err: true}, //-1-2 是错误的，忽略
		{h: "bytes=0-,1-2,1-4"},             // 1-4 超出长度
		{h: "bytes=0-a", err: true},         // 这不是有效的格式
	}

	shs := ServerHandlerStatic{
		fileInfo:    fi,
		PageExpired: 500,
	}

	w := httptest.NewRecorder()
	r, _ := http.NewRequest("GET", "./test/1.html", nil)
	for _, v := range test_Range {
		r.Header.Set("Range", v.h)
		_, err := shs.header(w, r)
		if err != nil && !v.err {
			t.Fatal(err)
		}
		// t.Log(block)
	}
}

func Test_serverHandlerStatic_body(t *testing.T) {
	tempFile, err := os.CreateTemp("", "T")
	if err != nil {
		t.Fatalf("打开文件错误：%v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	tempFile.Write([]byte("123456"))
	fi, err := tempFile.Stat()
	if err != nil {
		t.Fatalf("读取文件(%s)信息错误：%v", tempFile.Name(), err)
	}

	shsh := serverHandlerStaticHeader{
		fileInfo: fi,
	}
	shs := &ServerHandlerStatic{
		RootPath: filepath.Dir(tempFile.Name()),
		PagePath: fi.Name(),
		BuffSize: 1024,
		fileInfo: fi,
	}

	tests := []struct {
		rh     map[string]string
		code   int
		length string
	}{
		{rh: map[string]string{"If-None-Match": shsh.etag()}, code: 304, length: ""},
		{rh: map[string]string{"If-Modified-Since": shsh.lastModified()}, code: 200, length: shsh.contentLength()},
		{rh: map[string]string{"Range": "bytes=-1"}, code: 206, length: "1"},
		{rh: map[string]string{"Range": "bytes=-0"}, code: 416, length: ""},
		{rh: map[string]string{"Range": "bytes=-3333"}, code: 206, length: shsh.contentLength()},
		{rh: map[string]string{"Range": "bytes=0-0"}, code: 206, length: "1"},
		{rh: map[string]string{"Range": "bytes=0-"}, code: 206, length: shsh.contentLength()},
		{rh: map[string]string{"Range": "bytes=10-"}, code: 200, length: shsh.contentLength()},
		{rh: map[string]string{"Range": "bytes=-5-5"}, code: 416, length: ""},
		{rh: map[string]string{"Range": "bytes=0-0,0-2"}, code: 206, length: "326"},
		{rh: map[string]string{"Range": "bytes=0-0,0-77777"}, code: 200, length: "6"},
	}

	for _, test := range tests {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Type", "text/application")
		r := new(http.Request)
		r.Header = make(http.Header)
		for k, v := range test.rh {
			r.Header.Add(k, v)
		}
		rangeBlock, err := shs.header(w, r)
		if err != nil && w.Code != test.code {
			t.Fatal(err)
		}
		// t.Log(rangeBlock)
		shs.body(w, rangeBlock)
		// shs.serveHTTP(w, r)
		if w.Code != test.code || test.length != w.Header().Get("Content-Length") {
			t.Fatalf("\r\n\t请求Range:%v \r\n\t状态码：%d != %d \r\n\tHeader标头：%s \r\n\t内容：%s \r\n ", test.rh, test.code, w.Code, w.Header(), w.Body.String())
		}
	}
}
