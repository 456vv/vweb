package server

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/456vv/vweb/v2"
	"github.com/456vv/vweb/v2/server/config"
	"github.com/issue9/assert/v2"
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

	time.AfterFunc(2*time.Second, func() {
		sg.Close()
	})
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
		{
			fileExt:  ".html",
			allowExt: []string{".bw", ".go"},
			result:   false,
		},
		{
			fileExt:  ".go",
			allowExt: []string{".bw", ".go"},
			result:   true,
		},
		{
			fileExt:  ".bw",
			allowExt: []string{".bw", ".go"},
			result:   true,
		},
	}

	// 服务器
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
		{
			ext:    ".txt",
			MIME:   map[string]string{".txt": "application/text", ".html": "text/html", ".go": "application/go", ".bw": "text/html"},
			result: "application/text",
		}, {
			ext:    ".txt",
			MIME:   map[string]string{".txt": "", ".html": "text/html", ".go": "application/go", ".bw": "text/html"},
			result: "",
		}, {
			ext:    ".bw",
			MIME:   map[string]string{".txt": "", ".html": "text/html", ".go": "application/go", ".bw": "text/html"},
			result: "text/html",
		}, {
			ext:    ".htm",
			MIME:   map[string]string{".txt": "", ".html": "text/html", ".go": "application/go", ".bw": "text/html"},
			result: "text/html; charset=utf-8", // MIME中没有定义，默认向系统中的MIME表读取。
		},
	}
	// 服务器
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
		}, {
			r: &http.Request{URL: &url.URL{Path: "/abc"}},
			conf: &config.SiteDirectory{
				Root:    "/123/456/789",
				Virtual: []string{"/abc"},
			},
			root: "/",
		}, {
			r: &http.Request{URL: &url.URL{Path: "/abc"}},
			conf: &config.SiteDirectory{
				Root:    "/123/456/789",
				Virtual: []string{"aaa/bbbb/abc"},
			},
			root: "aaa/bbbb",
		}, {
			r: &http.Request{URL: &url.URL{Path: "/"}},
			conf: &config.SiteDirectory{
				Root:    "G:/123/456/789",
				Virtual: []string{"G:/abc", "C:/abc"},
			},
			root: "G:/123/456/789",
		}, {
			r: &http.Request{URL: &url.URL{Path: "/A/B/C"}},
			conf: &config.SiteDirectory{
				Root:    "G:/123/456/789",
				Virtual: []string{"G:/abc", "C:/abc", "D:/123/456/A"},
			},
			root: "D:/123/456",
		}, {
			r: &http.Request{URL: &url.URL{Path: "/A/B/C/"}},
			conf: &config.SiteDirectory{
				Root:    "G:/123/456/789",
				Virtual: []string{"G:/abc", "C:/abc", "D:/123/456/A"},
			},
			root: "D:/123/456",
		}, {
			r: &http.Request{URL: &url.URL{Path: "/B/C/"}},
			conf: &config.SiteDirectory{
				Root:    "G:/123/456/789",
				Virtual: []string{":/abc", "C:/abc", "D:/123/---/B"},
			},
			root: "D:/123/---",
		}, {
			r: &http.Request{URL: &url.URL{Path: "/B/C/"}},
			conf: &config.SiteDirectory{
				Root:    "",
				Virtual: []string{},
			},
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
	cstlsf1 := config.ServerTLSFile{
		CertFile: fileCert,
		KeyFile:  fileKey,
	}
	cstlsf2 := config.ServerTLSFile{
		CertFile: fileCert,
		KeyFile:  fileKey,
	}
	CS := &config.Server{
		TLS: &config.ServerTLS{
			RootCAs: []config.ServerTLSFile{cstlsf1, cstlsf2},
		},
	}
	err = srv.ConfigServer(CS)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if d := len(srv.TLSConfig.Certificates); d != 2 {
		t.Fatalf("证书绑定host 失败，预定3个数量，不正确数量：%d", d)
	}

	CS = &config.Server{
		TLS: &config.ServerTLS{
			RootCAs: []config.ServerTLSFile{},
		},
	}
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
	sg.sitePool = vweb.DefaultSitePool
	conf := config.Site{
		Identity: "A",
		Session: config.SiteSession{
			Name:         "BB",
			Expired:      0,
			Size:         128,
			ActivationID: true,
		},
	}
	sg.updateSitePoolAdd(conf)

	site := vweb.DefaultSitePool.NewSite(conf.Identity)

	if conf.Session.Expired != int64(site.Sessions.Expired) {
		t.Fatal("无法增加站点池")
	}

	sg.updateSitePoolDel([]string{})
	if int64(vweb.DefaultSitePool.NewSite(conf.Identity).Sessions.Expired) == conf.Session.Expired {
		t.Fatal("无法删除站点池")
	}
}
