package vweb

import(
	"strings"
	"time"
	"fmt"
    "crypto/rand"
    mathRand "math/rand"
    "encoding/hex"
	"os"
	"path"
)

//ExtendDotFuncMap 扩展点函数映射，在模板上的点（.）可以调用
//	deputy map[string]map[string]interface{}  点函数集
func ExtendDotFuncMap(deputy map[string]map[string]interface{}) {
    for k, v := range deputy {
        _, ok := DotFuncMap[k]
        if !ok {
            DotFuncMap[k] = make(map[string]interface{})
        }
        for j, d := range v {
            DotFuncMap[k][j] = d
        }
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
    	return fmt.Errorf("vweb.GenerateRandomId: 参数为 nil, 无法生成随机数据！")
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
	return hex.EncodeToString(b)[:length], err
}

//AddSalt 加盐
//	rnd []byte	标识字节串
//	salt string	盐
//	string  	标识符
func AddSalt(rnd []byte, salt string) string {
    var (
    	id 		string
        start 	int
    )
    length	:= len(salt)
    l		:= len(rnd)
    if length != 0 {
	    for i:=0; i<l; i++ {
	    	rnd[i] = rnd[i] ^ salt[start]
	       	start++
	        if start == length {
	        	start = 0
	        }
	    }
    }
    id = fmt.Sprintf("%x", rnd)
    return id[:l]

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
        fi  os.FileInfo
        err error
        //为了防止跨目录，这里清除 /../ok/index.html 路径为 \ok\index.html，保证安全。
        pc	= path.Clean(p)
        pi	string	//path+fileName
    )

    if strings.HasSuffix(p, "/") {
        //是目录，查找默认索引文件
        for _, v := range index {
        	pi	= path.Join(pc, v)
            fi, err = os.Stat(path.Join(root, pi))
            if err == nil && !fi.IsDir() {
				return fi, pi, nil
            }
        }
	}else{
        //是文件
	    fi, err = os.Stat(path.Join(root, pc))
		if err == nil && !fi.IsDir() {
			return fi, pc, nil
		}
	}
	return nil, "", fmt.Errorf("Access (%s) page document does not exist!", p)
}

