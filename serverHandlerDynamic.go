package vweb

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type DynamicTemplater interface {
	SetPath(root string, page string)
	Parse(r io.Reader) (err error)                        // 解析
	Execute(name string, out io.Writer, dot ...any) error // 执行
	Close() error
}
type DynamicTemplateFunc func(*ServerHandlerDynamic) DynamicTemplater

// web错误调用
func webError(rw http.ResponseWriter, v ...any) {
	// 500 服务器遇到了意料不到的情况，不能完成客户的请求。
	http.Error(rw, fmt.Sprint(v...), http.StatusInternalServerError)
}

// ServerHandlerDynamic 处理动态页面文件
type ServerHandlerDynamic struct {
	// 必须的
	RootPath string // 根目录

	// 可选的
	Site   *Site                                       // 网站配置
	Module func(name string) (DynamicTemplater, error) // 支持更动态文件类型

	ReadFile func(filePath string, u *url.URL) (io.ReadCloser, time.Time, error) // 读取文件。仅在 .ServeHTTP 方法中使用
	mu       sync.RWMutex                                                        // 保护 exec / modeTime 的并发安全
	exec     DynamicTemplater
	modeTime time.Time
}

// ServeHTTP 服务HTTP
//
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var (
		tplRead  io.ReadCloser
		modeTime time.Time
		err      error
	)

	pagePath := path.Clean(req.URL.Path)
	// 防止路径穿越（跨平台兼容）
	if strings.Contains(pagePath, "..") {
		// 简单安全检查，实际生产可更严格
		pagePath = path.Clean("/" + strings.TrimPrefix(pagePath, "/"))
	}
	// 跨平台路径拼接
	filePath := filepath.Join(T.RootPath, filepath.FromSlash(pagePath))

	if T.ReadFile != nil {
		tplRead, modeTime, err = T.ReadFile(filePath, req.URL)
		if err != nil {
			webError(rw, fmt.Sprintf("Failed to read the ReadFile! Error: %s", err.Error()))
			return
		}
		defer tplRead.Close()
	} else {
		osFile, err := os.Open(filePath)
		if err != nil {
			webError(rw, fmt.Sprintf("Failed to read the Open! Error: %s", err.Error()))
			return
		}
		defer osFile.Close()
		tplRead = osFile

		// 记录文件修改时间，用于缓存文件
		osFileInfo, err := osFile.Stat()
		if err != nil {
			webError(rw, fmt.Sprintf("Failed to stat file! Error: %s", err.Error()))
			return
		}
		modeTime = osFileInfo.ModTime()
	}

	// 并发安全检查是否需要重新解析
	T.mu.RLock()
	needParse := T.exec == nil || !modeTime.Equal(T.modeTime)
	exec := T.exec
	T.mu.RUnlock()

	if needParse {
		T.mu.Lock()
		// 双重检查
		if T.exec == nil || !modeTime.Equal(T.modeTime) {
			if T.exec != nil {
				T.exec.Close() // 释放旧实例
			}
			if err = T.parse(pagePath, tplRead); err != nil {
				T.mu.Unlock()
				webError(rw, err.Error())
				return
			}
			T.modeTime = modeTime
		}
		exec = T.exec
		T.mu.Unlock()
	}

	// 模板点
	dock := &Dot{
		R:    req,
		W:    rw,
		Site: T.Site,
	}
	defer dock.Close()

	ctx := req.Context()
	if site, ok := ctx.Value(SiteContextKey).(*Site); ok && site != nil {
		dock.Site = site
	}
	dock.WithContext(ctx)

	// 执行模板内容（使用本地副本，避免持锁）
	body := new(bytes.Buffer)
	callName := entryname(pagePath)
	if err = T.executeWith(exec, callName, body, dock); err != nil {
		log.Printf("%s\n执行模板错误: \n%s\n", pagePath, err.Error())
		if !dock.isWrited() {
			webError(rw, err.Error())
			return
		}

		io.WriteString(rw, err.Error())
		return
	}

	if !dock.isWrited() {
		// 写入到浏览器页面中去
		if body.Len() > 0 {
			body.WriteTo(rw)
		}
	}
}

func entryname(name string) string {
	base := filepath.Base(name)
	pos := strings.IndexAny(base, ".")
	if pos != -1 {
		base = base[:pos]
	}

	if base == "index" || base == "" {
		return "Main"
	}

	for _, v := range base {
		if v < '0' || (v > '9' && v < 'A') || (v > 'Z' && v < 'a') || v > 'z' {
			return "Main"
		}
	}
	return strings.ToUpper(base[:1]) + base[1:]
}

func fileFirstLine(buf *bufio.Reader) (dynamicType []byte, err error) {
	h, err := buf.Peek(8)
	if err != nil {
		if err == io.EOF {
			return nil, errors.New("vweb: The file content is empty")
		}
		return nil, err
	}

	// 可能是wasm文件，直接返回wasm
	//| 检查项					| 值（十六进制） 	  | 值（ASCII/解释） | 说明
	//| 魔数 (Magic Number)		| `00 61 73 6D` 	| `\0 a s m` 	  | **唯一标识WASM二进制格式**
	//| 版本 (Version) 			| `01 00 00 00` 	| 小端序的数字 `1`  | 当前核心标准版本为1
	if bytes.Equal(h[0:4], []byte{0x00, 0x61, 0x73, 0x6D}) {
		// wasm-01000000
		return fmt.Appendf(nil, "wasm-%0x", h[4:8]), nil
	}

	// 读取一行
	dynamicType, err = buf.ReadBytes('\n')
	if err != nil {
		return nil, errors.New("vweb: The file content is empty")
	}
	// 不是注释行退出
	if len(dynamicType) <= 2 || string(dynamicType[0:2]) != "//" {
		return nil, errors.New("vweb: The first line of the file needs to confirm the dynamic type")
	}

	// 过滤换行符（兼容 \n 与 \r\n）
	drop := 0
	if dynamicType[len(dynamicType)-1] == '\n' {
		drop = 1
		if len(dynamicType) > 1 && dynamicType[len(dynamicType)-2] == '\r' {
			drop = 2
		}
		dynamicType = dynamicType[:len(dynamicType)-drop]
	}

	dynamicType = bytes.TrimSpace(dynamicType[2:])
	// 空注释行跳过
	if len(dynamicType) == 0 {
		return nil, errors.New("vweb: The first line of comments in the file is empty")
	}

	return dynamicType, nil
}

func (T *ServerHandlerDynamic) parse(pagePath string, r io.Reader) (err error) {
	// 文件第一行，确认动态文件类型
	if T.Module == nil {
		return errors.New("vweb: the file type of the first line of the file is not recognized")
	}

	buf := bufio.NewReaderSize(r, 64*1024) // 适当增大缓冲提升性能
	dynmicType, err := fileFirstLine(buf)
	if err != nil {
		return err
	}

	T.exec, err = T.Module(string(dynmicType))
	if err != nil {
		return err
	}

	T.exec.SetPath(T.RootPath, pagePath)
	return T.exec.Parse(buf)
}

// executeWith 内部执行（支持传入已获取的 exec，避免额外加锁）
func (T *ServerHandlerDynamic) executeWith(exec DynamicTemplater, name string, bufw io.Writer, dock any) (err error) {
	if exec == nil {
		return errors.New("vweb: Parse the template content first and then call the Execute")
	}
	defer func() {
		if e := recover(); e != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			err = fmt.Errorf("vweb: Dynamic code execute error。%v\n%s", e, buf)
		}
	}()

	return exec.Execute(name, bufw, dock)
}

func (T *ServerHandlerDynamic) execute(name string, bufw io.Writer, dock any) (err error) {
	T.mu.RLock()
	exec := T.exec
	T.mu.RUnlock()
	return T.executeWith(exec, name, bufw, dock)
}

func (T *ServerHandlerDynamic) Close() error {
	T.mu.Lock()
	defer T.mu.Unlock()
	if T.exec != nil {
		err := T.exec.Close()
		T.exec = nil
		T.modeTime = time.Time{}
		return err
	}
	return nil
}
