package vweb

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

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

	// 私有证书快照。crypto/tls 在 GetCertificate 返回 (nil, nil) 时会无锁读取
	// tlsconf.Certificates，因此不能再原地改写该切片。
	var certMu sync.Mutex
	certs := slices.Clone(tlsconf.Certificates)

	// 保存先前安装的 GetCertificate，匹配失败时回退调用，
	// 避免覆盖共享 tlsconf 上的其它证书逻辑。
	prevGetCert := tlsconf.GetCertificate
	tlsconf.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// TLS-ALPN-01 验证握手必须由 autocert 应答
		if slices.Contains(hello.SupportedProtos, acme.ALPNProto) {
			return ac.GetCertificate(hello)
		}

		now := time.Now()
		renewAt := now.Add(ac.RenewBefore)

		certMu.Lock()
		pos := -1
		for i := range certs {
			cert := &certs[i]
			if cert.Leaf == nil {
				if len(cert.Certificate) == 0 {
					continue // 空证书链，原实现在这里越界 panic
				}
				leaf, err := x509.ParseCertificate(cert.Certificate[0])
				if err != nil {
					continue
				}
				cert.Leaf = leaf
			}
			if hello.SupportsCertificate(cert) == nil {
				if !now.Before(cert.Leaf.NotBefore) && !renewAt.After(cert.Leaf.NotAfter) {
					// 返回副本：之后替换 certs[pos] 不会改写正在握手中使用的对象。
					c := *cert
					certMu.Unlock()
					return &c, nil
				}
				pos = i // 记录即将过期的证书位置（包含 0 号位）
			}
		}
		certMu.Unlock()

		cert, err := ac.GetCertificate(hello)
		if err != nil {
			if prevGetCert != nil {
				return prevGetCert(hello)
			}
			log.Println(err)
			// 返回 nil,nil 让 tls 继续使用 tlsconf.Certificates
			return nil, nil
		}
		if pos >= 0 {
			certMu.Lock()
			if pos < len(certs) {
				certs[pos] = *cert
			}
			certMu.Unlock()
		}
		return cert, nil
	}

	// TLS-ALPN-01 挑战要求服务端 NextProtos 包含 acme-tls/1；
	// 正常访问需要 http/1.1 作为最小兼容基线，缺失时补上（不自动补 h2：
	// 用户自定义 CipherSuites 时 http.Server 可能未启用 HTTP/2）。
	if !slices.Contains(tlsconf.NextProtos, "http/1.1") {
		tlsconf.NextProtos = append(slices.Clone(tlsconf.NextProtos), "http/1.1")
	}
	if !slices.Contains(tlsconf.NextProtos, acme.ALPNProto) {
		tlsconf.NextProtos = append(slices.Clone(tlsconf.NextProtos), acme.ALPNProto)
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
	// 修改原因：会话标识失败时不能退化为可预测的时间种子。
	if _, err := rand.Read(rnd); err != nil {
		clear(rnd)
		return fmt.Errorf("vweb: generate secure random data: %w", err)
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
			return nil, "", fmt.Errorf("%s", fmt.Sprintf(errStr, p))
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
	return nil, "", fmt.Errorf("%s", fmt.Sprintf(errStr, p))
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

// classifyFileError 把文件系统错误归类为哨兵，供 ServeHTTP 映射状态码；
// 未识别的错误保留底层原因（%w），走 500。
func classifyFileError(err error, rel string) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return errors.Join(errFileNotFound, fmt.Errorf("%s: %w", rel, err))
	case errors.Is(err, fs.ErrPermission):
		return errors.Join(errPermissionDenied, fmt.Errorf("%s: %w", rel, err))
	default:
		return fmt.Errorf("vweb: file access error (%s): %w", rel, err)
	}
}
