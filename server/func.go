package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/456vv/vweb/v3/server/config"
)

// derogatoryDomain 贬域名匹配。
// 先全字匹配，再从左向右逐级将标签替换为 "*" 做通配符匹配，保留原始端口（若存在）。
// 一旦 f 返回 true 即停止。兼容 IPv4/IPv6（使用 net.SplitHostPort）。
// 优化：IP 字面量不参与域名级通配，直接结束，避免无意义的通配候选拼接。
func derogatoryDomain(host string, f func(string) bool) {
	if f(host) {
		return
	}

	h, port := splitHostPort(host)
	// IP 或空主机不存在可用的“贬域名”匹配（不会命中站点/转发等域名配置），提前返回
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
// 无端口时返回原始 host 与空 port。
// 补充：兼容“[IPv6]”这种“无端口但带方括号”的写法（net.SplitHostPort 会报 missing port），
// 去掉外层 [] 以便后续标签处理。
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
// 适用于插件名、Host 等少量元素；元素较多时可改用 map[string]struct{}。
func strSliceContains(ss []string, c string) bool {
	for _, v := range ss {
		if v == c {
			return true
		}
	}
	return false
}

// siteHeaderType 根据文件扩展名从映射中选取 SiteHeaderType，
// 并将其中的 Header 全部添加到 ResponseWriter。优先精确匹配 fileExt，其次 "*"。
// 返回选中的 SiteHeaderType（可能为零值）。
// 说明：mht 为 nil 时对 nil map 读取是安全的，无需额外判空。
func siteHeaderType(wh http.Header, mht map[string]config.SiteHeaderType, fileExt string) config.SiteHeaderType {
	var ht config.SiteHeaderType
	if h, ok := mht[fileExt]; ok {
		ht = h
	} else if h, ok := mht["*"]; ok {
		ht = h
	}
	for k, vs := range ht.Header {
		for _, v := range vs {
			wh.Add(k, v)
		}
	}
	return ht
}
