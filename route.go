package vweb

import(
	"sync"
	"regexp"
	"fmt"
	"net/http"
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
	inf, ok := T.rt.Load(r.URL.Path)
	if ok {
		inf.(http.Handler).ServeHTTP(w, r)
		return
	}else{
		var handleFunc http.Handler
		T.rt.Range(func(k, v interface{}) bool {
	        regexpRegexp, err := regexp.Compile(k.(string))
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
		});
		if ok {
			handleFunc.ServeHTTP(w, r)
			return
		}
	}
	
	//处理错误的请求
	if T.HandlerError != nil {
		T.HandlerError(w, r)
		return
	}
	
	//默认的错误处理
	w.Header().Set("Connection","close")
	http.Error(w, fmt.Sprintf("The path does not exist (%s)", r.URL.Path), 404)
}