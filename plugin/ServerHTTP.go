package plugin
import(
	"github.com/456vv/vweb"
	"net"
	"net/http"
    "crypto/tls"
)

type ServerTLSFile struct {
    CertFile, KeyFile   string                                              // 证书，key 文件地址
}

//ServerHTTP 服务器HTTP
type ServerHTTP struct {
	*http.Server													            // HTTP
	L           net.Listener										        	// 监听器
    Route       *vweb.Route                                     				// 路由表
}

//NewServerHTTP HTTP服务对象
func NewServerHTTP() *ServerHTTP {
	var shttp = &ServerHTTP{
			Server  : new(http.Server),
            Route   : &vweb.Route{},
	    }
        shttp.Server.Handler = http.HandlerFunc(shttp.Route.ServeHTTP)
    return shttp
}


//LoadTLS 加载证书文件
//	config *tls.Config          证书配置
//	files []ServerTLSFile       证书文件
func (shttp *ServerHTTP) LoadTLS(config *tls.Config, files []ServerTLSFile) error {
    shttp.Server.TLSConfig = config
    for _, file := range files {
	    cert, err := tls.LoadX509KeyPair(file.CertFile, file.KeyFile)
        if err != nil {
            shttp.Server.TLSConfig = nil
            return err
        }
        config.Certificates = append(config.Certificates, cert)
    }
    //多证书
    config.BuildNameToCertificate()
    return nil
}


//ListenAndServe 监听并启动
//	error 错误
func (shttp *ServerHTTP) ListenAndServe() error {
	addr := shttp.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
    shttp.L = tcpKeepAliveListener{ln.(*net.TCPListener)}
	return shttp.Serve(shttp.L)
}


//Serve 监听
//	error 错误
func (shttp *ServerHTTP) Serve(l net.Listener) error{
    config := shttp.Server.TLSConfig
    if config != nil {
        if !strSliceContains(config.NextProtos, "http/1.1") {
        	config.NextProtos = append(config.NextProtos, "http/1.1")
        }
        l = tls.NewListener(l, config)
    }
    shttp.L = l
    shttp.Server.Addr = l.Addr().String()
	return shttp.Server.Serve(l)
}


//Close 判断监听的连接
//	error 错误
func (shttp *ServerHTTP) Close() error {
    if shttp.L != nil {
        return shttp.L.Close()
    }
    return nil
}

func strSliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
