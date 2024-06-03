package vweb

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/456vv/vmap/v2"
)

type HandleFunc []func(*Dot)

func (T HandleFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d := &Dot{
		R:    r,
		W:    w,
		Site: r.Context().Value(SiteContextKey).(*Site),
		ctx:  r.Context(),
	}
	defer d.Close()

	for _, f := range T {
		f(d)
		if d.writed {
			return
		}
	}
}

type DotContexter interface {
	Context() context.Context        // 上下文
	WithContext(ctx context.Context) // 替换上下文
}

// Doter 可以在模本中使用的方法
type Doter interface {
	RootDir(path string) string             // 网站的根目录
	Request() *http.Request                 // 用户的请求信息
	RequestLimitSize(l int64) *http.Request // 请求限制大小
	Header() http.Header                    // 标头
	Response() Responser                    // 数据写入响应
	Session() Sessioner                     // 用户的会话缓存
	Global() Globaler                       // 全站缓存
	Cookie() Cookier                        // 用户的Cookie
	Swap() *vmap.Map                        // 信息交换
	Defer(call any, args ...any) error      // 退回调用
	SaveStatic(path string) error           // 保存为静态文件
	DotContexter                            // 上下文
}

// 模板点
type Dot struct {
	R          *http.Request       // 请求
	W          http.ResponseWriter // 响应
	BuffSize   int                 // 缓冲块大小
	Site       *Site               // 网站配置
	writed     bool                // 表示已经调用写入到客户端。这个是只读的
	exchange   vmap.Map            // 缓存映射
	ec         ExitCall            // 退回调用函数
	ctx        context.Context     // 上下文
	staticPath string              // 静态路径
	staticFile *os.File            // 静态文件
}

func (T *Dot) toStatic(b []byte) {
	if T.staticPath != "" {
		if T.staticFile != nil {
			T.staticFile.Write(b)
			return
		}
		var err error
		T.staticFile, err = os.CreateTemp("", "*.temp")
		if err != nil {
			log.Println(err)
		}
	}
}

// RootDir 网站的根目录
//
//	upath string	页面路径
//	string 			根目录
func (T *Dot) RootDir(upath string) string {
	if T.Site != nil && T.Site.RootDir != nil {
		return T.Site.RootDir(upath)
	}
	return "."
}

// Request 用户的请求信息
//
//	*http.Request 请求
func (T *Dot) Request() *http.Request {
	return T.R
}

// RequestLimitSize 请求限制大小
//
//	l int64         复制body大小
//	*http.Request   请求
func (T *Dot) RequestLimitSize(l int64) *http.Request {
	T.R.Body = http.MaxBytesReader(T.W, T.R.Body, l)
	return T.R
}

// Header 标头
//
//	http.Header   响应标头
func (T *Dot) Header() http.Header {
	return T.W.Header()
}

// Response 数据写入响应
//
//	Responser     响应
func (T *Dot) Response() Responser {
	return &response{
		buffSize: T.BuffSize,
		w:        T.W,
		r:        T.R,
		td:       T,
	}
}

// Session 用户的会话缓存
//
//	Sessioner  会话缓存
func (T *Dot) Session() Sessioner {
	if T.Site == nil || T.Site.Sessions == nil {
		return nil
	}
	return T.Site.Sessions.Session(T.W, T.R)
}

// Global 全站缓存
//
//	Globaler	公共缓存
func (T *Dot) Global() Globaler {
	if T.Site == nil || T.Site.Global == nil {
		return nil
	}
	return T.Site.Global
}

// Cookie 用户的Cookie
//
//	Cookier	接口
func (T *Dot) Cookie() Cookier {
	return &Cookie{
		W: T.W,
		R: T.R,
	}
}

// Swap 信息交换
//
//	Swaper  映射
func (T *Dot) Swap() *vmap.Map {
	return &T.exchange
}

// Defer 在用户会话时间过期后，将被调用。
//
//		call any            函数
//		args ... any        参数或更多个函数是函数的参数
//		error                       错误
//	 例：
//		.Defer(fmt.Println, "1", "2")
//		.Defer(fmt.Printf, "%s", "汉字")
func (T *Dot) Defer(call any, args ...any) error {
	return T.ec.Defer(call, args...)
}

// Close 释放
func (T *Dot) Close() error {
	T.ec.Free()
	if T.staticFile != nil {
		staticPath := filepath.Join(T.RootDir(T.staticPath) + T.staticPath)
		staticDir := filepath.Dir(staticPath)
		// 判断目录是否存在，不存在创建目录
		_, err := os.Stat(staticDir)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(staticDir, 0o644); err != nil {
				return err
			}
		}
		tempPath := T.staticFile.Name()
		T.staticFile.Close()
		T.staticFile = nil
		if err := os.Rename(tempPath, staticPath); err != nil {
			return err
		}
	}
	return nil
}

// Context 上下文
//
//	context.Context 上下文
func (T *Dot) Context() context.Context {
	if T.ctx != nil {
		return T.ctx
	}
	return context.Background()
}

// WithContext 替换上下文
//
//	ctx context.Context 上下文
func (T *Dot) WithContext(ctx context.Context) {
	if ctx == nil {
		panic("nil context")
	}
	T.ctx = ctx
}

// SaveStatic 保存为静态文件
//
//	path string	保存路径
func (T *Dot) SaveStatic(path string) error {
	T.staticPath = path
	return nil
}
