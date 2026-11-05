package server

import (
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/456vv/vweb/v3/server/config"
)

// derogatoryDomain 贬域名匹配。
// 先全字匹配，再从左向右逐级将标签替换为 "*" 做通配符匹配，保留原始端口（若存在）。
// 一旦 f 返回 true 即停止。IP 字面量不参与域名级通配。
//
// 示例：
//
//	host = "a.b.example.com:443"
//	依次尝试：
//	  a.b.example.com:443
//	  *.b.example.com:443
//	  *.*.example.com:443
//	  *.*.*.com:443
//	  *.*.*.*:443
func derogatoryDomain(host string, f func(string) bool) {
	if f(host) {
		return
	}

	h, port := splitHostPort(host)
	if h == "" || net.ParseIP(h) != nil {
		return
	}

	labels := strings.Split(h, ".")
	for i := range labels {
		labels[i] = "*"
		candidate := strings.Join(labels, ".") + port
		if f(candidate) {
			return
		}
	}
}

// splitHostPort 安全拆分 host:port，兼容 IPv4/IPv6 字面量（含 [] 与 zone）。
// 无端口时返回原始 host 与空 port；"[IPv6]" 无端口写法会去掉外层方括号。
//
// 示例：
//
//	splitHostPort("127.0.0.1:80") → ("127.0.0.1", ":80")
//	splitHostPort("[::1]:443")    → ("::1", ":443")
//	splitHostPort("example.com")  → ("example.com", "")
func splitHostPort(hostport string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, ":" + p
	}
	if n := len(hostport); n >= 2 && hostport[0] == '[' && hostport[n-1] == ']' {
		return hostport[1 : n-1], ""
	}
	return hostport, ""
}

// strSliceContains 线性扫描切片是否包含目标字符串。
func strSliceContains(ss []string, c string) bool {
	for _, v := range ss {
		if v == c {
			return true
		}
	}
	return false
}

// siteHeaderType 根据文件扩展名从映射中选取 SiteHeaderType，
// 并将其中的 Header 写入 ResponseWriter。优先精确匹配 fileExt，其次 "*"。
// 返回选中的 SiteHeaderType（可能为零值）。mht 为 nil 时读取是安全的。
//
// 注意：配置里的同名头会 Del 后再 Add，避免与服务器已写的 Content-Type 重复。
func siteHeaderType(wh http.Header, mht map[string]config.SiteHeaderType, fileExt string) config.SiteHeaderType {
	var ht config.SiteHeaderType
	if h, ok := mht[fileExt]; ok {
		ht = h
	} else if h, ok := mht["*"]; ok {
		ht = h
	}
	for k, vs := range ht.Header {
		wh.Del(k)
		for _, v := range vs {
			wh.Add(k, v)
		}
	}
	return ht
}

// isLocalPath 检查相对路径是否安全（无 ".."、非绝对路径、非 UNC、filepath.IsLocal）。
//
// 返回 true 表示可安全拼接在站点根目录下。
//
// 示例：
//
//	isLocalPath("css/app.css") → true
//	isLocalPath("../etc/passwd") → false
//	isLocalPath("/abs") → false
func isLocalPath(rel string) bool {
	if rel == "" || rel == "." {
		return true
	}
	// 拒绝绝对路径、盘符路径或 UNC 路径
	if filepath.IsAbs(rel) {
		return false
	}
	if filepath.VolumeName(rel) != "" {
		return false
	}
	if strings.HasPrefix(rel, `\\`) || strings.HasPrefix(rel, "//") {
		return false
	}

	// 使用标准库 IsLocal 增强跨平台安全（Go 1.20+）
	if !filepath.IsLocal(rel) {
		return false
	}

	// 检查中间是否存在 ".." 组件（同时兼容 / 与系统分隔符）
	return !slices.Contains(strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == filepath.Separator }), "..")
}
