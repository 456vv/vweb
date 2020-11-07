package vweb
import (
    "net/http"
	"golang.org/x/net/http/httpguts"
	"strings"
	"net/textproto"
	"strconv"
	"time"
)

type Cookier interface {
    ReadAll() map[string]string																						// 读取所有
    RemoveAll()																										// 删除所用
    Get(name string) string																							// 读取
    Add(name, value, path, domain string, maxAge int, secure, only bool, sameSite http.SameSite)					// 增加
    Del(name string)																								// 删除
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

func readCookies(h http.Header, filter string) []*http.Cookie {
	lines := h["Cookie"]
	if len(lines) == 0 {
		return []*http.Cookie{}
	}

	cookies := make([]*http.Cookie, 0, len(lines)+strings.Count(lines[0], ";"))
	for _, line := range lines {
		line = textproto.TrimString(line)

		var part string
		for len(line) > 0 { // continue since we have rest
			if splitIndex := strings.Index(line, ";"); splitIndex > 0 {
				part, line = line[:splitIndex], line[splitIndex+1:]
			} else {
				part, line = line, ""
			}
			part = textproto.TrimString(part)
			if len(part) == 0 {
				continue
			}
			name, val := part, ""
			if j := strings.Index(part, "="); j >= 0 {
				name, val = name[:j], name[j+1:]
			}
			if !isCookieNameValid(name) {
				continue
			}
			if filter != "" && filter != name {
				continue
			}
			val, ok := parseCookieValue(val, true)
			if !ok {
				continue
			}
			cookies = append(cookies, &http.Cookie{Name: name, Value: val})
		}
	}
	return cookies
}

func readSetCookies(h http.Header) []*http.Cookie {
	cookieCount := len(h["Set-Cookie"])
	if cookieCount == 0 {
		return []*http.Cookie{}
	}
	cookies := make([]*http.Cookie, 0, cookieCount)
	for _, line := range h["Set-Cookie"] {
		parts := strings.Split(textproto.TrimString(line), ";")
		if len(parts) == 1 && parts[0] == "" {
			continue
		}
		parts[0] = textproto.TrimString(parts[0])
		j := strings.Index(parts[0], "=")
		if j < 0 {
			continue
		}
		name, value := parts[0][:j], parts[0][j+1:]
		if !isCookieNameValid(name) {
			continue
		}
		value, ok := parseCookieValue(value, true)
		if !ok {
			continue
		}
		c := &http.Cookie{
			Name:  name,
			Value: value,
			Raw:   line,
		}
		
		for i := 1; i < len(parts); i++ {
			parts[i] = textproto.TrimString(parts[i])
			if len(parts[i]) == 0 {
				continue
			}

			attr, val := parts[i], ""
			if j := strings.Index(attr, "="); j >= 0 {
				attr, val = attr[:j], attr[j+1:]
			}
			lowerAttr := strings.ToLower(attr)
			val, ok = parseCookieValue(val, false)
			if !ok {
				c.Unparsed = append(c.Unparsed, parts[i])
				continue
			}
			switch lowerAttr {
			case "samesite":
				lowerVal := strings.ToLower(val)
				switch lowerVal {
				case "lax":
					c.SameSite = http.SameSiteLaxMode
				case "strict":
					c.SameSite = http.SameSiteStrictMode
				case "none":
					c.SameSite = http.SameSiteNoneMode
				default:
					c.SameSite = http.SameSiteDefaultMode
				}
				continue
			case "secure":
				c.Secure = true
				continue
			case "httponly":
				c.HttpOnly = true
				continue
			case "domain":
				c.Domain = val
				continue
			case "max-age":
				secs, err := strconv.Atoi(val)
				if err != nil || secs != 0 && val[0] == '0' {
					break
				}
				if secs <= 0 {
					secs = -1
				}
				c.MaxAge = secs
				continue
			case "expires":
				c.RawExpires = val
				exptime, err := time.Parse(time.RFC1123, val)
				if err != nil {
					exptime, err = time.Parse("Mon, 02-Jan-2006 15:04:05 MST", val)
					if err != nil {
						c.Expires = time.Time{}
						break
					}
				}
				c.Expires = exptime.UTC()
				continue
			case "path":
				c.Path = val
				continue
			}
			c.Unparsed = append(c.Unparsed, parts[i])
		}
		cookies = append(cookies, c)
	}
	return cookies
}

func parseCookieValue(raw string, allowDoubleQuote bool) (string, bool) {
	if allowDoubleQuote && len(raw) > 1 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	for i := 0; i < len(raw); i++ {
		if !validCookieValueByte(raw[i]) {
			return "", false
		}
	}
	return raw, true
}
func isCookieNameValid(raw string) bool {
	if raw == "" {
		return false
	}
	return strings.IndexFunc(raw, isNotToken) < 0
}
func isNotToken(r rune) bool {
	return !httpguts.IsTokenRune(r)
}
func validCookieValueByte(b byte) bool {
	return 0x20 <= b && b < 0x7f && b != '"' && b != ';' && b != '\\'
}