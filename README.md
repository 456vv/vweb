# vweb

一个用 Go 编写的高性能、模块化、可扩展的 Web 服务器框架。内置多站点托管、动态模板执行、插件化 RPC/HTTP 通信、配置热重载、Let's Encrypt 自动证书、会话管理、URL 重写、静态文件服务等能力，并提供一套安全的反射函数库（`builtin`）供动态模板与脚本层调用。

---

## 特性

- **多站点托管**：一个进程管理多个站点，按 `Host` 隔离；支持通配符域名匹配（如 `*.example.com`）。
- **多监听地址**：通过配置即可同时启动任意数量的 `IP:Port` 监听；连接参数、服务器参数、TLS 均可独立配置，支持热重载。
- **动态模板执行**：同一站点可按文件后缀路由到不同动态引擎（`Yaegi`、`Ixgo`/`Igop`、`Wazero`/WASM）；支持源码缓存（`Cache`、`CacheParseTimeout`）与静态化缓存（`CacheStaticFileDir`、`CacheStaticTimeout`）。
- **路由能力**：
  - 精确匹配（`O(1)`）
  - 正则匹配（以 `^`、`$` 界定）
  - 通配符匹配（`[]*?\`）
  - 占位符匹配（如 `/user/{id}`、`b{id}b`）
- **反向代理 / URL 重写**：规则在配置加载阶段预编译；支持重定向（`RedirectCode`）、排除路径、连续匹配（`End`）。
- **插件系统**：
  - `PluginRPC`：基于 `net/rpc` 与 `ugorji/go/codec`（MessagePack）的连接池 RPC 客户端
  - `PluginHTTP`：基于 `http.Transport` 的反向代理 / 网关
  - `plugin` 包同时提供 `ServerRPC` 与 `ServerHTTP`，可独立部署为 RPC/HTTP 后端
- **会话管理**：可配置 Cookie 名称、ID 长度、空闲过期、加盐；提供 `Defer` 退出回调、`InstantDeadAll` 批量清理、并发受限的 Defer 执行。
- **全局缓存与会话**：基于 `vmap`，支持键级别过期、过期回调、遍历、重置。
- **静态文件服务**：支持 `GET/HEAD`、`Range`（单/多区间、后缀区间）、`ETag`、`If-None-Match`、`If-Range`、`Cache-Control`、`Last-Modified`、多段 `multipart/byteranges` 响应；拒绝 FIFO/设备等非普通文件。
- **HTTPS / TLS**：手动加载证书（PEM/DER/CRT）、多证书、`ClientCAs` 双向认证、可选密码套件、TLS 版本约束、`NextProtos` 配置、`autocert` 自动申请证书。
- **Let's Encrypt 自动证书**：白名单 `host.txt` 动态热重载，白名单为空时默认拒绝自动申请。
- **配置热重载**：主配置及子配置变更后自动重载并重建站点、服务器与插件；支持 `config.json`、`config.toml`、`config.yaml` 及对应的 `*.site.*`、`*.listen.*` 子配置。
- **优雅关闭**：服务组 `Close`、站点池 `Close`、动态模板 `Close` 全部保证清理与 `Defer` 执行，`ShutdownConn=true` 时使用 `http.Server.Shutdown`。
- **健壮性**：路径穿越 / 符号链接逃逸防护、临时文件原子提交、动态模板引用计数、并发安全 `map` 快照、`WASM` 头探测、BOM 兼容、`Range` 段数上限防放大攻击等。
- **内置函数库**：`builtin` 包提供动态模板可用的 `Get` / `Set` / `Make` / `Compute` / `Len` / `Append` / `Copy` / `Convert` / `Init` / `CopyStruct` / `CopyStructDeep` / `GetSlice` / `SetSlice` / `DepthField` / `Chan` 操作等，兼顾安全与性能。

---

## 快速开始

### 环境要求

- Go **1.22+**（推荐 1.25+，因为项目使用了 `errors.AsType`、`sync.WaitGroup.Go` 等较新的标准库特性；若需回退到低版本，请留意这些 API）。

### 编译

```bash
git clone https://github.com/456vv/vweb.git
cd vweb
go build -o vweb ./cmd/main
```

### 运行

```bash
./vweb -RootDir=/opt/vweb -ConfigFile=config.json -LogFile=error.log
```

**启动参数**

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-RootDir` | 程序所在目录 | 程序根目录（会 `chdir`） |
| `-ConfigFile` | `config.json` | 主配置文件路径，支持 `.json` / `.toml` / `.yaml` / `.yml` |
| `-LogFile` | `error.log` | 日志文件 |
| `-CertCache` | `ssl/auto` | Let's Encrypt 证书缓存目录 |
| `-AllowCertHost` | `ssl/auto/host.txt` | 自动申请证书的域名白名单文件 |

> 提示：**不带任何 flag 时会打印帮助后退出**（`flag.NFlag() == 0`）。

---

## 项目结构

```
vweb/
├── builtin/                 # 动态模板内置函数库（Get/Set/Make/Compute/Convert/CopyStruct…）
│   ├── boolean.go           # And/Or/Not/EQ/LT/GT/… 及带 NaN 语义的比较
│   ├── builtin.go           # Get/Set/Make/Init/CopyStruct 等反射工具
│   ├── chan.go              # 通道发送/接收/关闭
│   ├── number.go            # 动态算术运算
│   └── types.go             # 类型解析、转换、指针初始化
├── cmd/main/                # 可执行入口
│   ├── internal/base/       # 跨平台信号处理
│   ├── internal/dynamic/    # 动态引擎注册表（Yaegi / Ixgo / Wazero）
│   ├── func.go              # 自动证书白名单加载、子配置识别
│   └── main.go              # 主入口
├── plugin/                  # 可独立部署的 RPC/HTTP 服务器
│   ├── ServerHTTP.go        # 标准库 http.Server 封装（TLS/KeepAlive）
│   ├── ServerRPC.go         # 基于 ugorji/go/codec 的 RPC 服务器
│   └── tcpKeepAliveListener.go
├── server/                  # 服务组、监听器、配置解析
│   ├── config/Config.go     # 配置结构、子配置合并、公共配置拷贝
│   ├── Server.go            # http.Server 封装
│   ├── listener.go          # TCP/KeepAlive/TLS 监听器
│   ├── plugin.go            # 插件客户端管理
│   └── func.go              # 通配符域名、Header、路径安全
├── PluginRPC.go             # RPC 插件客户端
├── pluginHTTP.go            # HTTP 插件客户端
├── dot.go                   # Dot 上下文（模板点）
├── route.go                 # 路由系统
├── rewrite.go               # URL 重写
├── response.go              # Response 封装
├── serverHandlerDynamic.go  # 动态模板处理
├── serverHandlerStatic.go   # 静态文件处理
├── sessions.go              # 会话管理
├── Session.go               # 单个会话
├── Site.go                  # 站点与站点池
├── cookie.go                # Cookie 工具
├── defer.go                 # 延迟调用
├── func.go                  # 通用工具
├── reflect.go               # 反射调试输出
└── vweb.go                  # 包常量与上下文 key
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
          "DisableKeepAlive": false,
          "KeepAlivePeriod": 180000,
          "Delay": false
        }
      },
      "CS": {
        "default": {
          "ReadTimeout": 15000,
          "WriteTimeout": 60000,
          "IdleTimeout": 60000,
          "DisableKeepAlives": false,
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
          "MIME": { ".wasm": "application/wasm" }
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

> 时间单位说明：`Conn` / `Server` / `SitePlugin` 的超时字段均为**毫秒**；`SiteSession.Expired`、`SiteDynamic.CacheParseTimeout`、`SiteHeaderType.PageExpired` 为**秒**；`Conn.Linger` 为**秒**。
>
> 布尔字段为「反向开关」：例如 `DisableKeepAlive: true` 表示禁用长连接，`ShutdownConn: true` 表示优雅关闭。

### 子配置文件

主配置所在目录下的以下文件会被自动加载并合并：

- `*.site.json` / `*.site.yaml` / `*.site.yml` / `*.site.toml`：按 `Identity` 合并站点（同名替换、异名追加）
- `*.listen.json` / `*.listen.yaml` / `*.listen.yml` / `*.listen.toml`：合并监听地址（同名覆盖）

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

> 说明：主配置支持 `.json` / `.toml` / `.yaml` / `.yml`；`ParseFiles` 会一次性加载主配置 + 目录内全部子配置，任一失败则整体不生效（事务性）。

---

## 路由

```go
route := new(vweb.Route)

// 精确匹配（O(1)）
route.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("ok"))
})

// 正则匹配（首尾需要 ^ / $）
route.HandleFunc(`^/api/v\d+/`, func(w http.ResponseWriter, r *http.Request) { /* ... */ })

// 通配符匹配（path.Match 语义）
route.HandleFunc("/static/*", func(w http.ResponseWriter, r *http.Request) { /* ... */ })

// 占位符匹配
route.HandleFuncDot("/user/{id}", func(d vweb.Doter) {
    params, _ := d.Context().Value(vweb.URLParamsContextKey).(map[string]string)
    d.Response().WriteString("id=" + params["id"])
})
```

占位符规则：

- `{name}` 提取任意非空值
- `b{id}b` 支持前后缀
- 不允许相邻占位符 `{a}{b}`
- 参数解析结果以 `map[string]string` 存入请求 context，键为 `vweb.URLParamsContextKey`

> 提示：以 `^` 或 `$` 开头/结尾的路由进入正则匹配，反之进入通配符或占位符匹配。正则与通配符路由按注册顺序遍历，精确匹配优先命中。

---

## 插件系统

### RPC 插件（`PluginRPC`）

服务端与客户端均使用 **`ugorji/go/codec`** 的 `MessagePack` 编解码（非 `gob`）。`Register` 为空实现，类型字段的接口支持通过 `MsgpackHandle` 的 `MapType` / `SetExt` 配置。

```go
import (
    "github.com/456vv/vweb/v3"
    "github.com/456vv/vconnpool/v3"
)

c := &vweb.PluginRPCClient{
    ConnPool: vconnpool.NewPool(),
    Addr:     "127.0.0.1:9001",
    Path:     "/_rpc",
}
rpc, err := c.Connection()
if err != nil { /* ... */ }
defer rpc.Close() // 连接回池；失败场景请用 Discard

var reply string
err = rpc.Call("Example.Hello", "world", &reply)
```

服务端可使用 `plugin.NewServerRPC()`：

```go
import "github.com/456vv/vweb/v3/plugin"

srv := plugin.NewServerRPC()
srv.RegisterName("Example", new(ExampleService))
srv.HandleHTTP("/_rpc", "/_rpc/debug") // 开启 HTTP CONNECT 隧道 + 只读 debug 页
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

> `pluginHTTP` 会剥离逐跳标头（`Connection` / `Keep-Alive` / `Transfer-Encoding` 等），`RoundTrip` 使用请求自身的 context，可被上层路由异步调用。

---

## 动态模板

`ServerHandlerDynamic` 依据文件第一行（注释）或 WASM 文件头选择动态引擎：

- 首行 `// yaegi` → Yaegi
- 首行 `// igop` / `// ixgo` → Ixgo
- 首行 `// wasm-01000000` 或 WASM 二进制（`\0asm`）→ Wazero
- 其他：文件第一行**必须**是 `// <engine>` 注释（除 WASM 外）

一个动态文件示例：

```go
// yaegi
package main

import "github.com/456vv/vweb/v3"

func Main(d vweb.Doter) {
    d.Response().WriteString("Hello, " + d.Request().URL.Path)
}
```

请求路径会映射为执行入口函数名：

- `index.go` → `Main`
- `about.go` → `About`
- `foo-bar.go`（含非标识符字符）→ `Main`

`ServerHandlerDynamic` 通过引用计数（`refCountedTemplater`）保证并发 `Execute` 与 `Close` 不冲突：即使站点热重载替换了处理器，进行中的请求仍能完成，底层引擎只 `Close` 一次。

---

## 会话与缓存

```go
// 在 Dot 中访问
s := d.Session()      // Sessioner
s.Set("uid", 42)
uid := s.Get("uid")
s.SetExpired("token", 5*time.Minute)
s.SetExpiredCall("token", 5*time.Minute, func(v any) {
    log.Println("token expired:", v)
})
s.Defer(func() { log.Println("session destroyed") })

g := d.Global()       // Globaler
g.Set("site_version", "v1")
```

`Sessions` 统一负责会话 ID 生成（`crypto/rand` + 可选 `Salt`）、Cookie 写入（`HttpOnly`、`SameSite=Lax`、在 HTTPS 下自动 `Secure`）、后台过期扫描、`InstantDeadAll` 批量清理。`SitePool` 内置 1 秒一次的回收 `ticker`，`Close` 时最多等待 3 秒让用户 `Defer` 完成。

---

## 静态文件服务

`ServerHandlerStatic` 采用两阶段设计：

1. **阶段 A**：`os.Stat` + 缓存协商（`ETag` / `Last-Modified` / `If-None-Match` / `If-Modified-Since` / `If-Range`），命中 304 直接返回，不打开文件。
2. **阶段 B**：`Open` + `fstat` 与阶段 A 对比；若被替换，重建响应头并重做协商，保证响应头与正文读同一文件版本。

支持能力：

- `GET` / `HEAD`（其他方法返回 405）
- 单 / 多 `Range`、后缀区间（`bytes=-500`）、`If-Range`（仅强校验）
- 多段响应用 `multipart/byteranges`，单段响应用 `Content-Range`
- `ETag` 弱校验 / 强校验、`Cache-Control`、`Expires`
- 单请求 `Range` 段数上限 100，防止 "bytes=0-0,2-2,..." 放大攻击
- 拒绝 FIFO/设备等非普通文件，避免阻塞 goroutine

---

## 自动证书（Let's Encrypt）

`cmd/main` 内置 `autocert.Manager`：

- 证书缓存到 `-CertCache` 目录
- 域名白名单由 `-AllowCertHost` 文件控制（每行一个域名，`#` 注释）
- 文件变更后自动热重载
- 白名单为空时**默认拒绝**自动申请
- 通过 `atomic.Pointer[autocert.HostPolicy]` 无锁替换策略，避免热重载与握手竞争

---

## 优雅关闭

```go
group := server.NewGroup()
// ... 加载配置、启动 ...

base.StartSigHandlers()

go func() {
    <-base.Interrupted
    group.Close()
}()
```

`Group.Close` 会：

1. 停止所有监听（`ShutdownConn=true` 时优雅关闭，否则立即关闭）
2. 清理 `srvMan` 中的服务器
3. 关闭全部站点插件、清空动态模板缓存
4. 清理会话、全局数据、站点扩展数据
5. 关闭 `sitePool`（最多等待 3 秒让 `Defer` 完成）

---

## 安全与并发说明

- **路径安全**：`resolvePagePath` / `resolveVirtualPath` 会规范化 `..`、拒绝绝对路径与 UNC、`filepath.IsLocal` 校验、`evalPathWithinRoot` 解析符号链接后再次校验，防止穿越站点根目录。
- **临时文件**：`SaveStatic` 使用 `CreateTemp` + 同目录 `Rename` 原子提交；写入失败不发布半成品。
- **会话 ID**：`crypto/rand` 生成；ID 空间耗尽时不再回退到时间种子，而是打时间戳 + 限流告警。
- **反射解包**：`builtin.InDirect` 限制 64 层，防止自引用 `interface` 栈溢出。
- **容器转换**：`autoConvert` 限制递归 256 层，数字转换做位宽 / 溢出检查。
- **`Range`**：段数 > 100 忽略 `Range` 返回全量，防响应放大。
- **热重载**：`srvConfigChanged` 比较配置快照，只有变更才重启监听，避免无谓断连。
- **并发安全**：`Route.list`、`ServerHandlerDynamic.pageCache`、`Sessions`、`Site.Extend` 均使用 `sync.RWMutex` / `atomic` / `sync.Map` 保护；`SitePool.start` 与 `Close` 通过 `closed` 标志避免 `ticker` 与关闭竞态。
- **`unsafe` 使用**：`builtin` 在初始化时探测 `reflect.Value` 布局，仅在兼容且验证可用时启用，否则回退纯反射路径（兼容 Wasm / TinyGo / AppEngine 等）。

---

## 内置函数库 `builtin`

供动态模板 / 脚本层使用的安全反射工具集，主要 API：

| 类别 | 函数 | 说明 |
| --- | --- | --- |
| 类型 | `Type` / `MustType` / `Value` / `New` / `Register` | 类型表达式解析（`"[]int"` / `"map[string]*Foo"` / `"chan bool"` 等）、零值构造、注册新类型 |
| 创建 | `Make` / `Init` | `Make` 模拟内置 `make`（slice/map/chan/func）；`Init` 沿指针链分配 nil 指针，可选零值化 |
| 访问 | `Get` / `GetUnexported` / `DepthField` | 按字段名 / 下标 / 标签 / 数字字符串索引读取，支持 map / slice / array / struct / string / 数值 |
| 修改 | `Set` / `SetUnexported` / `Delete` | 与 `Get` 对称的写入，带事务回滚；`Set` 写嵌入指针路径时按需分配，失败时回滚 |
| 转换 | `Convert` / `To` | 通用转换，含数字位宽检查、字符串↔数字、容器元素级转换 |
| 复制 | `CopyStruct` / `CopyStructDeep` | 浅拷贝（源空不覆盖）与深拷贝（含环检测、事务性） |
| 数值 | `Compute` | 字符串 `+`、整数位运算、浮点 / 复数四则运算 |
| 切片 | `GetSlice` / `SetSlice` / `Copy` / `Append` | 与 Go 切片表达式 / `copy` / `append` 语义对齐 |
| 通道 | `Chan` / `Send` / `Recv` / `TrySend` / `TryRecv` / `Close` | 带方向检查与 panic 兜底 |
| 比较 | `EQ` / `NE` / `LT` / `GT` / `LE` / `GE` / `And` / `Or` / `Not` | 含 NaN 无序语义、int↔uint↔float 精确比较、`interface` 相等 panic 兜底 |
| 其他 | `Bool` / `Len` / `Cap` / `Panic` / `ExecFunc` | 常用工具 |

> 示例：
> ```go
> builtin.Set(&user, "Name", "张三", "Age", "18")   // 值自动转换
> age, _ := builtin.Get(user, "Age")                // 返回 18
> builtin.Init(&user)                               // 分配 nil 指针
> ```

---

## 贡献

欢迎通过 GitHub Issue / Pull Request 反馈问题或贡献代码。提交前请确保：

- `go build ./...` 与 `go vet ./...` 通过
- 关键路径覆盖测试（尤其并发、生命周期、路径安全）
- 配置 / 协议变更请同步更新文档与示例

---

## 许可

本项目基于仓库中 `LICENSE` 文件所示的许可证发布。若未附带 `LICENSE` 文件，请在使用前联系作者确认授权条款。

部分源自 Go 标准库的代码（如 `cmd/main/internal/base/signal*.go`）保留其 BSD 风格版权声明。

---

## 致谢

感谢所有依赖项目作者与贡献者的开源工作。