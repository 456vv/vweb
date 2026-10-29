package server

import (
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/456vv/vweb/v3/server/config"
	"github.com/issue9/assert/v4"
)

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

func Test_NewServerGroup_1(t *testing.T) {
	as := assert.New(t, true)
	sg := NewGroup()
	time.AfterFunc(2*time.Second, func() {
		err := sg.Close()
		as.NotError(err)
	})
	_, err := sg.LoadConfigFile("./config/test/config.json")
	as.NotError(err)
	err = sg.Start()
	as.NotError(err)
}

func Test_NewServerGroup_2(t *testing.T) {
	as := assert.New(t, true)
	sg := NewGroup()
	osFile, err := os.Open("./config/test/config.json")
	as.NotError(err)
	defer osFile.Close()

	conf := new(config.Config)
	err = conf.ParseReader(osFile)
	as.NotError(err)

	err = sg.UpdateConfig(conf)
	as.NotError(err)

	time.AfterFunc(2*time.Second, func() { sg.Close() })
	sg.Start()
}

func Test_ServerGroup_LoadConfigFile(t *testing.T) {
	sg := NewGroup()
	defer sg.Close()
	_, err := sg.LoadConfigFile("./config/test/config.json")
	if err == nil && sg.config == nil {
		t.Fatalf("加载配置文件错误")
	}
}

func Test_ServerGroup_httpIsDynamic1(t *testing.T) {
	tests := []struct {
		fileExt  string
		allowExt []string
		result   bool
	}{
		{".html", []string{".bw", ".go"}, false},
		{".go", []string{".bw", ".go"}, true},
		{".bw", []string{".bw", ".go"}, true},
	}
	for _, test := range tests {
		if strSliceContains(test.allowExt, test.fileExt) != test.result {
			t.Fatalf("该文件后缀（%s）是无法从（%s）识别。", test.fileExt, test.allowExt)
		}
	}
}

func Test_ServerGroup_httpTypeByExtension1(t *testing.T) {
	tests := []struct {
		ext    string
		MIME   map[string]string
		result string
	}{
		{".txt", map[string]string{".txt": "application/text", ".html": "text/html", ".go": "application/go", ".bw": "text/html"}, "application/text"},
		{".txt", map[string]string{".txt": "", ".html": "text/html", ".go": "application/go", ".bw": "text/html"}, ""},
		{".bw", map[string]string{".txt": "", ".html": "text/html", ".go": "application/go", ".bw": "text/html"}, "text/html"},
		{".htm", map[string]string{".txt": "", ".html": "text/html", ".go": "application/go", ".bw": "text/html"}, "text/html; charset=utf-8"},
	}
	for _, test := range tests {
		extType := httpTypeByExtension(test.ext, test.MIME)
		if test.result != extType {
			t.Logf("该文件后缀(%s), 扩展类型是（%s）。\r\n", test.ext, extType)
		}
	}
}

func Test_ConfigSiteDirectory_RootDir(t *testing.T) {
	tests := []struct {
		r    *http.Request
		conf *config.SiteDirectory
		root string
	}{
		{
			r: &http.Request{URL: &url.URL{Path: "/A/B/C"}},
			conf: &config.SiteDirectory{
				Root:    "G:/123/456/789",
				Virtual: []string{"D:/123/456/A", "G:/abc", "C:/abc"},
			},
			root: "D:/123/456",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/abc"}},
			conf: &config.SiteDirectory{Root: "/123/456/789", Virtual: []string{"/abc"}},
			root: "/",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/abc"}},
			conf: &config.SiteDirectory{Root: "/123/456/789", Virtual: []string{"aaa/bbbb/abc"}},
			root: "aaa/bbbb",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/"}},
			conf: &config.SiteDirectory{Root: "G:/123/456/789", Virtual: []string{"G:/abc", "C:/abc"}},
			root: "G:/123/456/789",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/A/B/C"}},
			conf: &config.SiteDirectory{Root: "G:/123/456/789", Virtual: []string{"G:/abc", "C:/abc", "D:/123/456/A"}},
			root: "D:/123/456",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/A/B/C/"}},
			conf: &config.SiteDirectory{Root: "G:/123/456/789", Virtual: []string{"G:/abc", "C:/abc", "D:/123/456/A"}},
			root: "D:/123/456",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/B/C/"}},
			conf: &config.SiteDirectory{Root: "G:/123/456/789", Virtual: []string{":/abc", "C:/abc", "D:/123/---/B"}},
			root: "D:/123/---",
		},
		{
			r:    &http.Request{URL: &url.URL{Path: "/B/C/"}},
			conf: &config.SiteDirectory{Root: "", Virtual: []string{}},
			root: "",
		},
	}
	for i, test := range tests {
		root := test.conf.RootDir(test.r.URL.Path)
		if root != filepath.FromSlash(test.root) {
			t.Fatalf("%d,返回根目录和预先设定的不匹配。返回（%s），预先（%s）", i, root, filepath.FromSlash(test.root))
		}
	}
}

func Test_Server_ConfigServer(t *testing.T) {
	tempDir := os.TempDir()
	fileCert := filepath.Join(tempDir, "fileCert.pem")
	filec, err := os.OpenFile(fileCert, os.O_CREATE|os.O_RDWR, 0o777)
	if err != nil {
		t.Fatal(err)
	}
	filec.Write([]byte(testCert))
	filec.Close()
	defer os.RemoveAll(fileCert)

	fileKey := filepath.Join(tempDir, "fileCert.key")
	filec, err = os.OpenFile(fileKey, os.O_CREATE|os.O_RDWR, 0o777)
	if err != nil {
		t.Fatal(err)
	}
	filec.Write([]byte(testKey))
	filec.Close()
	defer os.RemoveAll(fileKey)

	srv := new(Server)
	cstlsf1 := config.ServerTLSFile{CertFile: fileCert, KeyFile: fileKey}
	cstlsf2 := config.ServerTLSFile{CertFile: fileCert, KeyFile: fileKey}
	CS := &config.Server{
		TLS: &config.ServerTLS{RootCAs: []config.ServerTLSFile{cstlsf1, cstlsf2}},
	}
	err = srv.ConfigServer(CS)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if d := len(srv.TLSConfig.Certificates); d != 2 {
		t.Fatalf("证书绑定host 失败，预定3个数量，不正确数量：%d", d)
	}

	CS = &config.Server{TLS: &config.ServerTLS{RootCAs: []config.ServerTLSFile{}}}
	err = srv.ConfigServer(CS)
	if err != nil {
		t.Fatal(err)
	}
	if d := len(srv.TLSConfig.Certificates); d != 0 {
		t.Fatalf("证书绑定host 失败，预定0个数量，不正确数量：%d", d)
	}
}

func Test_Server_updateSitePoolAdd(t *testing.T) {
	sg := NewGroup()
	conf := config.Site{
		Identity: "A",
		Session:  config.SiteSession{Name: "BB", Expired: 10, Size: 128},
	}
	sg.updateSitePoolAdd(conf)
	if time.Duration(conf.Session.Expired)*time.Second != sg.sitePool.NewSite(conf.Identity).Sessions().Expired {
		t.Fatal("站点池的会话时间不一致")
	}

	sg.updateSitePoolDel([]string{}, []string{})
	sg.updateSitePoolAdd(conf)
	if time.Duration(conf.Session.Expired)*time.Second != sg.sitePool.NewSite(conf.Identity).Sessions().Expired {
		t.Fatal("站点池的【默认】会话时间不一致")
	}
}

func TestServer_ConfigConn_Nil(t *testing.T) {
	srv := new(Server)
	if err := srv.ConfigConn(nil); err == nil {
		t.Fatal("nil 应报错")
	}
}

func TestServer_ConfigServer_Nil(t *testing.T) {
	srv := new(Server)
	if err := srv.ConfigServer(nil); err == nil {
		t.Fatal("nil 应报错")
	}
}

func TestServer_ListenAndServe_EmptyAddr(t *testing.T) {
	srv := new(Server)
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("空 Addr 应报错")
	}
}

func TestServer_Serve_NilListener(t *testing.T) {
	srv := new(Server)
	if err := srv.Serve(nil); err == nil {
		t.Fatal("nil listener 应报错")
	}
}

func TestServer_Serve_TypedNilListener(t *testing.T) {
	srv := new(Server)
	var ln *net.TCPListener
	if err := srv.Serve(ln); err == nil {
		t.Fatal("typed-nil listener 应报错")
	}
}

func TestServer_Close_Idempotent(t *testing.T) {
	srv := new(Server)
	_ = srv.Close()
	if err := srv.Close(); err != nil {
		t.Fatalf("重复 Close 不应报错：%v", err)
	}
}

func TestServer_ListenAndServe_AfterClose(t *testing.T) {
	srv := new(Server)
	srv.Addr = "127.0.0.1:0"
	srv.Close()
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Close 后应返回 ErrServerClosed，得到 %v", err)
	}
}

func TestServer_ConfigServer_BadTLS(t *testing.T) {
	srv := new(Server)
	err := srv.ConfigServer(&config.Server{
		TLS: &config.ServerTLS{
			RootCAs: []config.ServerTLSFile{{CertFile: "/nonexistent/c.pem", KeyFile: "/nonexistent/k.key"}},
		},
	})
	if err == nil {
		t.Fatal("坏证书应报错")
	}
	if srv.TLSConfig != nil {
		t.Fatal("失败后不应留下 TLSConfig")
	}
}

func TestGroup_SetServer_NilRemoves(t *testing.T) {
	g := NewGroup()
	defer g.Close()
	srv := new(Server)
	if err := g.SetServer("127.0.0.1:1", srv); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.GetServer("127.0.0.1:1"); !ok {
		t.Fatal("应已设置")
	}
	if err := g.SetServer("127.0.0.1:1", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.GetServer("127.0.0.1:1"); ok {
		t.Fatal("应已删除")
	}
}

func TestGroup_StartTwice(t *testing.T) {
	g := NewGroup()
	done := make(chan struct{})
	go func() { g.Start(); close(done) }()
	time.Sleep(30 * time.Millisecond)
	if err := g.Start(); err == nil {
		t.Fatal("重复 Start 应报错")
	}
	g.Close()
	<-done
}

func TestGroup_CloseTwice(t *testing.T) {
	g := NewGroup()
	done := make(chan struct{})
	go func() { g.Start(); close(done) }()
	time.Sleep(30 * time.Millisecond)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err == nil {
		t.Fatal("重复 Close 应报错")
	}
	<-done
}

func TestGroup_UpdateConfig_Nil(t *testing.T) {
	g := NewGroup()
	defer g.Close()
	if err := g.UpdateConfig(nil); err == nil {
		t.Fatal("nil 配置应报错")
	}
}

func TestServer_millis_Overflow(t *testing.T) {
	if d := millis(math.MaxInt64); d < 0 {
		t.Fatalf("溢出为负：%v", d)
	}
}

func TestServer_seconds_Overflow(t *testing.T) {
	if d := seconds(math.MaxInt64); d < 0 {
		t.Fatalf("溢出为负：%v", d)
	}
}

// TestServer_Boundary 生命周期边界：typed-nil listener、Close 幂等、坏证书不污染状态。
func TestServer_Boundary(t *testing.T) {
	t.Run("typed-nil listener 拒绝", func(t *testing.T) {
		var ln *net.TCPListener
		if err := new(Server).Serve(ln); err == nil {
			t.Fatal("typed-nil 应报错")
		}
	})

	t.Run("Close 幂等", func(t *testing.T) {
		s := new(Server)
		s.Close()
		if err := s.Close(); err != nil {
			t.Fatalf("重复 Close 应 nil，实得 %v", err)
		}
	})

	t.Run("Close 后 ListenAndServe 返回 ErrServerClosed", func(t *testing.T) {
		s := new(Server)
		s.Addr = "127.0.0.1:0"
		s.Close()
		if err := s.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("应 ErrServerClosed，实得 %v", err)
		}
	})

	t.Run("坏证书不污染状态", func(t *testing.T) {
		s := new(Server)
		err := s.ConfigServer(&config.Server{
			TLS: &config.ServerTLS{
				RootCAs: []config.ServerTLSFile{{CertFile: "/x.pem", KeyFile: "/x.key"}},
			},
		})
		if err == nil {
			t.Fatal("坏证书应报错")
		}
		if s.TLSConfig != nil {
			t.Fatal("失败后不应留下 TLSConfig")
		}
	})

	t.Run("毫秒/秒溢出", func(t *testing.T) {
		if d := millis(math.MaxInt64); d < 0 {
			t.Fatalf("毫秒溢出为负：%v", d)
		}
		if d := seconds(math.MaxInt64); d < 0 {
			t.Fatalf("秒溢出为负：%v", d)
		}
	})
}
