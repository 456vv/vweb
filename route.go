package vweb

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
)

type Router interface {
	HandleFunc(url string, handler func(w http.ResponseWriter, r *http.Request))
	HandleFuncDot(url string, handler ...func(Doter))
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// routePathPlaceholder 定义了路由路径的结构和占位符信息
type routePathPlaceholder struct {
	pathSegments  []string       // 路径的各个片段，可能包含占位符，如 "b{id}b"
	isDir         bool           // 如果路径以 '/' 结尾，表示这是一个目录
	isRegexp      bool           // 如果路径包含正则表达式，表示这是一个正则匹配的路由
	pathRegex     *regexp.Regexp // 路径的正则表达式
	isSymbol      bool           // 如果路径包含通配符，表示这是一个通配符匹配的路由
	isPlaceholder bool           // 如果路径包含占位符，表示这是一个占位符匹配的路由
	handle        http.Handler
	// 预解析的占位符段，避免每次请求重复解析，提升性能
	parsedParts [][]segmentPart
	// 原始注册的 url，用于 list 中删除定位
	url string
}

type Route struct {
	HandlerError http.HandlerFunc // 错误访问处理
	rt           sync.Map         // 路由表 map[string]*routePathPlaceholder，精确匹配走此表
	list         []*routePathPlaceholder
	siteMan      *SiteMan
	mu           sync.RWMutex // 保护 list 的并发读写
}

// SetSiteMan 设置站点管理，将会携带在请求上下文中
// siteMan *SiteMan	站点
func (T *Route) SetSiteMan(siteMan *SiteMan) {
	T.siteMan = siteMan
}

// HandleFunc 绑定处理函数，匹配的网址支持正则，这说明你要严格的检查。
//
//	url string                                          网址，支持正则匹配
//	handler func(w ResponseWriter, r *Request)    		处理函数
func (T *Route) HandleFunc(url string, handler func(w http.ResponseWriter, r *http.Request)) {
	if handler == nil {
		T.removeHandle(url)
		return
	}
	T.addHandle(url, http.Handler(http.HandlerFunc(handler)))
}

func (T *Route) HandleFuncDot(url string, handler ...func(Doter)) {
	if handler == nil {
		T.removeHandle(url)
		return
	}
	T.addHandle(url, http.Handler(HandleFunc(handler)))
}

func (T *Route) removeHandle(url string) {
	T.rt.Delete(url)
	T.mu.Lock()
	defer T.mu.Unlock()
	for i, p := range T.list {
		if p != nil && p.url == url {
			// 保持顺序，原地删除
			T.list = append(T.list[:i], T.list[i+1:]...)
			return
		}
	}
}

func (T *Route) addHandle(url string, handle http.Handler) {
	pathPlaceholder := &routePathPlaceholder{
		isDir:        strings.HasSuffix(url, "/"),
		pathSegments: strings.Split(url, "/"),
		handle:       handle,
		url:          url,
	}

	if strings.HasPrefix(url, "^") || strings.HasSuffix(url, "$") {
		pathRegex, err := regexp.Compile(url)
		if err != nil || !isRegex(pathRegex) {
			panic("invalid regular expression: " + url + "")
		}
		pathRegex.Longest()
		pathPlaceholder.isRegexp = true
		pathPlaceholder.pathRegex = pathRegex
	} else if strings.ContainsAny(url, "[]*?\\") {
		pathPlaceholder.isSymbol = true
	} else if strings.ContainsAny(url, "{}") {
		pathPlaceholder.isPlaceholder = true
		// 预解析每个段的占位符结构，请求时只做匹配，不再解析语法
		pathPlaceholder.parsedParts = make([][]segmentPart, len(pathPlaceholder.pathSegments))
		for i, seg := range pathPlaceholder.pathSegments {
			if strings.Contains(seg, "{") || strings.Contains(seg, "}") {
				parts, ok := parsePlaceholderSegment(seg)
				if !ok {
					panic("invalid placeholder segment: " + seg)
				}
				pathPlaceholder.parsedParts[i] = parts
			}
		}
	}

	// 精确路径直接存 sync.Map，模式路径同时维护有序列表以保证匹配顺序确定
	T.rt.Store(url, pathPlaceholder)

	T.mu.Lock()
	defer T.mu.Unlock()
	// 若已存在同 url 的旧项则替换，保持注册顺序稳定
	for i, p := range T.list {
		if p != nil && p.url == url {
			T.list[i] = pathPlaceholder
			return
		}
	}
	T.list = append(T.list, pathPlaceholder)
}

// ServeHTTP 服务HTTP
//
//	w ResponseWriter    响应
//	r *Request          请求
func (T *Route) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// 存在站点管理，检查host读取站点
	if T.siteMan != nil {
		//** 检查Host是否存在
		site, ok := T.siteMan.Get(r.Host)
		if !ok {
			// 如果在站点集中没有找到存在的Host，则关闭连接。
			hj, ok := w.(http.Hijacker)
			if !ok {
				// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
				http.Error(w, "Not supported Hijacker", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// 直接关闭连接
			defer conn.Close()
			return
		}
		ctx = context.WithValue(ctx, SiteContextKey, site)
	}

	ctx = context.WithValue(ctx, RouterContextKey, Router(T))
	r = r.WithContext(ctx)

	userPath := r.URL.Path

	// 1. 精确匹配（O(1)）
	if v, ok := T.rt.Load(userPath); ok {
		pathPlaceholder := v.(*routePathPlaceholder)
		// 只有当该条目本身不是模式路由时才直接命中；模式路由即使 key 相同也走下面逻辑
		if !pathPlaceholder.isRegexp && !pathPlaceholder.isSymbol && !pathPlaceholder.isPlaceholder {
			pathPlaceholder.handle.ServeHTTP(w, r)
			return
		}
	}

	// 2. 模式匹配：按注册顺序遍历 list，保证确定性
	filteredUserSegments := strings.Split(userPath, "/")
	isDir := strings.HasSuffix(userPath, "/")

	T.mu.RLock()
	list := T.list
	T.mu.RUnlock()

	for _, pathPlaceholder := range list {
		if pathPlaceholder == nil {
			continue
		}
		// 目录性必须一致
		if pathPlaceholder.isDir != isDir {
			continue
		}

		if pathPlaceholder.isRegexp {
			if pathPlaceholder.pathRegex.MatchString(userPath) {
				pathPlaceholder.handle.ServeHTTP(w, r)
				return
			}
			continue
		}

		if pathPlaceholder.isSymbol {
			if matched, _ := path.Match(pathPlaceholder.url, userPath); matched {
				pathPlaceholder.handle.ServeHTTP(w, r)
				return
			}
			continue
		}

		if pathPlaceholder.isPlaceholder {
			if param, ok := parsePathParamsPreparsed(filteredUserSegments, pathPlaceholder.pathSegments, pathPlaceholder.parsedParts); ok {
				r = r.WithContext(context.WithValue(r.Context(), "url-params", param))
				pathPlaceholder.handle.ServeHTTP(w, r)
				return
			}
			continue
		}

		// 纯文字但因某种原因进入 list 的路径（理论上不应出现）
		if pathPlaceholder.url == userPath {
			pathPlaceholder.handle.ServeHTTP(w, r)
			return
		}
	}

	// 处理错误的请求
	if T.HandlerError != nil {
		T.HandlerError.ServeHTTP(w, r)
		return
	}

	// 默认的错误处理
	w.Header().Set("Connection", "close")
	http.Error(w, fmt.Sprintf("The path does not exist (%s)", userPath), http.StatusNotFound)
}

// segmentPart 结构用于内部解析占位符片段，区分文字部分和占位符名称
type segmentPart struct {
	isPlaceholder bool   // true 表示是占位符 (如 "id"), false 表示是文字 (如 "b")
	value         string // 占位符名称或文字内容
}

// parsePathParamsPreparsed 使用注册时预解析的 segmentPart，避免每次请求重新解析语法
func parsePathParamsPreparsed(userSegments, pathSegments []string, preParsed [][]segmentPart) (result map[string]string, ok bool) {
	if len(userSegments) != len(pathSegments) {
		return nil, false
	}

	result = make(map[string]string, 4) // 预分配小容量，减少扩容
	for i := range userSegments {
		placeholderSegment := pathSegments[i]
		userSegment := userSegments[i]

		// 纯文字段必须精确匹配
		if !strings.Contains(placeholderSegment, "{") && !strings.Contains(placeholderSegment, "}") {
			if placeholderSegment != userSegment {
				return nil, false
			}
			continue
		}

		var params map[string]string
		var okExtract bool
		if preParsed != nil && i < len(preParsed) && preParsed[i] != nil {
			params, okExtract = extractParamsFromSegmentPreparsed(preParsed[i], userSegment)
		} else {
			params, okExtract = extractParamsFromSegment(placeholderSegment, userSegment)
		}
		if !okExtract {
			return nil, false
		}
		maps.Copy(result, params)
	}
	return result, true
}

// extractParamsFromSegment 负责从一个包含占位符的段中提取实际参数值。
// 例如，placeholderSeg="b{id}b", userSeg="b10b"，将提取 "id":"10"。
// 成功返回参数map和true，失败返回nil和false。
func extractParamsFromSegment(placeholderSeg, userSeg string) (map[string]string, bool) {
	parsedParts, ok := parsePlaceholderSegment(placeholderSeg)
	if !ok {
		return nil, false
	}
	return extractParamsFromSegmentPreparsed(parsedParts, userSeg)
}

// extractParamsFromSegmentPreparsed 使用已解析的 parts，仅做字符串匹配与切片
func extractParamsFromSegmentPreparsed(parsedParts []segmentPart, userSeg string) (map[string]string, bool) {
	result := make(map[string]string, len(parsedParts)/2+1)
	userSegIdx := 0
	userSegLen := len(userSeg)

	for i, part := range parsedParts {
		if !part.isPlaceholder {
			partLen := len(part.value)
			if userSegIdx+partLen > userSegLen {
				return nil, false
			}
			if userSeg[userSegIdx:userSegIdx+partLen] != part.value {
				return nil, false
			}
			userSegIdx += partLen
			continue
		}

		// 占位符：找到下一个文字的位置作为结束边界
		var endOfParam int
		if i+1 < len(parsedParts) {
			nextLiteral := parsedParts[i+1].value
			foundIdx := strings.Index(userSeg[userSegIdx:], nextLiteral)
			if foundIdx == -1 {
				return nil, false
			}
			endOfParam = userSegIdx + foundIdx
		} else {
			endOfParam = userSegLen
		}

		if userSegIdx > endOfParam {
			return nil, false
		}
		result[part.value] = userSeg[userSegIdx:endOfParam]
		userSegIdx = endOfParam
	}

	if userSegIdx != userSegLen {
		return nil, false
	}
	return result, true
}

// parsePlaceholderSegment 辅助函数，将占位符段（如 "b{id}b"）解析成
// 包含文字和占位符名称的 segmentPart 序列。
// 成功返回解析后的序列和true，失败返回nil和false（表示格式错误）。
func parsePlaceholderSegment(placeholderSeg string) ([]segmentPart, bool) {
	parts := make([]segmentPart, 0, 4)
	currentBuffer := strings.Builder{}
	inPlaceholder := false

	for i := 0; i < len(placeholderSeg); i++ {
		char := placeholderSeg[i]

		switch char {
		case '{':
			if inPlaceholder {
				return nil, false // 嵌套 { 非法
			}
			if currentBuffer.Len() > 0 {
				parts = append(parts, segmentPart{false, currentBuffer.String()})
				currentBuffer.Reset()
			}
			inPlaceholder = true
		case '}':
			if !inPlaceholder {
				return nil, false // 多余的 }
			}
			if currentBuffer.Len() == 0 {
				return nil, false // 空占位符 {}
			}
			parts = append(parts, segmentPart{true, currentBuffer.String()})
			currentBuffer.Reset()
			inPlaceholder = false
		default:
			currentBuffer.WriteByte(char)
		}
	}

	if inPlaceholder {
		return nil, false // 未闭合
	}
	if currentBuffer.Len() > 0 {
		parts = append(parts, segmentPart{false, currentBuffer.String()})
	}

	// 禁止相邻占位符 {a}{b}
	for i := 0; i < len(parts)-1; i++ {
		if parts[i].isPlaceholder && parts[i+1].isPlaceholder {
			return nil, false
		}
	}
	return parts, true
}
