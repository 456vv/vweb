package vweb

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	mathRand "math/rand"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/456vv/verror"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// 自动从 Let's Encrypt 申请证书
//
//	ac *autocert.Manager	申请证书管理
//	tlsconf *tls.Config		tls配置
//	handler http.Handler	http处理
func AutoCert(ac *autocert.Manager, tlsconf *tls.Config, handler http.Handler) http.Handler {
	if ac != nil && tlsconf != nil {
		tlsconf.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			// 先使用内置证书，过期后使用自动证书
			now := time.Now().Add(ac.RenewBefore)
			var err error
			for _, cert := range tlsconf.Certificates {
				if cert.Leaf == nil {
					cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
					if err != nil {
						continue
					}
				}
				if !now.Before(cert.Leaf.NotBefore) && !now.After(cert.Leaf.NotAfter) && hello.SupportsCertificate(&cert) == nil {
					return &cert, nil
				}
			}

			// 自动证书
			cert, err := ac.GetCertificate(hello)
			if err != nil {
				// src\crypto\tls\common.go
				// func (c *Config) getCertificate(clientHello *ClientHelloInfo) (*Certificate, error)
				// 返回空跳过继续使用原证书
				return nil, nil
			}
			return cert, nil
		}

		// 没有配置"acme-tls/1"
		if !strSliceContains(tlsconf.NextProtos, acme.ALPNProto) {
			tlsconf.NextProtos = append(tlsconf.NextProtos, acme.ALPNProto)
		}

		return ac.HTTPHandler(handler)
	}

	return handler
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

// GenerateRandomId 生成标识符
//
//	[]byte  	生成的标识符
//	err error	错误
func GenerateRandomId(rnd []byte) error {
	if rnd == nil {
		return verror.TrackErrorf("vweb: The parameter is nil, unable to generate random data!")
	}
	if _, err := rand.Read(rnd); err != nil {
		// 当系统随机API函数不可用，将使用备用随机数。
		// 该功能在效力上也不理想。
		// 这可能是在万分之一的情况下使用到。
		source := mathRand.NewSource(0)
		rr := mathRand.New(source)
		for i := 0; i < len(rnd); i++ {
			rr.Seed(time.Now().UnixNano() + int64(i))
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
	rnd := make([]byte, length)
	err := GenerateRandomId(rnd)
	if err != nil {
		return nil, err
	}
	encodeLength := len(encodeStd)
	for i := 0; i < length; i++ {
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
		for i := 0; i < len(rnd); i++ {
			pos := int(rnd[i]^salt[start]) % encodeLength
			rnd[i] = encodeStd[pos]
			start++
			if start == sl {
				start = 0
			}
		}
	} else {
		for i := 0; i < len(rnd); i++ {
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
		errStr = "Access (%s) page document does not exist!"
		err    error
		// 为了防止跨目录，这里清除 /../ok/index.html 路径为 \ok\index.html，保证安全。
		pc = path.Clean(p)
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
		wait = (maxDelay / 100)
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

// strSliceContains 从切片中查找匹配的字符串
func strSliceContains(ss []string, c string) bool {
	for _, v := range ss {
		if v == c {
			return true
		}
	}
	return false
}
