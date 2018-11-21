package plugin
import(
	"testing"
	"net/http"
    //"os"
	"io/ioutil"
	"time"
	"net"
    "bytes"
    "net/url"
    "crypto/tls"
    "os"
)

func ExamplePluginHTTP() {
    var srpc = NewServerHTTP()
    srpc.Addr="127.0.0.1:13968"
    var p = Plugin{
        Type: PluginTypeHTTP,
        Version: 1.0,
        Name: "名称ABC",
        Addr: srpc.Addr,
    }
    os.Stdout.WriteString(p.String())
    //{"Type":0,"Version":1,"Name":"名称ABC","Addr":"127.0.0.1:13968","Extra":{}}
}


func Test_Server_HTTP_1(t *testing.T) {
	var sendTest = []byte("test")
	//监听
	var sh = NewServerHTTP()
	defer sh.Close()
	//路径绑定一个函数，路径支持正则格式
	sh.Route.HandleFunc("^/(\\d+)$", func(rw http.ResponseWriter, r *http.Request){
		rw.Write(sendTest)
	})
    sh.Addr = "127.0.0.1:0"
	go sh.ListenAndServe()

	time.Sleep(time.Second * 2)

	//请求一个连接
	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://"+sh.L.Addr().String()+"/123")
	if err != nil {
		t.Fatal("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatal("读出数据出错，错误：%v", err)
	}
    if !bytes.Equal(sendTest, b) {
        t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
   }

}

func Test_Server_HTTP_2(t *testing.T) {
	var sendTest = []byte("test")

	//监听
	netListener, err := net.Listen("tcp", "127.0.0.1:5003")
	if err != nil {
		t.Fatal("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()

	var sh = NewServerHTTP()
	//路径绑定一个函数，路径支持正则格式
	sh.Route.HandleFunc("/(\\d+)$", func(rw http.ResponseWriter, r *http.Request){
		rw.Write(sendTest)
	})
	go sh.Serve(netListener)

	time.Sleep(time.Second * 2)

	//请求一个连接
	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://"+sh.L.Addr().String()+"/123")
	if err != nil {
		t.Fatal("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatal("读出数据出错，错误：%v", err)
	}
    if !bytes.Equal(sendTest, b) {
        t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
   }
}

func Test_Server_HTTP_3(t *testing.T) {
	var sendTest = []byte("test")

	//监听
	netListener, err := net.Listen("tcp", "127.0.0.1:5002")
	if err != nil {
		t.Fatal("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()
	var serverHTTP = NewServerHTTP()
	//路径绑定一个函数，路径支持正则格式
	serverHTTP.Route.HandleFunc("/123", func(rw http.ResponseWriter, r *http.Request){
		rw.Write(sendTest)
	})
	go serverHTTP.Serve(netListener)

	time.Sleep(time.Second * 2)

	//请求一个连接
	httpClient := &http.Client{}
	httpResponse, err := httpClient.Get("http://"+serverHTTP.L.Addr().String()+"/123")
	if err != nil {
		t.Fatal("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatal("读出数据出错，错误：%v", err)
	}
    if !bytes.Equal(sendTest, b) {
        t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
   }
}

func Test_Server_HTTP_4(t *testing.T) {
	var sendTest = []byte("test")

	//监听
	netListener, err := net.Listen("tcp", "127.0.0.1:5001")
	if err != nil {
		t.Fatal("端口可能被占用，错误：%v", err)
	}
	defer netListener.Close()
	var serverHTTP = NewServerHTTP()
	//路径绑定一个函数，路径支持正则格式
	serverHTTP.Route.HandleFunc("/123", func(rw http.ResponseWriter, r *http.Request){
		rw.Write(sendTest)
	})
    files := []ServerTLSFile{
        {CertFile: "./test/cer/Cert-test.pem", KeyFile: "./test/cer/Cert-test.key"},
    }
	
	conf :=&tls.Config{}
    err = serverHTTP.LoadTLS(conf, files)
	if err != nil {
		t.Fatal("错误：%v", err)
	}

	go serverHTTP.Serve(netListener)

	time.Sleep(time.Second * 2)
    addr := serverHTTP.L.Addr().String()
	//请求一个连接
	httpClient := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
            },
    }
    req := &http.Request{
        Method: "GET",
        URL: &url.URL{Scheme: "https", Host: addr, Path: "/123"},
    }
    httpResponse, err := httpClient.Do(req)
    	if err != nil {
		t.Fatal("请求连接失败，错误：%v", err)
	}
	body := httpResponse.Body
	b, err := ioutil.ReadAll(body)
	body.Close()
	if err != nil {
		t.Fatal("读出数据出错，错误：%v", err)
	}
    if !bytes.Equal(sendTest, b) {
        t.Fatalf("\r\n本地：%s\r\n远程：%s", sendTest, b)
   }
}


func Test_ServerHTTP_Print(t *testing.T) {
    var p = &Plugin{
        Type: PluginTypeHTTP,
        Version: 1.0,
        Name: "www.birdswo.com",
    }
    //服务器监听
    var shttp = NewServerHTTP()
    shttp.Addr = "127.0.0.1:80"

    var addr = p.Addr

    shttp.AutoFill(p)

    if addr == p.Addr {
        t.Fatalf("\r\n前Addr：%v\r\n后Addr：%v", addr, p.Addr)
    }
}
