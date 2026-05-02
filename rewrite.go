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

// Compile 预编译正则，建议在程序初始化或配置加载时调用一次
func (T *Forward) Compile() (*ForwardRewriter, error) {
	r := &ForwardRewriter{
		pathExact:    make(map[string]struct{}),
		excludeExact: make(map[string]struct{}),
		rePath:       T.RePath,
	}
	for _, p := range T.ExcludePath {
		if re, err := regexp.Compile(p); err == nil {
			if isRegex(re) {
				r.excludeRegex = append(r.excludeRegex, re)
			} else {
				r.excludeExact[p] = struct{}{}
			}
		} else {
			return nil, fmt.Errorf("vweb: 是错误正则 re2(%s)", p)
		}
	}
	for _, p := range T.Path {
		if re, err := regexp.Compile(p); err == nil {
			if isRegex(re) {
				r.pathRegex = append(r.pathRegex, re)
			} else {
				r.pathExact[p] = struct{}{}
			}
		} else {
			return nil, fmt.Errorf("vweb: 是错误正则 re2(%s)", p)
		}
	}
	return r, nil
}

func (T *ForwardRewriter) Rewrite(upath string) (string, bool) {
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
		// 精确匹配时，$0 替换为原路径
		res := strings.ReplaceAll(T.rePath, "$0", upath)
		return res, true
	}

	// 4. 检查包含路径 (正则匹配)
	for _, reg := range T.pathRegex {
		if reg.MatchString(upath) {
			// 使用正则官方替换方法，支持 $1, $2 等
			return reg.ReplaceAllString(upath, T.rePath), true
		}
	}
	return upath, false
}
