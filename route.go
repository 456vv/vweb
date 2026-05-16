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
}

type Route struct {
	HandlerError http.HandlerFunc // 错误访问处理
	rt           sync.Map         // 路由表 map[string]
	list         []*routePathPlaceholder
	siteMan      *SiteMan
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
		T.rt.Delete(url)
		return
	}

	T.addHandle(url, http.Handler(http.HandlerFunc(handler)))
}

func (T *Route) HandleFuncDot(url string, handler ...func(Doter)) {
	if handler == nil {
		T.rt.Delete(url)
		return
	}

	T.addHandle(url, http.Handler(HandleFunc(handler)))
}

func (T *Route) addHandle(url string, handle http.Handler) {
	pathPlaceholder := &routePathPlaceholder{
		isDir:        strings.HasSuffix(url, "/"),
		pathSegments: strings.Split(url, "/"),
		handle:       handle,
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
	}

	T.rt.Store(url, pathPlaceholder)
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

	// 处理 HandleFunc
	userPath := r.URL.Path

	inf, ok := T.rt.Load(userPath)
	if !ok {
		filteredUserSegments := strings.Split(userPath, "/")
		isDir := strings.HasSuffix(userPath, "/")

		T.rt.Range(func(k, v any) bool {
			pathPattern := k.(string)
			inf = v
			pathPlaceholder := v.(*routePathPlaceholder)

			//  基本检查：用户路径是否为目录必须与占位符路径的目录性匹配
			if pathPlaceholder.isDir != isDir {
				return true
			}

			// 正则检查路径是否匹配
			if pathPlaceholder.isRegexp {
				ok = pathPlaceholder.pathRegex.MatchString(userPath)
				return !ok
			}

			// 通配符
			if pathPlaceholder.isSymbol {
				ok, _ = path.Match(pathPattern, userPath)
				return !ok
			}

			// 占位符变量判断
			if pathPlaceholder.isPlaceholder {
				var param map[string]string
				if param, ok = parsePathParams(filteredUserSegments, pathPlaceholder.pathSegments); ok {
					r = r.WithContext(context.WithValue(r.Context(), "url-params", param))
					return false
				}
			}

			return true // false 退出
		})
	}

	if ok {
		pathPlaceholder := inf.(*routePathPlaceholder)
		pathPlaceholder.handle.ServeHTTP(w, r)
		return
	}

	// 处理错误的请求
	if T.HandlerError != nil {
		T.HandlerError.ServeHTTP(w, r)
		return
	}

	// 默认的错误处理
	w.Header().Set("Connection", "close")
	http.Error(w, fmt.Sprintf("The path does not exist (%s)", userPath), 404)
}

// segmentPart 结构用于内部解析占位符片段，区分文字部分和占位符名称
type segmentPart struct {
	isPlaceholder bool   // true 表示是占位符 (如 "id"), false 表示是文字 (如 "b")
	value         string // 占位符名称或文字内容
}

// parsePathParams1 根据 userPath 和 pathPlaceholder 解析路径参数
// 不使用正则表达式
func parsePathParams(filteredUserSegments, pathSegments []string) (result map[string]string, ok bool) {
	// 基本检查：用户路径的段数必须与占位符路径的段数匹配
	if len(filteredUserSegments) != len(pathSegments) {
		return nil, false
	}

	result = make(map[string]string)
	// 遍历每个段进行匹配和参数提取
	for i := range filteredUserSegments {
		placeholderSegment := pathSegments[i]
		userSegment := filteredUserSegments[i]

		// 检查占位符段是否包含占位符标记 "{}"
		if !strings.Contains(placeholderSegment, "{") && !strings.Contains(placeholderSegment, "}") {
			// 如果不包含占位符，说明这是纯文字段，必须精确匹配
			if placeholderSegment != userSegment {
				return nil, false // 文字段不匹配，返回空 map
			}
			// 无需提取参数，继续下一个段
			continue
		}

		// 段中包含占位符，调用辅助函数提取参数
		params, ok := extractParamsFromSegment(placeholderSegment, userSegment)
		if !ok {
			// 提取失败（例如格式不匹配或占位符语法错误），返回空 map
			return nil, false
		}

		// 将提取到的参数合并到最终结果 map 中
		maps.Copy(result, params)
	}

	return result, true
}

// extractParamsFromSegment 负责从一个包含占位符的段中提取实际参数值。
// 例如，placeholderSeg="b{id}b", userSeg="b10b"，将提取 "id":"10"。
// 成功返回参数map和true，失败返回nil和false。
func extractParamsFromSegment(placeholderSeg, userSeg string) (map[string]string, bool) {
	result := make(map[string]string)

	// 首先解析占位符段，得到文字和占位符的序列
	parsedParts, ok := parsePlaceholderSegment(placeholderSeg)
	if !ok {
		return nil, false // 占位符段本身有语法错误
	}

	userSegIdx := 0 // 当前在 userSeg 中匹配到的位置

	for i, part := range parsedParts {
		if !part.isPlaceholder { // 当前部分是文字 (literal)
			// 检查 userSeg 中是否有足够的长度来匹配这个文字部分
			if userSegIdx+len(part.value) > len(userSeg) {
				return nil, false // userSeg 太短，无法匹配文字
			}
			// 检查文字部分是否匹配
			if userSeg[userSegIdx:userSegIdx+len(part.value)] != part.value {
				return nil, false // 文字不匹配
			}
			userSegIdx += len(part.value) // 移动 userSeg 的匹配指针
		} else { // 当前部分是占位符 (placeholder)
			var endOfParam int          // 参数值在 userSeg 中的结束索引
			if i+1 < len(parsedParts) { // 如果后面还有部分（必须是文字，因为相邻占位符已被禁止）
				nextPart := parsedParts[i+1]
				// 寻找下一个文字部分在 userSeg 剩余部分中的位置，作为当前占位符值的结束标记
				nextLiteral := nextPart.value
				foundIdx := strings.Index(userSeg[userSegIdx:], nextLiteral)
				if foundIdx == -1 {
					return nil, false // 下一个文字分隔符未找到
				}
				endOfParam = userSegIdx + foundIdx // 计算参数值的结束索引
			} else { // 如果这是最后一个部分，那么占位符值就是 userSeg 的剩余全部
				endOfParam = len(userSeg)
			}

			// 确保起始索引不大于结束索引，否则参数值将是负长度（逻辑错误）
			// 如果 userSegIdx == endOfParam，表示参数值为空字符串，这通常是允许的
			if userSegIdx > endOfParam {
				return nil, false
			}

			paramValue := userSeg[userSegIdx:endOfParam] // 提取参数值
			result[part.value] = paramValue              // 存储参数
			userSegIdx = endOfParam                      // 移动 userSeg 的匹配指针
		}
	}

	// 遍历完所有部分后，userSeg 的匹配指针应该恰好到达 userSeg 的末尾
	if userSegIdx != len(userSeg) {
		return nil, false // userSeg 有未匹配的剩余字符
	}

	return result, true // 成功提取所有参数
}

// parsePlaceholderSegment 辅助函数，将占位符段（如 "b{id}b"）解析成
// 包含文字和占位符名称的 segmentPart 序列。
// 成功返回解析后的序列和true，失败返回nil和false（表示格式错误）。
func parsePlaceholderSegment(placeholderSeg string) ([]segmentPart, bool) {
	parts := []segmentPart{}

	currentBuffer := ""    // 用于构建当前文字或占位符名称
	inPlaceholder := false // 标志是否在占位符内部

	for i := 0; i < len(placeholderSeg); i++ {
		char := placeholderSeg[i]

		if char == '{' {
			if inPlaceholder { // 在占位符内部又遇到 '{'，如 "{id{sub}}"，视为格式错误
				return nil, false
			}
			if currentBuffer != "" { // 如果 '{' 前有内容，说明是文字部分
				parts = append(parts, segmentPart{false, currentBuffer})
				currentBuffer = ""
			}
			inPlaceholder = true // 进入占位符模式
			continue
		}

		if char == '}' {
			if !inPlaceholder { // 在非占位符模式下遇到 '}'，如 "foo}bar"，视为格式错误
				return nil, false
			}
			if currentBuffer == "" { // 占位符名称不能为空，如 "{}"，视为格式错误
				return nil, false
			}
			parts = append(parts, segmentPart{true, currentBuffer}) // 添加占位符部分
			currentBuffer = ""
			inPlaceholder = false // 退出占位符模式
			continue
		}

		currentBuffer += string(char) // 累积字符到当前缓冲区
	}

	if inPlaceholder { // 遍历结束后，如果仍在占位符模式中，说明占位符未闭合，如 "{param"
		return nil, false
	}

	if currentBuffer != "" { // 如果最后还有剩余内容，说明是文字部分
		parts = append(parts, segmentPart{false, currentBuffer})
	}

	// 额外检查：确保没有两个占位符直接相邻，如 "{param1}{param2}"。
	// 这种情况下很难不使用分隔符进行值提取，所以约定为不支持。
	for i := 0; i < len(parts)-1; i++ {
		if parts[i].isPlaceholder && parts[i+1].isPlaceholder {
			return nil, false // 相邻占位符，格式错误
		}
	}

	return parts, true // 成功解析
}
