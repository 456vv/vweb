package vweb

import (
	"fmt"
	"regexp"
	"strings"
)

// Forward 原始配置结构
type Forward struct {
	Path        []string // 多种路径匹配
	ExcludePath []string // 排除多种路径匹配
	RePath      string   // 重写路径
}

// ForwardRewriter 编译后的执行对象
type ForwardRewriter struct {
	pathExact    map[string]struct{}
	pathRegex    []*regexp.Regexp
	excludeExact map[string]struct{}
	excludeRegex []*regexp.Regexp
	rePath       string
}

// Compile 预编译正则，建议在程序初始化或配置加载时调用一次。
// 返回的 *ForwardRewriter 可被多 goroutine 并发安全地调用 Rewrite。
func (T *Forward) Compile() (*ForwardRewriter, error) {
	if T == nil {
		return nil, fmt.Errorf("vweb: Forward 不能为 nil")
	}

	r := &ForwardRewriter{
		pathExact:    make(map[string]struct{}, len(T.Path)),
		excludeExact: make(map[string]struct{}, len(T.ExcludePath)),
		pathRegex:    make([]*regexp.Regexp, 0, len(T.Path)),
		excludeRegex: make([]*regexp.Regexp, 0, len(T.ExcludePath)),
		rePath:       T.RePath,
	}

	for _, p := range T.ExcludePath {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("vweb: 无效的正则表达式(%s): %w", p, err)
		}
		// 完整字面量（含空字符串）放入 exact，避免空正则匹配一切的问题
		if prefix, complete := re.LiteralPrefix(); complete {
			r.excludeExact[prefix] = struct{}{}
		} else {
			r.excludeRegex = append(r.excludeRegex, re)
		}
	}

	for _, p := range T.Path {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("vweb: 无效的正则表达式(%s): %w", p, err)
		}
		if prefix, complete := re.LiteralPrefix(); complete {
			r.pathExact[prefix] = struct{}{}
		} else {
			r.pathRegex = append(r.pathRegex, re)
		}
	}

	return r, nil
}

// Rewrite 对给定路径进行重写。
// 返回值：(重写后的路径, 是否发生了重写)
// 方法本身是并发安全的，可在多个 goroutine 中同时调用。
func (T *ForwardRewriter) Rewrite(upath string) (string, bool) {
	if T == nil {
		return upath, false
	}

	// 1. 检查排除路径 (精确匹配)
	if _, ok := T.excludeExact[upath]; ok {
		return upath, false
	}
	// 2. 检查排除路径 (正则匹配)
	for _, reg := range T.excludeRegex {
		if reg.MatchString(upath) {
			return upath, false
		}
	}
	// 3. 检查包含路径 (精确匹配)
	if _, ok := T.pathExact[upath]; ok {
		// 精确匹配时仅替换 $0（与原语义保持一致）
		res := strings.ReplaceAll(T.rePath, "$0", upath)
		return res, true
	}

	// 4. 检查包含路径 (正则匹配)
	for _, reg := range T.pathRegex {
		if reg.MatchString(upath) {
			// 使用正则官方替换，完整支持 $0/$1/$2… 以及 $$ 转义
			return reg.ReplaceAllString(upath, T.rePath), true
		}
	}
	return upath, false
}

// 判断是否是真正的正则表达式（简单判断）
func isRegex1(re *regexp.Regexp) bool {
	prefix, complete := re.LiteralPrefix()
	return !complete || prefix == ""
}
