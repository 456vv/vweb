package config

import (
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/456vv/vweb/v3"
)

// -----------------------------------------------------------------------------
// configExclude：零值过滤
// -----------------------------------------------------------------------------

// TestConfigExclude_ZeroValues 表驱动验证 configExclude 的零值判断语义。
func TestConfigExclude_ZeroValues(t *testing.T) {
	f := configExclude(nil)

	tests := []struct {
		name string
		src  any
		want bool // true 表示跳过
	}{
		{"nil pointer", (*int)(nil), true},
		{"non-nil pointer", new(int), false},
		{"nil map", (map[string]int)(nil), true},
		{"empty non-nil map", map[string]int{}, true},
		{"nil slice", ([]string)(nil), true},
		{"empty non-nil slice", []string{}, true},
		{"zero int", int(0), true},
		{"non-zero int", int(1), false},
		{"zero string", "", true},
		{"non-zero string", "x", false},
		{"false bool", false, true},
		{"true bool", true, false},
		{"zero struct", struct{ A int }{}, true},
		{"non-zero struct", struct{ A int }{1}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := f("f", reflect.Value{}, reflect.ValueOf(tc.src))
			if got != tc.want {
				t.Fatalf("src=%v want skip=%v got=%v", tc.src, tc.want, got)
			}
		})
	}

	// handle 优先于零值判断
	always := configExclude(func(name string, dsc, src reflect.Value) bool { return true })
	if !always("f", reflect.Value{}, reflect.ValueOf(1)) {
		t.Fatal("handle 返回 true 应跳过")
	}
}

// -----------------------------------------------------------------------------
// SitePublic.ConfigSiteSession
// -----------------------------------------------------------------------------

func TestSitePublic_ConfigSiteSession_Overrides(t *testing.T) {
	pub := &SitePublic{Session: map[string]SiteSession{
		"pub": {Name: "p", Expired: 10, Size: 20},
	}}

	tests := []struct {
		name   string
		origin SiteSession
		want   SiteSession
		wantOK bool
	}{
		{
			name:   "PublicName 为空则返回 false",
			origin: SiteSession{Name: "x"},
			want:   SiteSession{Name: "x"},
			wantOK: false,
		},
		{
			name:   "公共模板不存在返回 false",
			origin: SiteSession{PublicName: "nope", Name: "x"},
			want:   SiteSession{PublicName: "nope", Name: "x"},
			wantOK: false,
		},
		{
			name:   "origin 零值字段不覆盖公共模板",
			origin: SiteSession{PublicName: "pub", Expired: 0},
			want:   SiteSession{PublicName: "pub", Name: "p", Expired: 10, Size: 20},
			wantOK: true,
		},
		{
			name:   "origin 非零字段覆盖公共模板",
			origin: SiteSession{PublicName: "pub", Name: "o"},
			want:   SiteSession{PublicName: "pub", Name: "o", Expired: 10, Size: 20},
			wantOK: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.origin
			ok := pub.ConfigSiteSession(&got, nil)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Fatalf("got=%+v\nwant=%+v", got, tc.want)
			}
		})
	}

	t.Run("nil receiver", func(t *testing.T) {
		var p *SitePublic
		origin := SiteSession{PublicName: "pub"}
		if p.ConfigSiteSession(&origin, nil) {
			t.Fatal("nil receiver 应返回 false")
		}
	})
	t.Run("nil origin", func(t *testing.T) {
		if pub.ConfigSiteSession(nil, nil) {
			t.Fatal("nil origin 应返回 false")
		}
	})
}

// -----------------------------------------------------------------------------
// SitePublic.ConfigSiteHeader
// -----------------------------------------------------------------------------

// TestSitePublic_ConfigSiteHeader_NoPollute 期望合并过程不污染公共模板。
func TestSitePublic_ConfigSiteHeader_NoPollute(t *testing.T) {
	pubStatic := map[string]SiteHeaderType{
		".html": {Header: map[string][]string{"X-A": {"1"}}},
	}
	pub := &SitePublic{Header: map[string]SiteHeader{
		"pub": {
			Static: pubStatic,
			MIME:   map[string]string{".html": "text/html"},
		},
	}}

	origin := SiteHeader{
		PublicName: "pub",
		Static: map[string]SiteHeaderType{
			".css": {Header: map[string][]string{"X-B": {"2"}}},
		},
	}

	if !pub.ConfigSiteHeader(&origin, nil) {
		t.Fatal("合并失败")
	}

	// 关键：公共模板的 Static 不应包含 origin 的 ".css"
	if _, dirty := pubStatic[".css"]; dirty {
		t.Fatalf("公共模板 Static 被 origin 污染: %+v", pubStatic)
	}

	// origin 同时具有公共和自身条目
	if _, ok := origin.Static[".html"]; !ok {
		t.Fatalf("origin 缺少公共条目: %+v", origin.Static)
	}
	if _, ok := origin.Static[".css"]; !ok {
		t.Fatalf("origin 缺少自身条目: %+v", origin.Static)
	}

	// 修改 origin 的 map 不应回写公共模板
	h := origin.Static[".html"].Header
	h["X-A"] = []string{"changed"}

	if v := pubStatic[".html"].Header["X-A"]; len(v) != 1 || v[0] != "1" {
		t.Fatalf("公共模板 Header 被修改: %v", pubStatic[".html"].Header)
	}
}

// -----------------------------------------------------------------------------
// SitePublic.ConfigSiteForward
// -----------------------------------------------------------------------------

func TestSitePublic_ConfigSiteForward_AppendAndDeepCopy(t *testing.T) {
	pubPath := []string{"/a", "/b"}
	pub := &SitePublic{Forward: map[string]SiteForwards{
		"pub": {List: []SiteForward{{Path: pubPath, RePath: "/x"}}},
	}}
	// 模拟公共模板已 Compile，forwardWriter 非 nil
	fs := pub.Forward["pub"]
	fs.List[0].forwardWriter = &vweb.ForwardRewriter{}
	pub.Forward["pub"] = fs

	origin := SiteForwards{
		PublicName: "pub",
		List:       []SiteForward{{Path: []string{"/own"}, RePath: "/y"}},
	}
	if !pub.ConfigSiteForward(&origin, nil) {
		t.Fatal("合并失败")
	}

	if len(origin.List) != 2 {
		t.Fatalf("List 长度 = %d, want 2", len(origin.List))
	}
	if origin.List[0].RePath != "/y" || origin.List[1].RePath != "/x" {
		t.Fatalf("追加顺序错误: %+v", origin.List)
	}
	// 复制出的 rewriter 必须被清空
	if origin.List[0].forwardWriter != nil {
		t.Fatal("forwardWriter 未在复制时清空")
	}
	// 修改 origin 的 Path 不应影响公共模板 Path
	origin.List[0].Path[0] = "/changed"
	if pubPath[0] != "/a" {
		t.Fatalf("公共 Path 被修改: %v", pubPath)
	}

	// 空列表时仍返回 ok（表示引用成功）
	pubEmpty := &SitePublic{Forward: map[string]SiteForwards{"pub": {List: nil}}}
	origin2 := SiteForwards{PublicName: "pub"}
	if !pubEmpty.ConfigSiteForward(&origin2, nil) {
		t.Fatal("引用存在但为空时应返回 true")
	}
}

// -----------------------------------------------------------------------------
// SitePublic.ConfigSiteDynamic
// -----------------------------------------------------------------------------

func TestSitePublic_ConfigSiteDynamic_ExtCopy(t *testing.T) {
	pubExt := []string{".go", ".tmpl"}
	pub := &SitePublic{Dynamic: map[string]SiteDynamic{
		"pub": {Ext: pubExt, Cache: true},
	}}
	origin := SiteDynamic{PublicName: "pub"}
	if !pub.ConfigSiteDynamic(&origin, nil) {
		t.Fatal("合并失败")
	}
	if !origin.Cache || len(origin.Ext) != 2 {
		t.Fatalf("合并结果错误: %+v", origin)
	}
	// 修改 origin.Ext 不应影响公共 Ext
	origin.Ext[0] = "changed"
	if pubExt[0] != ".go" {
		t.Fatalf("公共 Ext 被修改: %v", pubExt)
	}
}

// -----------------------------------------------------------------------------
// SitePublic.ConfigSitePlugin
// -----------------------------------------------------------------------------

func TestSitePublic_ConfigSitePlugin_CaseInsensitive(t *testing.T) {
	pub := &SitePublic{Plugin: SitePlugins{
		RPC: map[string]SitePlugin{
			"p": {Addr: "1.2.3.4:5", Path: "/rpc"},
		},
		HTTP: map[string]SitePlugin{
			"p": {Addr: "6.7.8.9:10", Scheme: "http"},
		},
	}}

	t.Run("rpc 小写", func(t *testing.T) {
		origin := SitePlugin{PublicName: "p"}
		if !pub.ConfigSitePlugin(&origin, "rpc", nil) {
			t.Fatal("应成功")
		}
		if origin.Addr != "1.2.3.4:5" || origin.Path != "/rpc" {
			t.Fatalf("RPC 合并结果错误: %+v", origin)
		}
	})
	t.Run("HTTP 大写", func(t *testing.T) {
		origin := SitePlugin{PublicName: "p"}
		if !pub.ConfigSitePlugin(&origin, "HTTP", nil) {
			t.Fatal("应成功")
		}
		if origin.Addr != "6.7.8.9:10" || origin.Scheme != "http" {
			t.Fatalf("HTTP 合并结果错误: %+v", origin)
		}
	})
	t.Run("未知 kind", func(t *testing.T) {
		origin := SitePlugin{PublicName: "p"}
		if pub.ConfigSitePlugin(&origin, "ws", nil) {
			t.Fatal("未知 kind 应返回 false")
		}
	})
	t.Run("未找到", func(t *testing.T) {
		origin := SitePlugin{PublicName: "missing"}
		if pub.ConfigSitePlugin(&origin, "rpc", nil) {
			t.Fatal("未找到应返回 false")
		}
	})
}

// -----------------------------------------------------------------------------
// SitePlugins.ConfigSitePluginHTTP（深拷贝隔离）
// -----------------------------------------------------------------------------

func TestSitePlugins_ConfigSitePluginHTTP_DeepCopy(t *testing.T) {
	pubNextProtos := []string{"h2"}
	pubHeader := http.Header{"X-Test": {"1"}}
	pub := &SitePlugins{HTTP: map[string]SitePlugin{
		"pub": {
			Addr:               "1.2.3.4:80",
			TLS:                &SitePluginTLS{NextProtos: pubNextProtos},
			ProxyConnectHeader: pubHeader,
		},
	}}

	origin := SitePlugin{PublicName: "pub"}
	if !pub.ConfigSitePluginHTTP(&origin, nil) {
		t.Fatal("合并失败")
	}
	if origin.Addr != "1.2.3.4:80" {
		t.Fatalf("Addr = %q", origin.Addr)
	}
	if origin.TLS == nil || origin.TLS == pub.HTTP["pub"].TLS {
		t.Fatal("TLS 指针未隔离")
	}
	origin.TLS.NextProtos[0] = "changed"
	if pubNextProtos[0] != "h2" {
		t.Fatalf("公共 NextProtos 被修改: %v", pubNextProtos)
	}
	if len(origin.ProxyConnectHeader["X-Test"]) == 0 || len(pubHeader["X-Test"]) == 0 {
		t.Fatal("Header 缺失")
	}
	if &origin.ProxyConnectHeader["X-Test"][0] == &pubHeader["X-Test"][0] {
		t.Fatal("ProxyConnectHeader 底层数组共享")
	}
}

// -----------------------------------------------------------------------------
// SitePlugins.ConfigSitePluginRPC（深拷贝隔离与合并语义）
// -----------------------------------------------------------------------------

func TestSitePlugins_ConfigSitePluginRPC(t *testing.T) {
	newPub := func() *SitePlugins {
		return &SitePlugins{RPC: map[string]SitePlugin{
			"pub": {
				Addr: "1.2.3.4:5", Path: "/rpc", MaxConn: 10, Timeout: 1000,
				TLS:                &SitePluginTLS{ServerName: "svc", NextProtos: []string{"h2"}},
				ProxyConnectHeader: http.Header{"X-A": {"1"}},
			},
		}}
	}

	// 1) 合并语义：origin 全零 / 非零覆盖 / handle 排除 / RPC 与 HTTP 模板分离
	t.Run("merge semantics", func(t *testing.T) {
		tests := []struct {
			name   string
			pub    *SitePlugins
			origin SitePlugin
			handle func(string, reflect.Value, reflect.Value) bool
			want   SitePlugin // 仅比较 PublicName/Addr/Path/MaxConn/Timeout
		}{
			{
				name:   "origin 全零继承公共",
				pub:    newPub(),
				origin: SitePlugin{PublicName: "pub"},
				want:   SitePlugin{PublicName: "pub", Addr: "1.2.3.4:5", Path: "/rpc", MaxConn: 10, Timeout: 1000},
			},
			{
				name:   "origin 非零覆盖公共",
				pub:    newPub(),
				origin: SitePlugin{PublicName: "pub", Addr: "9.9.9.9:1", MaxConn: 20},
				want:   SitePlugin{PublicName: "pub", Addr: "9.9.9.9:1", Path: "/rpc", MaxConn: 20, Timeout: 1000},
			},
			{
				name:   "handle 排除字段保留公共值",
				pub:    newPub(),
				origin: SitePlugin{PublicName: "pub", MaxConn: 20},
				handle: func(name string, _, _ reflect.Value) bool { return name == "MaxConn" },
				want:   SitePlugin{PublicName: "pub", Addr: "1.2.3.4:5", Path: "/rpc", MaxConn: 10, Timeout: 1000},
			},
			{
				name: "RPC 使用自身模板而非 HTTP",
				pub: &SitePlugins{
					RPC:  map[string]SitePlugin{"pub": {Addr: "rpc:1"}},
					HTTP: map[string]SitePlugin{"pub": {Addr: "http:1"}},
				},
				origin: SitePlugin{PublicName: "pub"},
				want:   SitePlugin{PublicName: "pub", Addr: "rpc:1"},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				origin := tc.origin
				if !tc.pub.ConfigSitePluginRPC(&origin, tc.handle) {
					t.Fatal("应成功合并")
				}
				got := SitePlugin{
					PublicName: origin.PublicName,
					Addr:       origin.Addr,
					Path:       origin.Path,
					MaxConn:    origin.MaxConn,
					Timeout:    origin.Timeout,
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("got=%+v\nwant=%+v", got, tc.want)
				}
			})
		}
	})

	// 2) 深拷贝隔离：TLS / ProxyConnectHeader 不共享；origin 自带 TLS 覆盖且深拷贝
	t.Run("deep copy isolation", func(t *testing.T) {
		pub := newPub()
		pubTLS := pub.RPC["pub"].TLS
		pubH := pub.RPC["pub"].ProxyConnectHeader
		origin := SitePlugin{PublicName: "pub"}
		if !pub.ConfigSitePluginRPC(&origin, nil) {
			t.Fatal("合并失败")
		}
		if origin.TLS == nil {
			t.Fatal("TLS 未继承公共模板")
		}
		if origin.TLS == pubTLS {
			t.Fatal("TLS 指针未隔离（仍与公共共享）")
		}
		origin.TLS.NextProtos[0] = "changed"
		if pubTLS.NextProtos[0] != "h2" {
			t.Fatalf("公共 NextProtos 被 origin 修改: %v", pubTLS.NextProtos)
		}
		if len(origin.ProxyConnectHeader["X-A"]) == 0 {
			t.Fatal("ProxyConnectHeader 未继承")
		}
		if &origin.ProxyConnectHeader["X-A"][0] == &pubH["X-A"][0] {
			t.Fatal("ProxyConnectHeader 底层数组仍共享")
		}
		origin.ProxyConnectHeader["X-A"][0] = "changed"
		if pubH["X-A"][0] != "1" {
			t.Fatalf("公共 Header 被 origin 修改: %v", pubH)
		}

		pub2 := newPub()
		ownTLS := &SitePluginTLS{ServerName: "own"}
		origin2 := SitePlugin{PublicName: "pub", TLS: ownTLS}
		if !pub2.ConfigSitePluginRPC(&origin2, nil) {
			t.Fatal("合并失败")
		}
		if origin2.TLS == nil || origin2.TLS.ServerName != "own" {
			t.Fatalf("origin TLS 未覆盖公共: %+v", origin2.TLS)
		}
		if origin2.TLS == ownTLS {
			t.Fatal("origin TLS 未深拷贝（仍指向调用方原对象）")
		}
		if pub2.RPC["pub"].TLS.ServerName != "svc" {
			t.Fatalf("公共 TLS 被 origin 污染: %+v", pub2.RPC["pub"].TLS)
		}
	})

	// 3) 无效输入：nil receiver / nil origin / nil map / PublicName 空 / 未找到
	t.Run("invalid inputs", func(t *testing.T) {
		tests := []struct {
			name string
			call func() bool
		}{
			{"nil receiver", func() bool {
				var p *SitePlugins
				return p.ConfigSitePluginRPC(&SitePlugin{PublicName: "pub"}, nil)
			}},
			{"nil origin", func() bool {
				return newPub().ConfigSitePluginRPC(nil, nil)
			}},
			{"RPC map 为 nil", func() bool {
				return (&SitePlugins{}).ConfigSitePluginRPC(&SitePlugin{PublicName: "pub"}, nil)
			}},
			{"PublicName 为空", func() bool {
				return newPub().ConfigSitePluginRPC(&SitePlugin{}, nil)
			}},
			{"PublicName 未找到", func() bool {
				return newPub().ConfigSitePluginRPC(&SitePlugin{PublicName: "missing"}, nil)
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if tc.call() {
					t.Fatal("应返回 false")
				}
			})
		}
	})
}

// -----------------------------------------------------------------------------
// ServerPublic.ConfigConn（合并语义、公共模板隔离、bool 零值陷阱）
// -----------------------------------------------------------------------------

func TestServerPublic_ConfigConn(t *testing.T) {
	newPub := func() *ServerPublic {
		return &ServerPublic{CC: map[string]Conn{
			"pub": {
				Deadline: 1000, WriteDeadline: 2000, ReadDeadline: 3000,
				KeepAlive: true, KeepAlivePeriod: 4000, Linger: 5,
				NoDelay: true, ReadBuffer: 65536, WriteBuffer: 65536,
			},
		}}
	}

	// 1) 合并语义：origin 全零继承 / 非零覆盖 / handle 排除 / bool 零值陷阱
	t.Run("merge semantics", func(t *testing.T) {
		tests := []struct {
			name   string
			pub    *ServerPublic
			origin Conn
			handle func(string, reflect.Value, reflect.Value) bool
			want   Conn
		}{
			{
				name:   "origin 全零继承公共",
				pub:    newPub(),
				origin: Conn{PublicName: "pub"},
				want: Conn{
					PublicName: "pub",
					Deadline:   1000, WriteDeadline: 2000, ReadDeadline: 3000,
					KeepAlive: true, KeepAlivePeriod: 4000, Linger: 5,
					NoDelay: true, ReadBuffer: 65536, WriteBuffer: 65536,
				},
			},
			{
				name:   "origin 非零覆盖公共",
				pub:    newPub(),
				origin: Conn{PublicName: "pub", Deadline: 9999, Linger: -1},
				want: Conn{
					PublicName: "pub",
					Deadline:   9999, WriteDeadline: 2000, ReadDeadline: 3000,
					KeepAlive: true, KeepAlivePeriod: 4000, Linger: -1,
					NoDelay: true, ReadBuffer: 65536, WriteBuffer: 65536,
				},
			},
			{
				name:   "handle 排除字段保留公共值",
				pub:    newPub(),
				origin: Conn{PublicName: "pub", Deadline: 9999},
				handle: func(name string, _, _ reflect.Value) bool { return name == "Deadline" },
				want: Conn{
					PublicName: "pub",
					Deadline:   1000, WriteDeadline: 2000, ReadDeadline: 3000,
					KeepAlive: true, KeepAlivePeriod: 4000, Linger: 5,
					NoDelay: true, ReadBuffer: 65536, WriteBuffer: 65536,
				},
			},
			{
				// 记录当前实现行为：bool 零值 false 被 configExclude 跳过，
				// 因此公共 KeepAlive=true 无法被 origin 关掉。若将来支持 bool 覆盖，
				// 本用例会失败，提醒审视语义变更。
				name:   "bool 零值无法关闭公共开关",
				pub:    newPub(),
				origin: Conn{PublicName: "pub", KeepAlive: false, NoDelay: false},
				want: Conn{
					PublicName: "pub",
					Deadline:   1000, WriteDeadline: 2000, ReadDeadline: 3000,
					KeepAlive: true, KeepAlivePeriod: 4000, Linger: 5,
					NoDelay: true, ReadBuffer: 65536, WriteBuffer: 65536,
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				origin := tc.origin
				if !tc.pub.ConfigConn(&origin, tc.handle) {
					t.Fatal("应成功合并")
				}
				if !reflect.DeepEqual(origin, tc.want) {
					t.Fatalf("got=%+v\nwant=%+v", origin, tc.want)
				}
			})
		}
	})

	// 2) 公共 CC 不被污染：map value 是值拷贝，写回 origin 不应影响公共模板
	t.Run("公共 CC 不被污染", func(t *testing.T) {
		pub := newPub()
		origin := Conn{PublicName: "pub", Deadline: 9999, Linger: -1, ReadBuffer: 1}
		if !pub.ConfigConn(&origin, nil) {
			t.Fatal("合并失败")
		}
		if got := pub.CC["pub"]; got.Deadline != 1000 || got.Linger != 5 || got.ReadBuffer != 65536 {
			t.Fatalf("公共 CC 被污染: %+v", got)
		}
	})

	// 3) 无效输入：nil receiver / nil origin / PublicName 空 / nil map / 未找到
	t.Run("invalid inputs", func(t *testing.T) {
		tests := []struct {
			name string
			call func() bool
		}{
			{"nil receiver", func() bool {
				var p *ServerPublic
				return p.ConfigConn(&Conn{PublicName: "pub"}, nil)
			}},
			{"nil origin", func() bool {
				return newPub().ConfigConn(nil, nil)
			}},
			{"PublicName 为空", func() bool {
				return newPub().ConfigConn(&Conn{}, nil)
			}},
			{"CC map 为 nil", func() bool {
				return (&ServerPublic{}).ConfigConn(&Conn{PublicName: "pub"}, nil)
			}},
			{"PublicName 未找到", func() bool {
				return newPub().ConfigConn(&Conn{PublicName: "missing"}, nil)
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if tc.call() {
					t.Fatal("应返回 false")
				}
			})
		}
	})
}

// -----------------------------------------------------------------------------
// ServerPublic.ConfigServer（TLS 深拷贝与 CipherSuitesAuto）
// -----------------------------------------------------------------------------

func TestServerPublic_ConfigServer_TLSDeepCopy(t *testing.T) {
	pubTLS := &ServerTLS{
		RootCAs:    []ServerTLSFile{{CertFile: "cert.pem", KeyFile: "key.pem"}},
		NextProtos: []string{"h2"},
		ClientCAs:  []string{"ca.pem"},
	}
	pub := &ServerPublic{CS: map[string]Server{
		"pub": {TLS: pubTLS, ReadTimeout: 100, MaxHeaderBytes: 1024},
	}}

	origin := Server{PublicName: "pub", MaxHeaderBytes: 4096}
	if !pub.ConfigServer(&origin, nil) {
		t.Fatal("合并失败")
	}
	if origin.ReadTimeout != 100 || origin.MaxHeaderBytes != 4096 {
		t.Fatalf("字段合并错误: %+v", origin)
	}
	if origin.TLS == nil {
		t.Fatal("TLS 未设置")
	}
	// 公共 TLS 不应被修改
	if len(pubTLS.NextProtos) == 0 || pubTLS.NextProtos[0] != "h2" {
		t.Fatalf("公共 NextProtos 被修改: %v", pubTLS.NextProtos)
	}
	// origin 的 TLS 切片应独立
	if len(origin.TLS.NextProtos) > 0 && len(pubTLS.NextProtos) > 0 &&
		&origin.TLS.NextProtos[0] == &pubTLS.NextProtos[0] {
		t.Fatal("NextProtos 底层数组仍共享")
	}
	// 空 CipherSuites 应被 CipherSuitesAuto 填充
	if len(origin.TLS.CipherSuites) == 0 {
		t.Fatal("CipherSuites 应被填充")
	}
}

// -----------------------------------------------------------------------------
// ServerTLS.CipherSuitesAuto
// -----------------------------------------------------------------------------

func TestServerTLS_CipherSuitesAuto(t *testing.T) {
	tests := []struct {
		name     string
		min, max uint16
		suites   []uint16
		wantMin  uint16
		wantMax  uint16
		wantFill bool
	}{
		{
			name:   "已有套件不覆盖",
			suites: []uint16{tls.TLS_AES_128_GCM_SHA256},
		},
		{
			name:     "空套件自动填充",
			wantFill: true,
		},
		{
			name:     "Min>Max 会被 clamp",
			min:      tls.VersionTLS13 + 1,
			max:      tls.VersionTLS12,
			wantMin:  tls.VersionTLS12,
			wantMax:  tls.VersionTLS12,
			wantFill: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tlsc := &ServerTLS{MinVersion: tc.min, MaxVersion: tc.max, CipherSuites: tc.suites}
			tlsc.CipherSuitesAuto()

			if tc.wantMin != 0 && tlsc.MinVersion != tc.wantMin {
				t.Fatalf("MinVersion = %#x, want %#x", tlsc.MinVersion, tc.wantMin)
			}
			if tc.wantMax != 0 && tlsc.MaxVersion != tc.wantMax {
				t.Fatalf("MaxVersion = %#x, want %#x", tlsc.MaxVersion, tc.wantMax)
			}
			if tc.wantFill && len(tlsc.CipherSuites) == 0 {
				t.Fatal("CipherSuites 应被填充")
			}
			if !tc.wantFill && len(tc.suites) > 0 &&
				!reflect.DeepEqual(tlsc.CipherSuites, tc.suites) {
				t.Fatalf("已有 CipherSuites 被修改: %v", tlsc.CipherSuites)
			}
		})
	}

	var nilT *ServerTLS
	nilT.CipherSuitesAuto() // 不应 panic
}

// -----------------------------------------------------------------------------
// Config.ParseReader / ParseFile / ParseFiles
// -----------------------------------------------------------------------------

func TestConfig_ParseReader(t *testing.T) {
	const data = `{
		"Servers": {"Listen": {"127.0.0.1:8080": {"Status": true}}},
		"Sites":   {"Site": [{"Identity": "s1", "Status": true, "Host": ["example.com"]}]}
	}`
	var c Config
	if err := c.ParseReader(strings.NewReader(data)); err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	if _, ok := c.Servers.Listen["127.0.0.1:8080"]; !ok {
		t.Fatalf("Listen 未解析: %+v", c.Servers.Listen)
	}
	if len(c.Sites.Site) != 1 || c.Sites.Site[0].Identity != "s1" {
		t.Fatalf("Sites 解析错误: %+v", c.Sites.Site)
	}
	if c.Servers.Public.CC == nil {
		t.Fatal("ensureMaps 未生效")
	}
}

func TestConfig_ParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"Servers":{"Listen":{"x":{}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := c.ParseFile(path); err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if _, ok := c.Servers.Listen["x"]; !ok {
		t.Fatalf("未解析 listen: %+v", c.Servers.Listen)
	}

	if err := c.ParseFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("不存在的文件应报错")
	}
}

func TestConfig_ParseFiles(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.json")
	if err := os.WriteFile(main, []byte(`{
		"Servers": {"Listen": {"127.0.0.1:8080": {"Status": true}}},
		"Sites":   {"Site": [{"Identity": "s1"}]}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.site.json"), []byte(`[
		{"Identity": "s2"},
		{"Identity": "s1", "Name": "replaced"}
	]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.listen.json"), []byte(`{
		"127.0.0.1:9090": {"Status": true}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var c Config
	if err := c.ParseFiles(main); err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if len(c.Sites.Site) != 2 {
		t.Fatalf("Sites 长度 = %d, want 2", len(c.Sites.Site))
	}
	found := map[string]Site{}
	for _, s := range c.Sites.Site {
		found[s.Identity] = s
	}
	if found["s1"].Name != "replaced" {
		t.Fatalf("s1 应被同名替换: %+v", found["s1"])
	}
	if _, ok := found["s2"]; !ok {
		t.Fatal("s2 未加载")
	}
	if len(c.Servers.Listen) != 2 {
		t.Fatalf("Listen 长度 = %d, want 2", len(c.Servers.Listen))
	}
}

// -----------------------------------------------------------------------------
// SiteForward.Compile
// -----------------------------------------------------------------------------

func TestSiteForward_Compile(t *testing.T) {
	fw := &SiteForward{
		Path:   []string{`^/api/(?P<id>\d+)$`},
		RePath: "/v1/$id",
	}
	r1, err := fw.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	r2, err := fw.Compile()
	if err != nil {
		t.Fatalf("Compile 第二次: %v", err)
	}
	if r1 != r2 {
		t.Fatal("Compile 应返回缓存对象")
	}

	bad := &SiteForward{Path: []string{"["}}
	if _, err := bad.Compile(); err == nil {
		t.Fatal("无效正则应报错")
	}
}

// -----------------------------------------------------------------------------
// SiteDirectory.RootDir
// -----------------------------------------------------------------------------

func TestSiteDirectory_RootDir(t *testing.T) {
	t.Run("无虚拟目录", func(t *testing.T) {
		d := &SiteDirectory{Root: "/var/www"}
		if got, want := d.RootDir("/index.html"), filepath.FromSlash("/var/www"); got != want {
			t.Fatalf("got=%q want=%q", got, want)
		}
	})

	t.Run("虚拟目录命中，返回其父目录", func(t *testing.T) {
		d := &SiteDirectory{Root: "/default", Virtual: []string{"/data/upload"}}
		got := d.RootDir("/upload/file.txt")
		if filepath.ToSlash(got) != "/data" {
			t.Fatalf("got=%q want=/data", got)
		}
	})

	t.Run("虚拟目录未命中，返回默认 Root", func(t *testing.T) {
		d := &SiteDirectory{Root: "/default", Virtual: []string{"/data/upload"}}
		got := d.RootDir("/other/file.txt")
		if filepath.ToSlash(got) != "/default" {
			t.Fatalf("got=%q want=/default", got)
		}
	})

	t.Run("空 Virtual 项被忽略", func(t *testing.T) {
		d := &SiteDirectory{Root: "/default", Virtual: []string{"", "/data/upload"}}
		got := d.RootDir("/upload/x")
		if filepath.ToSlash(got) != "/data" {
			t.Fatalf("got=%q want=/data", got)
		}
	})
}

func Test_SiteForward(t *testing.T) {
	tests := []struct {
		forward *SiteForward
		upath   string
		rpath   string
		re      bool
	}{
		{
			forward: &SiteForward{ExcludePath: []string{"/"}},
			upath:   "/",
			rpath:   "/",
			re:      false,
		}, { // 路径被排除了，不重定向
			forward: &SiteForward{ExcludePath: []string{"/a/index.html"}, Path: []string{"/a/index.html"}, RePath: "/b/index.html"},
			upath:   "/a/index.html",
			rpath:   "/a/index.html",
			re:      false,
		}, { // 路径不匹配，不重定向
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/a/index.html"}, RePath: "/b/index.html"},
			upath:   "/c/index.html",
			rpath:   "/c/index.html",
			re:      false,
		}, { // 全部匹配，重定向
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/a/index.html"}, RePath: "/b/index.html"},
			upath:   "/a/index.html",
			rpath:   "/b/index.html",
			re:      true,
		}, { // 正则匹配，重定向
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/(\\w)/index.html"}, RePath: "/$1/b/index.html"},
			upath:   "/a/index.html",
			rpath:   "/a/b/index.html",
			re:      true,
		},
	}
	for index, test := range tests {
		fr, err := test.forward.Compile()
		if err != nil {
			t.Fatal(err)
		}
		rpath, re := fr.Rewrite(test.upath)
		if test.re != re || test.rpath != rpath {
			t.Fatalf("error %d", index)
		}
	}
}

func Test_FileParse(t *testing.T) {
	conf := &Config{}
	err := conf.ParseFile("./test/config.json")
	if err != nil {
		t.Fatal(err)
	}
}

func Test_DataParse(t *testing.T) {
	osFile, err := os.Open("./test/config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer osFile.Close()

	conf := &Config{}
	err = conf.ParseReader(osFile)
	if err != nil {
		t.Fatal(err)
	}
}

// -----------------------------------------------------------------------------
// TOML 配置支持
// -----------------------------------------------------------------------------

// TestConfig_ParseReaderTOML 验证 TOML 数据流式解析（含数组表、嵌套表、map 键带点号）。
func TestConfig_ParseReaderTOML(t *testing.T) {
	const data = `
[Servers.Listen."127.0.0.1:8080"]
Status = true

[[Sites.Site]]
Identity = "s1"
Status = true
Host = ["example.com"]
IndexFile = ["index.html", "index.htm"]

[Sites.Site.Dynamic]
Ext = [".go", ".md"]
Cache = true
`
	var c Config
	if err := c.ParseReaderTOML(strings.NewReader(data)); err != nil {
		t.Fatalf("ParseReaderTOML: %v", err)
	}
	if !c.Servers.Listen["127.0.0.1:8080"].Status {
		t.Fatalf("Listen 未解析: %+v", c.Servers.Listen)
	}
	if len(c.Sites.Site) != 1 {
		t.Fatalf("Sites 长度 = %d", len(c.Sites.Site))
	}
	s := c.Sites.Site[0]
	if s.Identity != "s1" || !s.Status {
		t.Fatalf("Site 解析错误: %+v", s)
	}
	if !reflect.DeepEqual(s.Host, []string{"example.com"}) {
		t.Fatalf("Host = %v", s.Host)
	}
	if !s.Dynamic.Cache || !reflect.DeepEqual(s.Dynamic.Ext, []string{".go", ".md"}) {
		t.Fatalf("Dynamic = %+v", s.Dynamic)
	}
	if c.Servers.Public.CC == nil {
		t.Fatal("ensureMaps 未生效")
	}

	// 无效 TOML（未闭合的表头）应报错
	if err := c.ParseReaderTOML(strings.NewReader("[unclosed")); err == nil {
		t.Fatal("无效 TOML 应报错")
	}
}

// TestConfig_ParseFile_TOML 验证 .toml 主配置文件走 TOML 解码器。
func TestConfig_ParseFile_TOML(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "config.toml")
	body := `
[Servers.Listen.x]
Status = true

[[Sites.Site]]
Identity = "s-toml"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := c.ParseFile(path); err != nil {
		t.Fatalf("ParseFile toml: %v", err)
	}
	if _, ok := c.Servers.Listen["x"]; !ok {
		t.Fatalf("Listen 未解析: %+v", c.Servers.Listen)
	}
	if len(c.Sites.Site) != 1 || c.Sites.Site[0].Identity != "s-toml" {
		t.Fatalf("Sites 解析错误: %+v", c.Sites.Site)
	}

	// 文件不存在
	if err := c.ParseFile(filepath.Join(dir, "missing.toml")); err == nil {
		t.Fatal("不存在的文件应报错")
	}

	// 坏 TOML 内容
	bad := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(bad, []byte("= invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	var c2 Config
	if err := c2.ParseFile(bad); err == nil {
		t.Fatal("损坏的 TOML 应报错")
	}
}

// TestConfig_ParseFiles_TOML 验证主 .toml + .site.toml / .listen.toml 子配置的合并语义：
//   - .site.toml 以 Identity 为准，同名替换、异名追加
//   - .listen.toml 以监听地址为键，合并到 Servers.Listen
func TestConfig_ParseFiles_TOML(t *testing.T) {
	dir := t.TempDir()

	main := filepath.Join(dir, "main.toml")
	if err := os.WriteFile(main, []byte(`
[Servers.Listen."127.0.0.1:8080"]
Status = true

[[Sites.Site]]
Identity = "s1"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// 站点子配置：s2 新增，s1 同名替换（TOML 根必须是 table，故用 [[Site]] 包裹）
	if err := os.WriteFile(filepath.Join(dir, "extra.site.toml"), []byte(`
[[Site]]
Identity = "s2"

[[Site]]
Identity = "s1"
Name = "replaced"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// 监听子配置：新增 9090
	if err := os.WriteFile(filepath.Join(dir, "extra.listen.toml"), []byte(`
["127.0.0.1:9090"]
Status = true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var c Config
	if err := c.ParseFiles(main); err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}

	if len(c.Sites.Site) != 2 {
		t.Fatalf("Sites 长度 = %d, want 2", len(c.Sites.Site))
	}
	found := map[string]Site{}
	for _, s := range c.Sites.Site {
		found[s.Identity] = s
	}
	if found["s1"].Name != "replaced" {
		t.Fatalf("s1 应被同名替换: %+v", found["s1"])
	}
	if _, ok := found["s2"]; !ok {
		t.Fatal("s2 未加载")
	}
	if len(c.Servers.Listen) != 2 {
		t.Fatalf("Listen 长度 = %d, want 2", len(c.Servers.Listen))
	}
	if _, ok := c.Servers.Listen["127.0.0.1:9090"]; !ok {
		t.Fatalf("监听子配置未加载: %+v", c.Servers.Listen)
	}
}

// TestConfig_ParseFiles_MixedTOMLJSON 验证 JSON/TOML 子配置在同一目录内可混用。
func TestConfig_ParseFiles_MixedTOMLJSON(t *testing.T) {
	dir := t.TempDir()

	main := filepath.Join(dir, "main.json")
	if err := os.WriteFile(main, []byte(`{
		"Servers": {"Listen": {"a": {"Status": true}}},
		"Sites": {"Site": [{"Identity": "s1"}]}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// TOML 站点子配
	if err := os.WriteFile(filepath.Join(dir, "extra.site.toml"), []byte(`
[[Site]]
Identity = "s2"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// JSON 监听子配置
	if err := os.WriteFile(filepath.Join(dir, "extra.listen.json"), []byte(`{"b":{"Status":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var c Config
	if err := c.ParseFiles(main); err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if _, ok := c.Servers.Listen["a"]; !ok {
		t.Fatal("主配置 a 未加载")
	}
	if _, ok := c.Servers.Listen["b"]; !ok {
		t.Fatal("JSON 子配置 b 未加载")
	}
	idents := map[string]bool{}
	for _, s := range c.Sites.Site {
		idents[s.Identity] = true
	}
	if !idents["s1"] || !idents["s2"] {
		t.Fatalf("Sites = %+v", c.Sites.Site)
	}
}

// TestConfig_TOML_SiteForward 验证 TOML 中 Forward.List 数组表可被正确解析。
func TestConfig_TOML_SiteForward(t *testing.T) {
	const data = `
[[Sites.Site]]
Identity = "s1"

[[Sites.Site.Forward.fwd.List]]
Path = ["/a", "/b"]
RePath = "/x"
End = true

[[Sites.Site.Forward.fwd.List]]
Path = ["^/api/\\d+$"]
RePath = "/v1"
RedirectCode = 301
`
	var c Config
	if err := c.ParseReaderTOML(strings.NewReader(data)); err != nil {
		t.Fatalf("ParseReaderTOML: %v", err)
	}
	if len(c.Sites.Site) != 1 {
		t.Fatalf("Sites = %+v", c.Sites.Site)
	}
	fwd, ok := c.Sites.Site[0].Forward["fwd"]
	if !ok {
		t.Fatalf("Forward.fwd 缺失: %+v", c.Sites.Site[0].Forward)
	}
	if len(fwd.List) != 2 {
		t.Fatalf("List 长度 = %d, want 2", len(fwd.List))
	}
	if !reflect.DeepEqual(fwd.List[0].Path, []string{"/a", "/b"}) {
		t.Fatalf("List[0].Path = %v", fwd.List[0].Path)
	}
	if !fwd.List[0].End {
		t.Fatal("End 未解析")
	}
	if fwd.List[1].RedirectCode != 301 {
		t.Fatalf("RedirectCode = %d", fwd.List[1].RedirectCode)
	}
}

// TestConfig_TOML_SitePluginTLS 验证 SitePlugin.TLS 指针字段可由 TOML 子表创建。
func TestConfig_TOML_SitePluginTLS(t *testing.T) {
	const data = `
[[Sites.Site]]
Identity = "s1"

[Sites.Site.Plugin.HTTP.pub]
Status = true
Addr = "1.2.3.4:80"
Scheme = "http"

[Sites.Site.Plugin.HTTP.pub.TLS]
ServerName = "svc"
InsecureSkipVerify = true
NextProtos = ["h2", "http/1.1"]
CipherSuites = [4865, 4866]
`
	var c Config
	if err := c.ParseReaderTOML(strings.NewReader(data)); err != nil {
		t.Fatalf("ParseReaderTOML: %v", err)
	}
	p := c.Sites.Site[0].Plugin.HTTP["pub"]
	if p.TLS == nil {
		t.Fatal("TLS 未解析")
	}
	if p.TLS.ServerName != "svc" || !p.TLS.InsecureSkipVerify {
		t.Fatalf("TLS = %+v", p.TLS)
	}
	if !reflect.DeepEqual(p.TLS.NextProtos, []string{"h2", "http/1.1"}) {
		t.Fatalf("NextProtos = %v", p.TLS.NextProtos)
	}
	if !reflect.DeepEqual(p.TLS.CipherSuites, []uint16{4865, 4866}) {
		t.Fatalf("CipherSuites = %v", p.TLS.CipherSuites)
	}
}

// TestConfig_TOML_IndependentParse 验证多次 TOML 解析互不共享指针/切片。
func TestConfig_TOML_IndependentParse(t *testing.T) {
	const data = `
[[Sites.Site]]
Identity = "s1"

[Sites.Site.Plugin.RPC.pub]
Addr = "1.2.3.4:5"

[Sites.Site.Plugin.RPC.pub.TLS]
NextProtos = ["h2"]
`
	var a, b Config
	if err := a.ParseReaderTOML(strings.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := b.ParseReaderTOML(strings.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	aTLS := a.Sites.Site[0].Plugin.RPC["pub"].TLS
	bTLS := b.Sites.Site[0].Plugin.RPC["pub"].TLS
	if aTLS == nil || bTLS == nil {
		t.Fatal("TLS 未解析")
	}
	if aTLS == bTLS {
		t.Fatal("多次解析应得到独立 TLS 指针")
	}
	if len(aTLS.NextProtos) > 0 && len(bTLS.NextProtos) > 0 &&
		&aTLS.NextProtos[0] == &bTLS.NextProtos[0] {
		t.Fatal("NextProtos 底层数组不应共享")
	}
	aTLS.NextProtos[0] = "changed"
	if bTLS.NextProtos[0] != "h2" {
		t.Fatalf("b 的 NextProtos 被污染: %v", bTLS.NextProtos)
	}
}

func TestConfig_ParseFiles_MissingDir(t *testing.T) {
	var c Config
	if err := c.ParseFiles("/nonexistent/dir/config.json"); err == nil {
		t.Fatal("不存在的目录应报错")
	}
}

func TestConfig_ParseReaderTOML_UnknownField(t *testing.T) {
	const data = `
[UnknownSection]
Foo = 1
`
	var c Config
	if err := c.ParseReaderTOML(strings.NewReader(data)); err == nil {
		t.Fatal("TOML 未知字段应报错")
	}
}

func TestConfigSiteForward_EmptyPublicName(t *testing.T) {
	pub := &SitePublic{Forward: map[string]SiteForwards{"p": {}}}
	origin := SiteForwards{} // 空 PublicName
	if pub.ConfigSiteForward(&origin, nil) {
		t.Fatal("PublicName 为空应返回 false")
	}
}

func TestSiteDirectory_RootDir_EmptyVirtualItem(t *testing.T) {
	d := &SiteDirectory{Root: "/root", Virtual: []string{"", "   "}}
	if got := d.RootDir("/x"); filepath.ToSlash(got) != "/root" {
		t.Fatalf("空白 Virtual 项应被忽略，got %q", got)
	}
}

func TestSiteDirectory_RootDir_TrailingSlash(t *testing.T) {
	d := &SiteDirectory{Root: "/root", Virtual: []string{"/data/"}}
	if got := d.RootDir("/other"); filepath.ToSlash(got) != "/root" {
		t.Fatalf("末尾分隔符不应导致全匹配，got %q", got)
	}
}

func TestSitePlugin_clone(t *testing.T) {
	orig := &SitePlugin{
		Addr:               "1.2.3.4",
		TLS:                &SitePluginTLS{NextProtos: []string{"h2"}},
		ProxyConnectHeader: http.Header{"X": {"1"}},
	}
	cp := orig.clone()
	if cp == orig || cp.TLS == orig.TLS {
		t.Fatal("clone 应深拷贝")
	}
	cp.TLS.NextProtos[0] = "changed"
	if orig.TLS.NextProtos[0] != "h2" {
		t.Fatal("NextProtos 应深拷贝")
	}
	cp.ProxyConnectHeader.Set("X", "changed")
	if orig.ProxyConnectHeader.Get("X") != "1" {
		t.Fatal("ProxyConnectHeader 应深拷贝")
	}
}

func TestConfigSitePluginHTTP_NilMap(t *testing.T) {
	var p SitePlugins // nil maps
	origin := &SitePlugin{PublicName: "x"}
	if p.ConfigSitePluginHTTP(origin, nil) {
		t.Fatal("nil map 应返回 false")
	}
}

func TestConfigSitePluginHTTP_PublicNameEmpty(t *testing.T) {
	p := &SitePlugins{HTTP: map[string]SitePlugin{"x": {}}}
	origin := &SitePlugin{}
	if p.ConfigSitePluginHTTP(origin, nil) {
		t.Fatal("空 PublicName 应返回 false")
	}
}

func TestCipherSuitesAuto_MinGreaterThanMax(t *testing.T) {
	tlsc := &ServerTLS{
		MinVersion: tls.VersionTLS13 + 1,
		MaxVersion: tls.VersionTLS12,
	}
	tlsc.CipherSuitesAuto()
	if tlsc.MinVersion > tlsc.MaxVersion {
		t.Fatalf("Min>Max 应被修正：%x > %x", tlsc.MinVersion, tlsc.MaxVersion)
	}
	if len(tlsc.CipherSuites) == 0 {
		t.Fatal("应填充密码套件")
	}
}

func TestConfigHTTPClient_Basic(t *testing.T) {
	c := &vweb.PluginHTTPClient{}
	conf := &SitePlugin{
		Addr:      "127.0.0.1:8080",
		Host:      "example.com",
		Scheme:    "http",
		Timeout:   5000,
		KeepAlive: 30000,
	}
	if err := configHTTPClient(c, conf); err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:8080" || c.Host != "example.com" || c.Scheme != "http" {
		t.Fatalf("基础字段未设置: %+v", c)
	}
	if c.Dialer == nil || c.Dialer.Timeout == 0 || c.Dialer.KeepAlive == 0 {
		t.Fatalf("Dialer 未设置: %+v", c.Dialer)
	}
	if c.Tr == nil {
		t.Fatal("Transport 未创建")
	}
}

func TestConfigHTTPClient_Nil(t *testing.T) {
	if err := configHTTPClient(nil, nil); err == nil {
		t.Fatal("nil 参数应报错")
	}
	c := &vweb.PluginHTTPClient{}
	if err := configHTTPClient(c, nil); err == nil {
		t.Fatal("nil conf 应报错")
	}
}

func TestConfigHTTPClient_BadProxyURL(t *testing.T) {
	c := &vweb.PluginHTTPClient{}
	conf := &SitePlugin{ProxyURL: "://bad-url"}
	if err := configHTTPClient(c, conf); err == nil {
		t.Fatal("非法 ProxyURL 应报错")
	}
}

func TestConfigRPCClient_Basic(t *testing.T) {
	c := &vweb.PluginRPCClient{}
	conf := &SitePlugin{
		Addr:      "127.0.0.1:8080",
		Path:      "/rpc",
		IdeConn:   5,
		MaxConn:   10,
		Timeout:   5000,
		KeepAlive: 30000,
	}
	if err := configRPCClient(c, conf); err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:8080" || c.Path != "/rpc" {
		t.Fatalf("字段未设置: %+v", c)
	}
	if c.ConnPool == nil {
		t.Fatal("ConnPool 未创建")
	}
	if c.ConnPool.IdleConn != 5 || c.ConnPool.MaxConn != 10 {
		t.Fatalf("连接池配置: %+v", c.ConnPool)
	}
}

func TestConfigRPCClient_Nil(t *testing.T) {
	if err := configRPCClient(nil, nil); err == nil {
		t.Fatal("nil 参数应报错")
	}
	c := &vweb.PluginRPCClient{}
	if err := configRPCClient(c, nil); err == nil {
		t.Fatal("nil conf 应报错")
	}
}

func TestConfigRPCClient_BadLocalAddr(t *testing.T) {
	c := &vweb.PluginRPCClient{}
	conf := &SitePlugin{LocalAddr: "\x00invalid"}
	if err := configRPCClient(c, conf); err == nil {
		t.Fatal("非法 LocalAddr 应报错")
	}
}
