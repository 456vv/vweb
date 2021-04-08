package vweb

import(
	"sync"
	"regexp"
	"fmt"
	"net/http"
	"path"
	"strings"
)

type Route struct{
	HandlerError	func(w http.ResponseWriter, r *http.Request)	// 错误访问处理
	rt       		sync.Map										// 路由表 map[string]
}


//HandleFunc 绑定处理函数，匹配的网址支持正则，这说明你要严格的检查。
//	url string                                          网址，支持正则匹配
//	handler func(w ResponseWriter, r *Request)    		处理函数
func (T *Route) HandleFunc(url string,  handler func(w http.ResponseWriter, r *http.Request)){
	if handler == nil {
    	T.rt.Delete(url)
		return
	}
    T.rt.Store(url, http.HandlerFunc(handler))
}

//ServeHTTP 服务HTTP
//	w ResponseWriter    响应
//	r *Request          请求
func (T *Route) ServeHTTP(w http.ResponseWriter, r *http.Request){
	upath := r.URL.Path
	inf, ok := T.rt.Load(upath)
	if ok {
		inf.(http.Handler).ServeHTTP(w, r)
		if upath == r.URL.Path {
			return
		}
	}else{
		var handleFunc http.Handler
		T.rt.Range(func(k, v interface{}) bool {
			pattern := k.(string)
			//正则
			if strings.HasPrefix(pattern, "^") || strings.HasSuffix(pattern, "$") {
		        regexpRegexp, err := regexp.Compile(pattern)
		        if err != nil {
		            return true
		        }
		        _, complete := regexpRegexp.LiteralPrefix()
		        if !complete {
	           		regexpRegexp.Longest()
			        if regexpRegexp.MatchString(r.URL.Path) {
			        	ok = true
			            handleFunc = v.(http.Handler)
			            return false
			        }
		        }
				return true
			}
			//通配符
			matched, _ := path.Match(pattern, r.URL.Path)
			if matched {
	        	ok = true
	            handleFunc = v.(http.Handler)
	            return false
			}
			return true
		});
		if ok {
			handleFunc.ServeHTTP(w, r)
			if upath == r.URL.Path {
				return
			}
		}
	}
	
	//处理错误的请求
	if T.HandlerError != nil {
		T.HandlerError(w, r)
		return
	}
	
	//默认的错误处理
	w.Header().Set("Connection","close")
	http.Error(w, fmt.Sprintf("The path does not exist (%s)", upath), 404)
}