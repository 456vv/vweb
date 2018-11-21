package vweb

import (
//	"fmt"
	"testing"
    "encoding/json"
    "bytes"
    "io"
    "io/ioutil"
    "time"
    "net/url"
    "net/http"
   //"fmt"
    //"runtime"
    "os"
    "path/filepath"
    //"crypto/tls"
    //"net"
    "github.com/456vv/vmap/v2"
)

var configJson  = []byte(`
{
    "Servers":{
        "127.0.0.1:443":{
            "Status":true,
            "CC":{
                "Deadline":10000,
                "WriteDeadline":10000,
                "ReadDeadline":10000,
                "KeepAlive":true,
                "KeepAlivePeriod":600000,
                "Linger":0,
                "NoDelay":true,
                "ReadBuffer":4096,
                "WriteBuffer":4096
            },
            "CS":{
                "ReadTimeout":10000,
                "WriteTimeout":10000,
                "MaxHeaderBytes":0,
                "KeepAlivesEnabled":true,
                "TLS":{
                    "File":[{
                        "CertFile":"./test/Cer/Cert-test.pem",
                        "KeyFile":"./test/Cer/Cert-test.key"
                    }],
                    "NextProtos":["http/1.1","h2"],
                    "CipherSuites":[],
                    "PreferServerCipherSuites":true,
                    "SessionTicketsDisabled":false,
                    "SessionTicketKey":[],
                    "MinVersion":771,
                    "MaxVersion":771
                }
            }
        },"127.0.0.1:80":{
            "Status":true,
            "CC":{
                "Deadline":0,
                "WriteDeadline":10000,
                "ReadDeadline":10000,
                "KeepAlive":true,
                "KeepAlivePeriod":60000,
                "Linger":0,
                "NoDelay":true,
                "ReadBuffer":4096,
                "WriteBuffer":4096
            },
            "CS":{
                "ReadTimeout":0,
                "WriteTimeout":0,
                "MaxHeaderBytes":0,
                "KeepAlivesEnabled":true
            }
        }
    },
    "Sites":{
    	"RecoverySessionTick":5000,
	    "Site": [
	        {
	            "Status":true,
	            "Name":"A",
	            "Host":["a.baidu.com", "b.baidu.com", "127.0.0.1"],
	            "Forward":{
	                "127.0.0.1":[{
	                    "Path":["/more/"],
	                    "RePath":"/template/baidu.bw",
	                    "End":true
	                },{
	                    "Path":["/template/b.bw"],
	                    "RePath":"/template/a.bw"
	                }]
	            },
	            "Plugin":{
	                "RPC":{
	                    "bw":{
                    		"LocalAddr":":0",
                    		"Timeout":10000,
                    		"KeepAlive":1800000,
                    		"FallbackDelay":300,
                    		"DualStack":true,
							"IdeConn":100,
							"MaxConn":100,
	                        "Addr":"127.0.0.1:9000",
	                        "Path":"/_goRPC_"
	                    }
	                },
	                "HTTP":{
	                    "bw":{
	                    	"Addr":"127.0.0.1:60000",
	                    	"Host":"www.baidu.com",
                    		"LocalAddr":":0",
                    		"Timeout":5000,
                    		"KeepAlive":60000,
                    		"FallbackDelay":300,
                    		"DualStack":true,
							"TLSHandshakeTimeout":0,
							"DisableKeepAlives":false,
							"DisableCompression":false,
							"MaxIdleConnsPerHost":100,
							"MaxConnsPerHost":100,
							"IdleConnTimeout":60000,
							"ResponseHeaderTimeout":5000,
							"ExpectContinueTimeout":0,
							"ProxyConnectHeader":{
								"A":["a1"]
							},
							"MaxResponseHeaderBytes":20480,
							"TLS":{
								"ServerName":"",
								"InsecureSkipVerify":false,
								"NextProtos":["http/1.1", "h2"],
								"CipherSuites":[157, 49162, 49172, 49187, 49199, 49195, 49200, 49196, 52392, 52393, 22016],
								"ClientSessionCache":0,
								"CurvePreferences":[],
								"File":[]
							}
	                    }
	                }
	            },
	            "Directory":{
	                "Root":"./test/wwwroot",
	                "Virtual":["./test/wwwroot"]
	            },
	            "IndexFile":["index.html"],
	            "DynamicExt":[".bw"],
	            "Header":{
	                "Static":{
	                    "*":{
	                        "Header":{},
	                        "PageExpired":86400
	                    }
	                },
	                "Dynamic":{},
	                "PageExpired":0,
	                "MIME":{
	                    ".txt":"text/html",
	                    ".bw":"text/html"
	                }
	            },
	            "Log":{},
	            "ErrorPage":{},
	            "Property":{
	                "ConnMaxNumber":100,
	                "ConnSpeed":1000,
	                "BuffSize":1000,
	                "Session":{
	                    "Name":"BWID",
	                	"Expired":1200000,
	                    "Size":128,
	                    "Salt":"fjasdfjpoiqrj943j9vn43ny",
	                    "ActivationID":false
	                }
	            }
	        },{
	            "Status":false,
	            "Name":"B",
	            "Host":["a.baidu.com", "b.baidu.com"],
	            "HostForwarding":{},
	            "PathForwarding":{},
	            "Directory":{},
	            "IndexFile":[],
	            "DynamicExt":[],
	            "Header":{
	                "Static":{},
	                "Dynamic":{},
	                "MIME":{}
	            },
	            "Log":{},
	            "ErrorPage":{},
	            "Property":{
	                "ConnMaxNumber":100,
	                "ConnSpeed":1000,
	                "BuffSize":1000,
	                "Session":{
	                    "Name":"BWID",
	                	"Expired":0,
	                    "ActivationID":false
	                }
	            }
	        }
	    ]
    }
}

`)

var testCert = `
-----BEGIN CERTIFICATE-----
MIIDgzCCAuygAwIBAgICEEEwDQYJKoZIhvcNAQEFBQAwQjELMAkGA1UEBhMCQ04x
CzAJBgNVBAgTAkdEMQ4wDAYDVQQKEwU0NTZWdjEWMBQGA1UEAxMNU1NMLjQ1NlZ2
LmNvbTAeFw0xNjA2MjQwMjQ1MDBaFw0xODA2MjQwMjQ1MDBaMEQxCzAJBgNVBAYT
AkNOMQswCQYDVQQIEwJHRDEOMAwGA1UEChMFNDU2VnYxGDAWBgNVBAMTD2xvZ2lu
LjQ1NnZ2LmNvbTCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEA4Lm/CRoipI4Q
ErgiIq/sUgZStQB15gFj33Tm29zMKMGeNUGsIEUMOy902oBRPR59fX3jZSzC1qBq
8PWokkgKbhB2dgzgnOAzQiW01N0X7h3WdOv0YuAQwiojzsQx5vm/+7Bh/MWb/Y7G
Gc7fYH+J7hox3gNKDKc4s5ioYddKheECAwEAAaOCAYQwggGAMA8GA1UdDwEB/wQF
AwMH/4AwJwYDVR0lBCAwHgYIKwYBBQUHAwEGCCsGAQUFBwMCBggrBgEFBQcDBDAM
BgNVHRMBAf8EAjAAMB0GA1UdDgQWBBSFDIISd23v2BgaOhz5RCSzspH5+TAfBgNV
HSMEGDAWgBTV2H3LpFlboBGqiyYsB13mgiRMIDAxBggrBgEFBQcBAQQlMCMwIQYI
KwYBBQUHMAGGFWh0dHA6Ly9vY3NwLjQ1NlZ2LmNvbTBEBgNVHREEPTA7ggsqLjQ1
NnZ2LmNvbYIJMTI3LjAuMC4xgglsb2NhbGhvc3SHBMCoAWSHECABSGAAACABAAAA
AAAAAGgwLAYDVR0eAQH/BCIwIKAeMA2CC0EuNDU2VnYuY29tMA2CC0IuNDU2VnYu
Y29tME8GA1UdHwRIMEYwH6AdoBuGGWh0dHA6Ly80NTZWdi5jb20vY2VydC5jcmww
I6AhoB+GHWh0dHA6Ly9jcmwuNDU2VnYuY29tL2NlcnQuY3JsMA0GCSqGSIb3DQEB
BQUAA4GBAKaorFGUwuyFshVj9tjR8TIYwVWMBN+o5ipwpB+L1kE0IMFE8pDBCZrj
roQdgLT7Y3RbckYOMWHMStzs2EFQUZCBUthpFhfGKmyPrCDzZiuZHFzD1VHzwlVl
AJ7GzUT9TKQDHvXP5tNWCkvPSEbMLCKd0w1HkQofhxMdbOlqs94N
-----END CERTIFICATE-----
`
var testKey = `
-----BEGIN PRIVATE KEY-----
MIICXQIBAAKBgQDgub8JGiKkjhASuCIir+xSBlK1AHXmAWPfdObb3MwowZ41Qawg
RQw7L3TagFE9Hn19feNlLMLWoGrw9aiSSApuEHZ2DOCc4DNCJbTU3RfuHdZ06/Ri
4BDCKiPOxDHm+b/7sGH8xZv9jsYZzt9gf4nuGjHeA0oMpzizmKhh10qF4QIDAQAB
AoGAFGAC+BpMhcrznh7fyXFV5eH44bxW9DGwEnSQ8eJFCHT1mTKJHqvj/gHBgIYd
14LKMfSWB3hVegw1Zf9/9zNc7o5FGNrnaOpYRe+8SO9gU+4lm9ITehzVTzkBxCcX
dkX9iGjC3pARgkXJ+zW6TvEHWrQ2zYehDzkup9BC67TvMzkCQQDvi3WHk/YYXzu7
MdvVqlBSrq45XrqspVi+r4TEsKPUrt9Y5YmvYKn8G8iM4gKnaEBJjJdeKmt31yYA
9FRYjGILAkEA8Cmuv7Vv/UlJBbACQ26CBw+QmUgvd/JYhHxsxbQ2wqxdeITWOUxf
aG7R1JRNEgXFya/4u2pMjzQDr+JpWsW3QwJBAOmWQYZytyCvBQ0WonspOGhYJFaX
VEt0dSSE/V/bq/aCjBMgyfF1vmy0Hw2aeuIKG95ctWJC1UcoSsvVdcZfJl8CQQDm
c6j6zri6vKL0cTOKzzS4X8gqPelG2Ob1oouhns9ZOJqsthL2goGerZBtwyy9WYq0
gUZVWKhEVe4fzUu5TbYPAkAkwJWVpG3zZOflwKxqnCfC4mcL9qv2oyWqBT3S5oxE
LzeIJd6AClByowsdS5v/DeZQnfDaW68OB3+vqKQbMbei
-----END PRIVATE KEY-----
`

func Test_NewServerGroup_ServeHTTP1(t *testing.T){
	sg := NewServerGroup()
	go func(){
		time.AfterFunc(time.Second*2, func(){
	        sg.Close()
	        DefaultSitePool.Pool.Reset()
	        DefaultSites.Host.Reset()
	    })
	    file := "./test/config.json"
	    _, _, err := sg.LoadConfigFile(file)
	    if(err != nil){
	        t.Fatalf("挂载文件失败：%s", err)
	    }
	}()
    err := sg.Start()
    if(err != nil){
        t.Fatalf("启动失败：%s", err)
    }

}
func Test_NewServerGroup2(t *testing.T){
    sg := NewServerGroup()

	time.AfterFunc(time.Second, func(){
        sg.Close()
        DefaultSitePool.Pool.Reset()
        DefaultSites.Host.Reset()
    })

    file := "./test/config.json"
    _, _, err := sg.LoadConfigFile(file)
    if(err == nil){
        t.Fatalf("失败：%s", file)
    }
    sg.Start()
}

func Test_NewServerGroup1(t *testing.T){

    sg := NewServerGroup()
    osFile, err := os.Open("./test/config.json")
    if err != nil {
    	t.Fatal(err)
    }
    b, err := ioutil.ReadAll(osFile)
    if err != nil {
    	t.Fatal(err)
    }
    buf := bytes.NewBuffer(b)
    conf    := &Config{}
    err = ConfigDataParse(conf, buf)
    if(err != nil){
        t.Fatal(err)
    }
    err = sg.UpdateConfig(conf)
    if err == nil && sg.config == nil {
        t.Fatalf("失败，配置文件无法保存到sg.config")
    }


	time.AfterFunc(time.Second, func(){
        sg.Close()
        DefaultSitePool.Pool.Reset()
        DefaultSites.Host.Reset()
    })
    sg.Start()

}

func Test_ServerGroup_LoadConfigFile(t *testing.T){
	sg := NewServerGroup()
    defer sg.Close()
    file := "./test/config.json"
    conf, _, err := sg.LoadConfigFile(file)
    if err == nil && sg.config == nil {
        t.Fatalf("加载配置文件错误：%s", file)
    }
    if conf == nil {
    	t.Fatalf("错误的conf不应该为nil")
    }
}




func Test_ServerGroup_httpIsDynamic1(t *testing.T){
    var tests = []struct{
        fileExt string
        allowExt []string
        result  bool
    }{
        {
        fileExt:".html",
        allowExt:[]string{".bw", ".go"},
        result:false,
        },
        {
        fileExt:".go",
        allowExt:[]string{".bw", ".go"},
        result:true,
        },
        {
        fileExt:".bw",
        allowExt:[]string{".bw", ".go"},
        result:true,
        },
    }

    //服务器
    for _, test := range tests {
        if strSliceContains(test.allowExt, test.fileExt) != test.result{
            t.Fatalf("该文件后缀（%s）是无法从（%s）识别。", test.fileExt, test.allowExt)
        }
    }
}

func Test_ServerGroup_httpTypeByExtension1(t *testing.T){
    var tests = []struct{
        ext         string
        MIME        map[string]string
        result      string
    }{
        {
            ext:    ".txt",
            MIME:   map[string]string{".txt":"application/text",".html":"text/html", ".go":"application/go", ".bw":"text/html"},
            result: "application/text",
        },{
            ext:    ".txt",
            MIME:   map[string]string{".txt":"",".html":"text/html", ".go":"application/go", ".bw":"text/html"},
            result: "",
        },{
            ext:    ".bw",
            MIME:   map[string]string{".txt":"",".html":"text/html", ".go":"application/go", ".bw":"text/html"},
            result: "text/html",
        },{
            ext:    ".htm",
            MIME:   map[string]string{".txt":"",".html":"text/html", ".go":"application/go", ".bw":"text/html"},
            result: "text/html; charset=utf-8",//MIME中没有定义，默认向系统中的MIME表读取。
        },

    }
    //服务器
    sg := NewServerGroup()

    for _, test := range tests {
        extType := sg.httpTypeByExtension(test.ext, test.MIME)
        if test.result != extType{
            t.Logf("该文件后缀(%s), 扩展类型是（%s）。\r\n", test.ext, extType)
        }
    }
}


func Test_ServerGroup_httpRootPath(t *testing.T){
    tests := []struct {
        r       *http.Request
    	conf    *ConfigSiteDirectory
        root    string
    }{
        {
            r:&http.Request{URL:&url.URL{Path:"/"}},
            conf:&ConfigSiteDirectory{
            	Root:"G:\\123\\456\\789",
                Virtual:[]string{"G:/abc", "C:/abc"},
            },
            root:"G:\\123\\456\\789",
        },{
            r:&http.Request{URL:&url.URL{Path:"/abc"}},
            conf:&ConfigSiteDirectory{
            	Root:"G:\\123\\456\\789",
                Virtual:[]string{"G:/abc", "C:/abc"},
            },
            root:"G:",
        },{
            r:&http.Request{URL:&url.URL{Path:"/A/B/C"}},
            conf:&ConfigSiteDirectory{
            	Root:"G:\\123\\456\\789",
                Virtual:[]string{"G:/abc", "C:/abc", "D:/123/456/A"},
            },
            root:"D:\\123\\456",
        },{
            r:&http.Request{URL:&url.URL{Path:"/A/B/C"}},
            conf:&ConfigSiteDirectory{
            	Root:"G:\\123\\456\\789",
                Virtual:[]string{"G:/abc", "C:/abc", "D:/123/456/A/"},
            },
            root:"D:\\123\\456",
        },{
            r:&http.Request{URL:&url.URL{Path:"/A/B/C/"}},
            conf:&ConfigSiteDirectory{
            	Root:"G:\\123\\456\\789",
                Virtual:[]string{"G:/abc", "C:/abc", "D:/123\\456\\A/"},
            },
            root:"D:\\123\\456",
        },{
            r:&http.Request{URL:&url.URL{Path:"/B/C/"}},
            conf:&ConfigSiteDirectory{
            	Root:"G:\\123\\456\\789",
                Virtual:[]string{":/abc", "C:/abc", "D:123\\---\\B/"},
            },
            root:"D:\\123\\---",
        },{
            r:&http.Request{URL:&url.URL{Path:"/B/C/"}},
            conf:&ConfigSiteDirectory{
            	Root:"",
                Virtual:[]string{},
            },
            root:".",
        },
    }

    sg := NewServerGroup()
    for _, test := range tests {
        root := sg.httpRootPath(test.conf, test.r)
        if root != test.root {
        	t.Fatalf("返回根目录和预先设定的不匹配。返回（%s），预先（%s）", root, test.root)
        }
    }

}

func Test_Server_ConfigServer(t *testing.T){
    tempDir := os.TempDir()
    fileCert := filepath.Join(tempDir, "fileCert.pem")
    os.Remove(fileCert)
    filec, err := os.OpenFile(fileCert, os.O_CREATE|os.O_RDWR, 0777)
    if err != nil {
    	t.Fatal(err)
    }
    filec.Write([]byte(testCert))
    filec.Close()

    fileKey := filepath.Join(tempDir, "fileCert.key")
    os.Remove(fileKey)
    filec, err = os.OpenFile(fileKey, os.O_CREATE|os.O_RDWR, 0777)
    if err != nil {
    	t.Fatal(err)
    }
    filec.Write([]byte(testKey))
    filec.Close()

	var srv = new(Server)
    cstlsf1 := ConfigServerTLSFile{
        CertFile    : fileCert,
        KeyFile     : fileKey,
    }
    cstlsf2 := ConfigServerTLSFile{
        CertFile    : fileCert,
        KeyFile     : fileKey,
    }
	CS := &ConfigServer{
        TLS:&ConfigServerTLS{
            File:[]ConfigServerTLSFile{cstlsf1,cstlsf2},
         },
    }
    CC := &ConfigConn{
    	
    }
    srv.ConfigListener(":0", CC)
    if err != nil {
    	t.Fatal(err)
    }
    err = srv.ConfigServer(CS)
    if err != nil {
    	t.Fatal(err)
    }
	defer srv.Close()
    if srv.TLSConfig == nil || len(srv.TLSConfig.NameToCertificate) != 4 {
    //	t.Log(srv.TLSConfig.NameToCertificate)
    	t.Fatalf("证书绑定host 失败，预定4个数量，不正确数量：%d",  len(srv.TLSConfig.NameToCertificate))
    }
    
	CS = &ConfigServer{
        TLS:&ConfigServerTLS{
            File:[]ConfigServerTLSFile{},
         },
	}
    err = srv.ConfigServer(CS)
    if err != nil {
    	t.Fatal(err)
    }
    if srv.TLSConfig == nil || len(srv.TLSConfig.NameToCertificate) != 0 {
    	t.Fatalf("证书绑定host 失败，预定0个数量，不正确数量：%d",  len(srv.TLSConfig.NameToCertificate))
    }

    
}


func Test_Server_updateSitePoolAdd(t *testing.T){
	var sg = NewServerGroup()
    conf := &ConfigSite{
        Name:"A",
        Property:ConfigSiteProperty{
            Session:ConfigSitePropertySession{
                Name         : "BB",
                Expired      : 0,
                Size         : 128,
                ActivationID : true,
            },
        },
    }
    sg.updateSitePoolAdd(conf)
    if !DefaultSitePool.Pool.Has("A") {
    	t.Fatal("无法增加站点池")
    }
    sg.updateSitePoolDel([]string{})
    if DefaultSitePool.Pool.Has("A") {
    	t.Fatal("无法删除站点池")
    }
    DefaultSitePool.Pool.Reset()

}

func Test_Server_updateSitePoolDel(t *testing.T){
    site := NewSite()
    DefaultSitePool.Pool.Set("A", site)
    DefaultSitePool.Pool.Set("B", site)
    DefaultSitePool.Pool.Set("C", site)

	var sg = NewServerGroup()
	sg.SetSitePool(DefaultSitePool)
    newName := []string{"A", "B", "C", "D"}
    sg.updateSitePoolDel(newName)

    newName = []string{"A", "B", "D"}
    sg.updateSitePoolDel(newName)

    if sg.sitePool.Pool.Has("C") {
    	t.Fatal("无法删除C键值")
    }
    DefaultSitePool.Pool.Reset()
}

func Test_ServerGroup_updateSiteConfig(t *testing.T){

    site := NewSite()
    DefaultSitePool.Pool.Set("A", site)

	var sg = NewServerGroup()
	sg.SetSitePool(DefaultSitePool)
	sg.SetSites(DefaultSites)

    DefaultSites.Host.Set("google.com", site)
    DefaultSites.Host.Set("baidu.com", site)

    mm := vmap.NewMap()
    mm.Set("baidu.com", "A")
    mm.Set("google.com", "A")

    sg.updateSiteConfig(mm)
    mm.Del("google.com")

    sg.updateSiteConfig(mm)

    _, ok := sg.sites.Host.GetHas("google.com")
    if ok {
    	t.Log("删除失败，内部还存在google.com键值")
    }

    _, ok = sg.sites.Host.GetHas("baidu.com")
    if !ok {
    	t.Log("错误删除，内部没有存在baidu.com键值")
    }
    sg.sitePool.Pool.Reset()
}


func Test_ServerGroup_UpdateConfig(t *testing.T){
    conf := &Config{}

    bytesReader := bytes.NewReader(configJson)
    jsonDecoder := json.NewDecoder((io.Reader)(bytesReader))
    err := jsonDecoder.Decode(conf)
    if err != nil {
    	t.Fatal(err)
    }
    var sg  = NewServerGroup()
    err = sg.UpdateConfig(conf)
    if err == nil || sg.config == nil {
    	t.Fatal("错误")
    }
}

func Test_ServerGroup_updateConfigServers(t *testing.T){
    cs := ConfigServer{
        ReadTimeout: int64(time.Millisecond * 500),
        WriteTimeout: int64(time.Millisecond * 500),
        ShutdownConn:false,
    }
    tests := []struct{
        css map[string]ConfigServers
        length  int
    }{
        {
            css: map[string]ConfigServers{
                "127.0.0.1:60001":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60002":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60003":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60004":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60005":ConfigServers{Status:true, CS: cs},
            },
            length:5,
        },{
            css: map[string]ConfigServers{
                "127.0.0.1:60003":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60004":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60005":ConfigServers{Status:true, CS: cs},
            },
            length:3,
        },{
            css: map[string]ConfigServers{
                "127.0.0.1:60003":ConfigServers{Status:false, CS: cs},
                "127.0.0.1:60004":ConfigServers{Status:true, CS: cs},
                "127.0.0.1:60005":ConfigServers{Status:true, CS: cs},
            },
            length:2,
        },
    }

    var sg  = NewServerGroup()
	for _, test := range tests {
	    //启动
	    sg.updateConfigServers(test.css)
	    //由于 .decideListen 方法中有使用 goroutine，需要等待启动
	    time.Sleep(time.Second)


	    if sg.srvMan.Len() != test.length {
	        t.Fatalf("启动监听数目不正确，预先数目为：%d，成功启动数目为：%d", test.length, sg.srvMan.Len())
	    }
	}
   	go sg.Close()
	sg.Start()
}


func Test_ServerGroup_listenStart2listenStop(t *testing.T){
    tests := []string{"127.0.0.1:60001","127.0.0.1:60002","127.0.0.1:60003","127.0.0.1:60004","127.0.0.1:60005"}
    var sg  = NewServerGroup()
    for _, k := range tests {
        cs := ConfigServer{
            ReadTimeout: int64(time.Millisecond * 500),
            WriteTimeout: int64(time.Millisecond * 500),
            ShutdownConn: false,
        }
    	err := sg.listenStart(k, ConfigServers{Status:true, CS: cs})
        if err == nil {
        	time.Sleep(time.Second)
        	if !sg.srvMan.Has(k) {
            	t.Fatalf("监听启动成功，但无法增加记录：%s", k)
        	}
            time.Sleep(time.Second*3)
            err = sg.listenStop(k)
            if err == nil{
                time.Sleep(time.Second*3)
        	    if sg.srvMan.Has(k) {
            	    t.Fatalf("监听停止成功，但无法删除记录：%s", k)
                }
            }else{
                t.Logf("监听（%s）停止失败，原因：%v", k, err)
            }
        }else{
            t.Logf("监听（%s）启动失败，原因：%v", k, err)
        }
    }
}

func Test_ServerGroup_updatePluginConn(t *testing.T){
	config := &ConfigSite{
				Plugin:map[string]ConfigSitePlugins{
					"HTTP":map[string]ConfigSitePlugin{
						"www.xxx.com:80":{
							Addr:"www.xxx.cn:80",
	                    	Host:"www.xxx.cn",
	                    	Scheme:"http",
                    		LocalAddr:":0",
                    		Timeout:5000,
                    		KeepAlive:60000,
                    		FallbackDelay:300,
                    		DualStack:true,
							TLSHandshakeTimeout:0,
							DisableKeepAlives:false,
							DisableCompression:false,
							MaxIdleConnsPerHost:100,
							IdleConnTimeout:60000,
							ResponseHeaderTimeout:5000,
							ExpectContinueTimeout:0,
							ProxyConnectHeader:http.Header{},
							MaxResponseHeaderBytes:20480,
						},
					},
				},
			}
	
	sg := NewServerGroup()
	site := NewSite()
	sg.updatePluginConn(config, site)
	
	_, ok := site.Plugin.IndexHas("HTTP", "www.xxx.com:80")
	if !ok {
		t.Fatal("无法创建HTTP插件！")
	}
}
