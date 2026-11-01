package vweb

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	mathRand "math/rand"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/456vv/vmap/v2"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// AutoCert 自动从 Let's Encrypt 申请证书
//
//	ac *autocert.Manager	申请证书管理
//	tlsconf *tls.Config		tls配置
//	handler http.Handler	http处理
func AutoCert(ac *autocert.Manager, tlsconf *tls.Config, handler http.Handler) http.Handler {
	if ac == nil {
		return handler
	}
	if tlsconf == nil {
		return ac.HTTPHandler(handler)
	}

	// GetCertificate 的 会话在多个连接上并发调用 Get，，必须用锁保护。
	// TLS 握手会在多个连接上并发调用 GetCertificate，必须用锁保护。
	var certMu sync.Mutex

	tlsconf.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		now := time.Now().Add(ac.RenewBefore)

		certMu.Lock()
		certs := tlsconf.Certificates
		pos := -1
		for i := range certs {
			cert := &certs[i]
			if cert.Leaf == nil {
				leaf, err := x509.ParseCertificate(cert.Certificate[0])
				if err != nil {
					continue
				}
				cert.Leaf = leaf
			}
			if hello.SupportsCertificate(cert) == nil {
				if !now.Before(cert.Leaf.NotBefore) && !now.After(cert.Leaf.NotAfter) {
					certMu.Unlock()
					return cert, nil
				}
				// 记录过期证书位置（包含 0 号位）
				pos = i
			}
		}
		certMu.Unlock()

		// 自动证书
		cert, err := ac.GetCertificate(hello)
		if err != nil {
			// 返回 nil,nil 让 tls 继续使用默认逻辑
			log.Println(err)
			return nil, nil
		}
		if pos >= 0 {
			certMu.Lock()
			if pos < len(tlsconf.Certificates) {
				tlsconf.Certificates[pos] = *cert
			}
			certMu.Unlock()
		}
		return cert, nil
	}

	// 没有配置 "acme-tls/1" 协议，自动添加
	if !slices.Contains(tlsconf.NextProtos, acme.ALPNProto) {
		tlsconf.NextProtos = append(tlsconf.NextProtos, acme.ALPNProto)
	}
	return ac.HTTPHandler(handler)
}

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
	h, portStr, err := net.SplitHostPort(host)
	if err != nil {
		h, portStr = host, ""
	}
	var portSuffix string
	if portStr != "" {
		portSuffix = ":" + portStr
	}
	if h == "" {
		return
	}
	// 兼容 "[::1]"（无端口但带方括号）写法
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	}
	// IP 字面量不存在域名级通配，直接结束
	if net.ParseIP(h) != nil {
		return
	}
	labels := strings.Split(h, ".")
	for i := range labels {
		labels[i] = "*"
		candidate := strings.Join(labels, ".") + portSuffix
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

// GenerateRandomID 生成标识符
//
//	[]byte  	生成的标识符
//	err error	错误
func GenerateRandomID(rnd []byte) error {
	if rnd == nil {
		return errors.New("vweb: The parameter is nil, unable to generate random data")
	}
	if _, err := rand.Read(rnd); err != nil {
		// 当系统随机API函数不可用，将使用备用随机数。
		// 该功能在效力上也不理想。
		// 这可能是在万分之一的情况下使用到。
		seed := time.Now().UnixNano()
		source := mathRand.NewSource(seed)
		rr := mathRand.New(source)
		for i := range rnd {
			r := rr.Int()
			rnd[i] = byte(r)
		}
	}
	return nil
}

// GenerateRandom 生成标识符
//
//	length int	长度
//	[]byte  	生成的标识符
//	err error	错误
func GenerateRandom(length int) ([]byte, error) {
	if length < 0 {
		return nil, errors.New("vweb: length must be non-negative")
	}
	rnd := make([]byte, length)
	err := GenerateRandomID(rnd)
	if err != nil {
		return nil, err
	}
	encodeLength := len(encodeStd)
	for i := range length {
		pos := int(rnd[i]) % encodeLength
		rnd[i] = encodeStd[pos]
	}
	return rnd, nil
}

// GenerateRandomString 生成标识符
//
//	length int	长度
//	string  	生成的标识符
//	err error	错误
func GenerateRandomString(length int) (string, error) {
	r, err := GenerateRandom(length)
	return string(r), err
}

// AddSalt 加盐
//
//	rnd []byte	标识字节串
//	salt string	盐
//	string  	标识符
func AddSalt(rnd []byte, salt string) string {
	var (
		start        int
		sl           = len(salt)
		encodeLength = len(encodeStd)
	)
	if sl != 0 {
		for i := range rnd {
			pos := int(rnd[i]^salt[start]) % encodeLength
			rnd[i] = encodeStd[pos]
			start++
			if start == sl {
				start = 0
			}
		}
	} else {
		for i := range rnd {
			pos := int(rnd[i]) % encodeLength
			rnd[i] = encodeStd[pos]
		}
	}
	return string(rnd)
}

// PagePath 页路径
//
//	root string		    根目录
//	p string            路径
//	index []string      索引文件名
//	os.FileInfo         文件信息
//	string          	路径
//	error				错误，如果文件不存在
func PagePath(root, p string, index []string) (os.FileInfo, string, error) {
	var (
		isDir  = strings.HasSuffix(p, "/")
		fi     os.FileInfo
		errStr = "access (%s) page document does not exist"
		err    error
		// 为了防止跨目录，这里清除 /../ok/index.html 路径为 \ok\index.html，保证安全。
		pc = path.Clean("/" + p)
		pi string // path+fileName
	)
	if !isDir {
		fi, err = os.Stat(path.Join(root, pc))
		if err != nil {
			return nil, "", fmt.Errorf(errStr, p)
		}
		isDir = fi.IsDir()
		if !isDir {
			// 是文件
			return fi, pc, nil
		}
	}

	// 是目录，查找默认索引文件
	for _, v := range index {
		pi = path.Join(pc, v)
		fi, err = os.Stat(path.Join(root, pi))
		if err == nil && !fi.IsDir() {
			return fi, pi, nil
		}
	}
	return nil, "", fmt.Errorf(errStr, p)
}

func delay(wait, maxDelay time.Duration) time.Duration {
	if wait == 0 {
		wait = maxDelay / 100
	} else if wait > maxDelay/2 {
		wait = maxDelay
	} else {
		wait *= 2
	}
	if wait >= maxDelay {
		wait = maxDelay
	}
	time.Sleep(wait)
	return wait
}

// ExecFunc 执行函数调用
//
//	call any            函数
//	args ... any        参数或更多个函数是函数的参数
//	[]any				返回直
//	error                       错误
func ExecFunc(f any, args ...any) ([]any, error) {
	var ef ExecCall
	if err := ef.Func(f, args...); err != nil {
		return nil, err
	}
	return ef.Exec(), nil
}

// 判断是否是真正的正则表达式（简单判断）
func isRegex(re *regexp.Regexp) bool {
	prefix, complete := re.LiteralPrefix()
	return !complete || prefix == ""
}

func closeAllMap(v any) {
	if m, ok := v.(*vmap.Map); ok {
		m.Range(func(key any, value any) bool {
			closeAllMap(value)
			return true
		})
		m.Close()
	}
}
