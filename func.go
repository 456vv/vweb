package vweb

import(
	"strings"
	"time"
    "crypto/rand"
    mathRand "math/rand"
    "encoding/base64"
	"os"
	"path"
	"github.com/456vv/verror"
	"text/template"
	"fmt"
)


//ExtendTemplatePackage 扩展模板的包
//	pkgName string					包名
//	deputy map[string]interface{} 	函数集
func ExtendTemplatePackage(pkgName string, deputy template.FuncMap) {
	if _, ok := dotPackage[pkgName]; !ok {
		dotPackage[pkgName] = make(template.FuncMap)
	}
	for name, fn  := range deputy {
		dotPackage[pkgName][name]=fn
	}
}

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

//GenerateRandomId 生成标识符
//	[]byte  	生成的标识符
//	err error	错误
func GenerateRandomId(rnd []byte) error {
    if rnd == nil {
    	return verror.TrackErrorf("vweb: The parameter is nil, unable to generate random data!")
    }
    if _, err := rand.Read(rnd); err != nil {
	    //当系统随机API函数不可用，将使用备用随机数。
	    //该功能在效力上也不理想。
	    //这可能是在万分之一的情况下使用到。
	    source := mathRand.NewSource(0)
	    rr := mathRand.New(source)
	    for i:=0; i<len(rnd); i++ {
	        rr.Seed(time.Now().UnixNano()+int64(i))
	        r := rr.Int()
	        rnd[i]=byte(r)
	    }
    }
    return  nil
}

//GenerateRandom 生成标识符
//	length int	长度
//	[]byte  	生成的标识符
//	err error	错误
func GenerateRandom(length int) ([]byte, error){
	rnd := make([]byte, length)
	err := GenerateRandomId(rnd)
	if err != nil {
		return nil, err
	}
	return rnd, nil
}

//GenerateRandomString 生成标识符
//	length int	长度
//	string  	生成的标识符
//	err error	错误
func GenerateRandomString(length int) (string, error){
	b, err := GenerateRandom(length)
	if err != nil {
		return "", err
	}
	base64Encoding := base64.NewEncoding(encodeStd).WithPadding(base64.StdPadding).Strict()
	base64Encoding.EncodedLen(length)
	return base64Encoding.EncodeToString(b)[:length], nil
}

//AddSalt 加盐
//	rnd []byte	标识字节串
//	salt string	盐
//	string  	标识符
func AddSalt(rnd []byte, salt string) string {
    var (
        start 	int
	    length	= len(salt)
	    l		= len(rnd)
    )
    if l == 0 {
    	return ""
    }
    if length != 0 {
	    for i:=0; i<l; i++ {
	    	rnd[i] = rnd[i] ^ salt[start]
	       	start++
	        if start == length {
	        	start = 0
	        }
	    }
    }
    
	base64Encoding := base64.NewEncoding(encodeStd).WithPadding(base64.StdPadding).Strict()
	base64Encoding.EncodedLen(length)
	return base64Encoding.EncodeToString(rnd)[:l]
}


//PagePath 页路径
//	root string		    根目录
//	p string            路径
//	index []string      索引文件名
//	os.FileInfo         文件信息
//	string          	路径
//	error				错误，如果文件不存在
func PagePath(root, p string, index []string) (os.FileInfo, string, error) {
    var (
    	isDir	= strings.HasSuffix(p, "/")
        fi		os.FileInfo
        errStr	= "Access (%s) page document does not exist!"
        err		error
        //为了防止跨目录，这里清除 /../ok/index.html 路径为 \ok\index.html，保证安全。
        pc		= path.Clean(p)
        pi		string	//path+fileName
    )
    if !isDir {
	    fi, err = os.Stat(path.Join(root, pc))
    	if err != nil {
			return nil, "", fmt.Errorf(errStr, p)
    	}
    	isDir = fi.IsDir()
    	if !isDir {
	        //是文件
			return fi, pc, nil
    	}
    }

    //是目录，查找默认索引文件
    for _, v := range index {
    	pi	= path.Join(pc, v)
        fi, err = os.Stat(path.Join(root, pi))
        if err == nil && !fi.IsDir() {
			return fi, pi, nil
        }
    }
	return nil, "", fmt.Errorf(errStr, p)
}

func delay(wait, maxDelay time.Duration) time.Duration {
	if wait == 0 {
		wait = (maxDelay/100)
	}else{
		wait *= 2
	}
	if wait >= maxDelay {
	    wait = maxDelay
	}
	time.Sleep(wait)
    return wait
}

//ExecFunc 执行函数调用
//	call interface{}            函数
//	args ... interface{}        参数或更多个函数是函数的参数
//	[]interface{}				返回直
//	error                       错误
func ExecFunc(f interface{}, args ...interface{}) ([]interface{}, error) {
	return call(f, args...)
}