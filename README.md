# vweb

一个用 Go 编写的高性能、模块化、可扩展的 Web 服务器框架，内置多站点托管、动态模板执行、插件化 RPC/HTTP 通信、配置热重载、Let's Encrypt 自动证书等能力。

---

## 特性

- **多站点托管**：一个进程管理多个站点，按 `Host` 隔离；支持通配符域名匹配（如 `*.example.com`）。
- **多监听地址**：通过配置即可同时启动任意数量的 `IP:Port` 监听；服务器连接参数与 TLS 可独立配置。
- **动态模板执行**：支持 `Yaegi`（Go 解释器）、`Ixgo`/`Igop`、`Wazero`（WASM）等动态引擎；同一站点可按文件后缀路由到不同引擎。
- **路由能力**：
  - 精确匹配（`O(1)`）
  - 正则匹配（以 `^`、`$` 界定）
  - 通配符匹配（`[]*?\`）
  - 占位符匹配（如 `/user/{id}`、`b{id}b`）
- **反向代理 / URL 重写**：规则在配置加载阶段预编译；支持重定向、排除路径、连续匹配。
- **插件系统**：
  - `PluginRPC`：基于 `net/rpc` + `gob` 的 RPC 连接池
  - `PluginHTTP`：基于 `http.Transport` 的反向代理
- **会话管理**：可配置名称、ID 长度、过期时间、加盐；支持 `Defer` 退出回调与哈希失效。
- **全局缓存与会话**：基于 `vmap`，支持键级别过期、过期回调、重置。
- **静态文件服务**：支持 `GET/HEAD`、`Range`（单/多区间、后缀区间）、`ETag`、`If-None-Match`、`If-Range`、`Cache-Control` 等。
- **HTTPS 支持**：
  - 手动加载证书（PEM/DER/CRT）
  - Let's Encrypt `autocert` 自动申请与缓存
  - 白名单 `host.txt` 动态热重载
- **配置热重载**：`config.json` / `config.toml` 变更后自动重载并重建站点、服务器与插件。
- **优雅关闭**：服务组 `Close`、站点池 `Close`、动态模板 `Close` 全部保证清理与 `Defer` 执行。
- **健壮性**：并发安全、反射越界防护、路径穿越防护、WASM 头探测、临时文件原子提交等。
- **内置解释器**：`builtin` 包提供动态模板可用的 `Get/Set/Make/Compute/Len/Append/Copy/Convert/Init/CopyStruct` 等函数，兼顾安全与性能。

---

## 快速开始

### 编译

```bash
git clone https://github.com/456vv/vweb.git
cd vweb
go build -o vweb ./cmd/main
```

> 需要 **Go 1.22+**（依赖 `reflect.TypeFor`、`for range int`、`maps.Copy` 等特性）。

### 运行

```bash
./vweb -RootDir=/opt/vweb -ConfigFile=config.json -LogFile=error.log
```

**启动参数**

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-RootDir` | 程序所在目录 | 程序根目录（会 `chdir`） |
| `-ConfigFile` | `config.json` | 主配置文件路径 |
| `-LogFile` | `error.log` | 日志文件 |
| `-CertCache` | `ssl/auto` | Let's Encrypt 证书缓存目录 |
| `-AllowCertHost` | `ssl/auto/host.txt` | 自动申请证书的域名白名单 |

> 注意：**不带任何 flag 时会打印帮助后退出**（`flag.NFlag() == 0`）。

---

## 项目结构

```
vweb/
├── builtin/              # 动态模板内置函数库（Get/Set/Compute/Make/Convert…）
├── cmd/main/             # 可执行入口
│   ├── internal/
│   │   ├── base/         # 信号处理（跨平台）
│   │   └── dynamic/      # 动态引擎注册表（Yaegi/Ixgo/Wazero）
│   ├── func.go           # 自动证书白名单加载
│   └── main.go           # 主入口
├── plugin/               # 独立 RPC/HTTP 插件服务器
├── server/               # 服务组、监听器、配置解析
│   ├── config/           # 配置文件结构 & 合并逻辑
│   ├── Server.go         # http.Server 封装
│   ├── listener.go       # TCP/KeepAlive/TLS 监听器
│   ├── plugin.go         # 插件客户端管理
│   └── func.go           # 通配符域名、请求头处理
├── PluginRPC.go          # RPC 插件客户端
├── pluginHTTP.go         # HTTP 插件客户端
├── dot.go                # Dot 上下文（模板点）
├── route.go              # 路由系统
├── rewrite.go            # URL 重写
├── response.go           # Response 封装
├── serverHandlerDynamic.go # 动态模板处理
├── serverHandlerStatic.go  # 静态文件处理
├── sessions.go           # 会话管理
├── Session.go            # 单个会话
├── Site.go               # 站点与站点池
├── cookie.go             # Cookie 工具
├── defer.go              # 延迟调用
├── func.go               # 工具函数（随机 ID、域名匹配等）
├── reflect.go            # 反射调试输出
└── vweb.go               # 包常量与上下文 key
```

---

## 配置文件

### 主配置 `config.json`

```jsonc
{
  "Servers": {
    "Public": {
      "CC": {
        "default": {
          "KeepAlive": true,
          "KeepAlivePeriod": 180000,
          "NoDelay": true
        }
      },
      "CS": {
        "default": {
          "ReadTimeout": 15000,
          "WriteTimeout": 60000,
          "IdleTimeout": 60000,
          "KeepAlivesEnabled": true,
          "ShutdownConn": true
        }
      }
    },
    "Listen": {
      ":8080": {
        "Status": true,
        "CC": { "PublicName": "default" },
        "CS": { "PublicName": "default" }
      },
      ":8443": {
        "Status": true,
        "CC": { "PublicName": "default" },
        "CS": {
          "PublicName": "default",
          "TLS": {
            "RootCAs": [
              { "CertFile": "ssl/server.crt", "KeyFile": "ssl/server.key" }
            ],
            "MinVersion": 771,
            "NextProtos": ["h2", "http/1.1"]
          }
        }
      }
    }
  },
  "Sites": {
    "Public": {
      "Session": {
        "default": { "Name": "SID", "Size": 64, "Expired": 1200 }
      },
      "Header": {
        "default": {
          "MIME": { ".wasm": "application/wasm" },
          "Static": {
            ".*": { "PageExpired": 3600 }
          }
        }
      }
    },
    "Site": [
      {
        "Status": true,
        "Name": "示例站点",
        "Identity": "example",
        "Host": ["example.com", "*.example.com"],
        "IndexFile": ["index.html", "index.go"],
        "Directory": {
          "Root": "/srv/web/example",
          "Virtual": ["/static/"]
        },
        "Session": { "PublicName": "default" },
        "Header":  { "PublicName": "default" },
        "Dynamic": {
          "Ext": [".go"],
          "Cache": true,
          "CacheParseTimeout": 300
        },
        "Forward": {
          "example.com": {
            "List": [
              {
                "Status": true,
                "Path": ["^/api/(.*)$"],
                "RePath": "/gateway/$1",
                "End": true
              }
            ]
          }
        },
        "Plugin": {
          "HTTP": {
            "gw": {
              "Status": true,
              "Addr": "127.0.0.1:9000",
              "Scheme": "http",
              "Timeout": 3000
            }
          }
        },
        "ErrorPage": {
          "404": "404.html",
          "500": "500.html"
        }
      }
    ]
  }
}
```

### 子配置文件

主配置所在目录下的以下文件会被自动加载并合并：

- `*.site.json` / `*.site.toml`：按 `Identity` 合并站点列表（同名替换、异名追加）
- `*.listen.json` / `*.listen.toml`：合并监听地址

TOML 示例：

```toml
[[Site]]
Status = true
Name = "示例站点"
Identity = "example"
Host = ["example.com"]
IndexFile = ["index.html"]

[Site.Directory]
Root = "/srv/web/example"
```

---

## 路由

```go
route := new(vweb.Route)

// 精确匹配
route.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("ok"))
})

// 正则匹配
route.HandleFunc(`^/api/v\d+/`, func(w http.ResponseWriter, r *http.Request) { /* ... */ })

// 通配符匹配
route.HandleFunc("/static/*", func(w http.ResponseWriter, r *http.Request) { /* ... */ })

// 占位符匹配
route.HandleFuncDot("/user/{id}", func(d vweb.Doter) {
    params, _ := d.Context().Value("url-params").(map[string]string)
    d.Response().WriteString("id=" + params["id"])
})
```

占位符规则：

- `{name}` 提取任意非空值
- `b{id}b` 支持前后缀
- 不允许相邻占位符 `{a}{b}`

---

## 插件系统

### RPC 插件（`PluginRPC`）

```go
c := &vweb.PluginRPCClient{
    ConnPool: vconnpool.NewPool(),
    Addr:     "127.0.0.1:9001",
    Path:     "/_rpc",
}
rpc, err := c.Connection()
if err != nil { /* ... */ }
defer rpc.Discard()

var reply string
err = rpc.Call("Example.Hello", "world", &reply)
```

服务端可使用 `plugin.NewServerRPC()`：

```go
srv := plugin.NewServerRPC()
srv.RegisterName("Example", new(ExampleService))
srv.HandleHTTP("/_rpc", "/_rpc/debug")
srv.Addr = ":9001"
srv.ListenAndServe()
```

### HTTP 插件（`PluginHTTP`）

```go
c := &vweb.PluginHTTPClient{
    Tr:     &http.Transport{},
    Addr:   "127.0.0.1:9000",
    Scheme: "http",
    Host:   "backend",
}
p, _ := c.Connection()
// p 直接实现 http.Handler
http.ListenAndServe(":8080", p)
```

---

## 动态模板

`ServerHandlerDynamic` 依据文件第一行（注释）或 WASM 文件头选择动态引擎：

- 首行 `// yaegi` → Yaegi
- 首行 `// igop` / `// ixgo` → Ixgo
- 首行 `// wasm-01000000` 或 WASM 二进制（`\0asm`）→ Wazero

一个动态文件示例：

```go
// yaegi
package main

import "github.com/456vv/vweb/v3"

func Main(d vweb.Doter) {
    d.Response().WriteString("Hello, " + d.Request().URL.Path)
}
```

请求路径会映射为执行入口函数名（`index.go` → `Main`，`about.go` → `About`）。

---

## 会话与缓存

```go
// 在 Dot 中访问
s := d.Session()      // Sessioner
s.Set("uid", 42)
uid := s.Get("uid")

g := d.Global()       // Globaler
g.SetExpiredCall("token", 5*time.Minute, func(v any) {
    log.Println("token expired:", v)
})
```

会话 ID 的生成、加盐、Cookie 写入、过期扫描由 `Sessions` 统一处理；`InstantDeadAll` 会遍历并清理全部会话并执行 `Defer`。

---

## 自动证书（Let's Encrypt）

`cmd/main` 内置 `autocert.Manager`：

- 证书缓存到 `-CertCache` 目录
- 域名白名单由 `-AllowCertHost` 文件控制（每行一个域名，`#` 注释）
- 文件变更后自动重载
- 白名单为空时 **默认拒绝** 一切自动申请（`HostPolicy` 返回错误）

---

## 优雅关闭

```go
group := server.NewGroup()
// ...
base.StartSigHandlers()

go func() {
    <-base.Interrupted
    group.Close()
}()
```

`Group.Close` 会：

1. 停止所有监听（`ShutdownConn=true` 时优雅关闭，否则立即关闭）
2. 清理 `srvMan` 中的服务器
3. 清理全部站点、插件、动态模板缓存、会话、全局数据
4. 关闭 `sitePool`（最多等待 3 秒让 `Defer` 完成）

---

## 贡献

欢迎通过 GitHub Issue / Pull Request 反馈问题或贡献代码。提交前请确保：

- `go build ./...` 与 `go vet ./...` 通过
- 关键路径覆盖测试（尤其是并发与生命周期）
- 配置/协议变更请同步更新文档与示例

---

## 许可

本项目基于仓库中 `LICENSE` 文件所示的许可证发布。若未附带 `LICENSE` 文件，请在使用前联系作者确认授权条款。

部分源自 Go 标准库的代码（如 `cmd/main/internal/base/signal*.go`）保留其 BSD 风格版权声明。

---

## 致谢

感谢所有依赖项目作者与贡献者的开源工作。