package server

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/456vv/vweb/v2/server/config"
)

// derogatoryDomain 贬域名
//
//	host string             host地址
//	f func(string) bool     调用 f 函数，并传入贬域名
func derogatoryDomain(host string, f func(string) bool) {
	// 先全字匹配
	if f(host) {
		return
	}
	// 后通配符匹配
	pos := strings.Index(host, ":")
	var port string
	if pos >= 0 {
		port = host[pos:]
		host = host[:pos]
	}
	labels := strings.Split(host, ".")
	for i := range labels {
		labels[i] = "*"
		candidate := strings.Join(labels, ".") + port
		if f(candidate) {
			break
		}
	}
}

// equalDomain 贬域名
//
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

// strSliceContains 从切片中查找匹配的字符串
func strSliceContains(ss []string, c string) bool {
	for _, v := range ss {
		if v == c {
			return true
		}
	}
	return false
}

func inDirect(v reflect.Value) reflect.Value {
	for ; v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface; v = v.Elem() {
	}
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

func staticAt(T *ServerGroup, cacheStaticFileDir string, dynamic config.SiteDynamic) func(u *url.URL, r io.Reader, l int) (int, error) {
	return func(u *url.URL, r io.Reader, l int) (int, error) {
		// 存储路径
		var (
			fileDir  string
			filePath string
		)
		if fileExt := path.Ext(u.Path); fileExt != "" {
			// 这是文件

			// 后缀名称是动态扩展名称，不支持保存
			for _, ext := range dynamic.Ext {
				if ext == fileExt {
					return 0, nil
				}
			}

			fileDir = path.Dir(u.Path)
			filePath = u.Path
		} else {
			// 这是目录
			fileDir = u.Path
			filePath = path.Join(fileDir, "index.html")
		}

		// 判断有没有符合的路径
		var (
			matched bool
			err     error
		)
		for _, spath := range dynamic.CacheStaticAllowPath {
			matched, err = path.Match(spath, filePath)
			if err != nil {
				T.ErrorLog.Printf("server: Dynamic.CacheStaticPaths 通配符格式不正确：%s, %s\n", spath, err.Error())
				continue
			}
			if matched {
				break
			}
		}
		if !matched {
			return 0, nil
		}

		// 目录创建
		fileDir = filepath.Join(cacheStaticFileDir, fileDir)
		if err = os.MkdirAll(fileDir, 0o644); err != nil {
			T.ErrorLog.Printf("server: 创建静态文件目录失败，路径：%s, 错误：%s\n", fileDir, err.Error())
			return 0, nil
		}

		// 文件保存
		filePath = filepath.Join(cacheStaticFileDir, filePath)
		if fi, err := os.Stat(filePath); err == nil {
			currTime := time.Now()
			mTime := fi.ModTime()
			cSecond := time.Duration(dynamic.CacheStaticTimeout)
			// 文件修改时间+允许缓存时间 大于 当时时间，跳过
			// 文件大小一样，跳过
			if mTime.Add(cSecond).After(currTime) && fi.Size() == int64(l) {
				return 0, nil
			}
		}

		osFile, err := os.Create(filePath)
		if err != nil {
			T.ErrorLog.Printf("server: 静态文件保存发生错误，路径：%s, 错误：%s\n", filePath, err.Error())
			return 0, nil
		}
		defer osFile.Close()

		if n, err := io.Copy(osFile, r); err != nil {
			T.ErrorLog.Printf("server: 静态文件保存发生错误，路径：%s, 预期长度：%d, 结果长度：%d, 错误：%s\n", filePath, l, n, err.Error())
		}
		return 0, nil
	}
}

func headerAdd(wh http.Header, mht map[string]config.SiteHeaderType, fileExt string) {
	var ht config.SiteHeaderType
	if h, ok := mht[fileExt]; ok {
		ht = h
	} else if h, ok := mht["*"]; ok {
		ht = h
	}
	for k, v := range ht.Header {
		for _, v1 := range v {
			wh.Add(k, v1)
		}
	}
}
