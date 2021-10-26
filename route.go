package vweb

import(
	"sync"
	"regexp"
	"fmt"
	"net/http"
	"path"
	"strings"
	"context"
)



type Route struct{
	HandlerError	http.HandlerFunc	// 错误访问处理
	rt       		sync.Map			// 路由表 map[string]
	siteMan			*SiteMan
}

//SetSiteMan 设置站点管理，将会携带在请求上下文中
//siteMan *SiteMan	站点
func (T *Route) SetSiteMan(siteMan *SiteMan) {
	T.siteMan = siteMan
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
	
	ctx := r.Context()
	
	//存在站点管理，检查host读取站点
	if T.siteMan != nil {
	    //** 检查Host是否存在
	    site, ok := T.siteMan.Get(r.Host)
	    if !ok {
	        //如果在站点集中没有找到存在的Host，则关闭连接。
			hj, ok := w.(http.Hijacker)
			if !ok {
	            //500 服务器遇到了意料不到的情况，不能完成客户的请求。
				http.Error(w, "Not supported Hijacker", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
	            //500 服务器遇到了意料不到的情况，不能完成客户的请求。
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			//直接关闭连接
			defer conn.Close()
	        return
	    }
	    ctx = context.WithValue(ctx, "Site", site)
	}
	
	//处理 HandleFunc
	upath := r.URL.Path
	req := r

	forkReq := func() {
		if r == req {
			req = r.WithContext(ctx)
		}
	}

	inf, ok := T.rt.Load(upath)
	if ok {
		forkReq()
		inf.(http.Handler).ServeHTTP(w, req)
		if upath == req.URL.Path {
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
			        if regexpRegexp.MatchString(upath) {
			        	ok = true
			            handleFunc = v.(http.Handler)
			            return false
			        }
		        }
				return true
			}
			//通配符
			matched, _ := path.Match(pattern, upath)
			if matched {
	        	ok = true
	            handleFunc = v.(http.Handler)
	            return false
			}
			return true
		});
		if ok {
			forkReq()
			handleFunc.ServeHTTP(w, req)
			if upath == req.URL.Path {
				return
			}
		}
	}
	
	//处理错误的请求
	if T.HandlerError != nil {
		forkReq()
		T.HandlerError.ServeHTTP(w, req)
		return
	}
	
	//默认的错误处理
	w.Header().Set("Connection","close")
	http.Error(w, fmt.Sprintf("The path does not exist (%s)", upath), 404)
}