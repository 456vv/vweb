package server

import (
	"reflect"
	"strings"
	"github.com/456vv/vcipher"
	"github.com/456vv/verifycode"
    "github.com/456vv/vforward"
    "github.com/456vv/vbody"
    "github.com/456vv/vweb/v2/builtin"
)	


var _ = builtin.Init
var _ = vcipher.AES
var _ *verifycode.Color
var _ *vforward.Addr
var _ *vbody.Reader

//derogatoryDomain 贬域名
//	host string             host地址
//	f func(string) bool     调用 f 函数，并传入贬域名
func derogatoryDomain(host string, f func(string) bool){
	//先全字匹配
    if f(host) {
    	return
    }
    //后通配符匹配
	pos := strings.Index(host, ":")
	var port string
	if pos >= 0 {
		port = host[pos:]
		host = host[:pos]
	}
	labels := strings.Split(host, ".")
	for i := range labels {
		labels[i]="*"
		candidate := strings.Join(labels, ".")+port
        if f(candidate) {
        	break
        }
	}
}


//equalDomain 贬域名
//	host string             host地址
//	domain string			贬域名
//	ok bool					如果相等，返回true
func equalDomain(host, domain string) (ok bool) {
	derogatoryDomain(host, func(d string) bool {
		ok = (d == domain)
		return ok
	})
	return
}


//strSliceContains 从切片中查找匹配的字符串
func strSliceContains(ss []string, c string) bool {
	for _, v := range ss {
		if v == c {
			return true
		}
	}
	return false
}

func inDirect(v reflect.Value) reflect.Value {
	for ; v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface; v = v.Elem() {}
    return v
}

func isTrue(val reflect.Value) bool {
	if !val.IsValid() {
		return false
	}
	switch val.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return val.Len() > 0
	case reflect.Bool:
		return val.Bool()
	case reflect.Complex64, reflect.Complex128:
		return val.Complex() != 0
	case reflect.Chan, reflect.Func, reflect.Ptr, reflect.Interface:
		return !val.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return val.Int() != 0
	case reflect.Float32, reflect.Float64:
		return val.Float() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return val.Uint() != 0
	case reflect.Struct:
		return true
	}
	return false
}


