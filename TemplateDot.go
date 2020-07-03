package vweb
import(
	"net/http"
    "github.com/456vv/vmap/v2"
    "context"
)

type DotContexter interface{
    Context() context.Context                                             					// 上下文
    WithContext(ctx context.Context)														// 替换上下文
}
// TemplateDoter 可以在模本中使用的方法
type TemplateDoter interface{
    RootDir(path string) string																// 网站的根目录
    Request() *http.Request                                                                 // 用户的请求信息
    RequestLimitSize(l int64) *http.Request                                                 // 请求限制大小
    Header() http.Header                                                                    // 标头
    Response() Responser                                                                    // 数据写入响应
    ResponseWriter() http.ResponseWriter                                                    // 数据写入响应
    Session() Sessioner                                                                     // 用户的会话缓存
    Global() Globaler                                                                       // 全站缓存
    Cookie() Cookier                                                                        // 用户的Cookie
    Swap() Swaper                                                                           // 信息交换
    Defer(call interface{}, args ... interface{}) error										// 退回调用
    DotContexter
}


//模板点
type TemplateDot struct {
    R     				*http.Request                                                               // 请求
    W    				http.ResponseWriter                                                         // 响应
    BuffSize			int64																		// 缓冲块大小
    Site       		 	*Site                                                                       // 网站配置
    Writed      		bool                                                                        // 表示已经调用写入到客户端。这个是只读的
    exchange       		vmap.Map                                                                    // 缓存映射
    ec					exitCall																	// 退回调用函数
    ctx					context.Context																// 上下文
}

//RootDir 网站的根目录
//	upath string	页面路径
//	string 			根目录
func (T *TemplateDot) RootDir(upath string) string {
	if T.Site != nil && T.Site.RootDir != nil {
		return T.Site.RootDir(upath)
	}
	return "."
}

//Request 用户的请求信息
//	*http.Request 请求
func (T *TemplateDot) Request() *http.Request {
    return T.R
}

//RequestLimitSize 请求限制大小
//	l int64         复制body大小
//	*http.Request   请求
func (T *TemplateDot) RequestLimitSize(l int64) *http.Request {
	T.R.Body = http.MaxBytesReader(T.W, T.R.Body, l)
	return T.R
}

//Header 标头
//	http.Header   响应标头
func (T *TemplateDot) Header() http.Header {
    return T.W.Header()
}

//Response 数据写入响应
//	Responser     响应
func (T *TemplateDot) Response() Responser {
    return &response{
    	buffSize: T.BuffSize,
        w		: T.W,
        r		: T.R,
        td		: T,
    }
}

//ResponseWriter 数据写入响应，http 的响应接口，调用这个接口后，模板中的内容就不会显示页客户端去
//	http.ResponseWriter      响应
func (T *TemplateDot) ResponseWriter() http.ResponseWriter {
    T.Writed = true
    return T.W
}

//Session 用户的会话缓存
//	Sessioner  会话缓存
func (T *TemplateDot) Session() Sessioner {	
	if T.Site == nil || T.Site.Sessions == nil {
		return nil
	}
    return T.Site.Sessions.Session(T.W, T.R)
}

//Global 全站缓存
//	Globaler	公共缓存
func (T *TemplateDot) Global() Globaler {
	if T.Site == nil || T.Site.Global == nil {
		return nil
	}
    return T.Site.Global
}

//Cookie 用户的Cookie
//	Cookier	接口
func (T *TemplateDot) Cookie() Cookier {
    return &Cookie{
        W:T.W,
        R:T.R,
    }
}

//Swap 信息交换
//	Swaper  映射
func (T *TemplateDot) Swap() Swaper {
    return &T.exchange
}

//Defer 在用户会话时间过期后，将被调用。
//	call interface{}            函数
//	args ... interface{}        参数或更多个函数是函数的参数
//	error                       错误
//  例：
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (T *TemplateDot) Defer(call interface{}, args ... interface{}) error {
    return T.ec.Defer(call, args...)
}

//Free 释放Defer
func (T *TemplateDot) Free() {
    T.ec.Free()
}

//Context 上下文
//	context.Context 上下文
func (T *TemplateDot) Context() context.Context {
	if T.ctx != nil {
		return T.ctx
	}
	return context.Background()
}

//WithContext 替换上下文
//	ctx context.Context 上下文
func (T *TemplateDot) WithContext(ctx context.Context) {
	if ctx == nil {
		panic("nil context")
	}
	T.ctx = ctx
}
