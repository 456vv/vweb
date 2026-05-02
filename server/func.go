package server

import (
	"net/http"
	"strings"

	"github.com/456vv/vweb/v2/server/config"
)

// derogatoryDomain 贬域名
//
//	host string             host地址
//	f func(string) bool     调用 f 函数，并传入贬域名
func derogatoryDomain(host string, f func(string) bool) {
	// 先全字匹配
	if f(host) {
		return
	}
	// 后通配符匹配
	pos := strings.Index(host, ":")
	var port string
	if pos >= 0 {
		port = host[pos:]
		host = host[:pos]
	}
	labels := strings.Split(host, ".")
	for i := range labels {
		labels[i] = "*"
		candidate := strings.Join(labels, ".") + port
		if f(candidate) {
			break
		}
	}
}

// strSliceContains 从切片中查找匹配的字符串
func strSliceContains(ss []string, c string) bool {
	for _, v := range ss {
		if v == c {
			return true
		}
	}
	return false
}

func siteHeaderType(wh http.Header, mht map[string]config.SiteHeaderType, fileExt string) config.SiteHeaderType {
	var ht config.SiteHeaderType
	if h, ok := mht[fileExt]; ok {
		ht = h
	} else if h, ok := mht["*"]; ok {
		ht = h
	}
	for k, v := range ht.Header {
		for _, v1 := range v {
			wh.Add(k, v1)
		}
	}
	return ht
}
