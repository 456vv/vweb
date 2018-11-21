package vweb
import (
    "net/http"
)

type Cookier interface {
	ReadAll() map[string]string
	RemoveAll()
	Get(name string) string
	Add(name, value, path, domain string, maxAge int, secure, only bool, sameSite http.SameSite)
	Del(name string)
}


type Cookie struct{
    R   *http.Request            	//请求
    W   http.ResponseWriter      	//响应
}


//All 读出所有Cookie
//	map[string]string        读取出所有Cookie
func (c *Cookie) ReadAll() map[string]string {
    cookie := make(map[string]string)
    httpCookie :=  c.R.Cookies()
    for _, v := range httpCookie {
        cookie[v.Name] = v.Value
    }
    return cookie
}

//RemoveAll 移除所有Cookie
func (c *Cookie) RemoveAll() {
    httpCookie :=  c.R.Cookies()
    for _, v := range httpCookie {
        c.Del(v.Name)
    }
}

//Get 读取Cookie，如果不存在返回空字符
//	name string     指定名称读取的Cookie
//	string          返回值
func (c *Cookie) Get(name string) string {
    httpCookie, err := c.R.Cookie(name)
    if err != nil {return ""}
    return httpCookie.Value
}

//Add 增加,写入一条Cookie，可以写入多条Cookie保存至浏览器
//	name string         	名称
//	value string        	值
//	path string         	路径
//	domain string       	域
//	maxAge int          	过期时间，以毫秒为单位
//	secure bool         	源，如果通过 SSL 连接 (HTTPS) 传输 Cookie，则为 true；否则为 false。默认值为 false。
//	only bool           	验证，如果 Cookie 具有 HttpOnly 属性且不能通过客户端脚本(JS)访问，则为 true；否则为 false。默认为 false。
//	sameSite http.SameSite	是否严格模式
func (c *Cookie) Add(name, value, path, domain string, maxAge int, secure, only bool, sameSite http.SameSite) {
    var cookie = &http.Cookie{
        Name: name,
        Value: value,
        Path: path,
        Domain: domain,
        //Expires: time.Now(),
        MaxAge: maxAge,
        Secure: secure,
        HttpOnly: only,
        SameSite: sameSite,
    }
    c.W.Header().Add("Set-Cookie", cookie.String())
}

//Del 删除，删除指定的Cookie
//	name string            名称
func (c *Cookie) Del(name string) {
    c.Add(name, "", "/", "", -1, false, false, 0)
}
