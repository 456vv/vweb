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
	"time"
)

type DynamicTemplater interface {
	SetPath(root string, page string)
	Parse(r io.Reader) (err error)                        // 解析
	Execute(name string, out io.Writer, dot ...any) error // 执行
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
	PagePath string // 主模板文件路径

	// 可选的
	Site   *Site                                       // 网站配置
	Module func(name string) (DynamicTemplater, error) // 支持更动态文件类型

	SaveStatic func(filePath string, r io.Reader, l int) (int, error)          // 静态结果。仅在 .ServeHTTP 方法中使用
	ReadFile   func(filePath string, u *url.URL) (io.Reader, time.Time, error) // 读取文件。仅在 .ServeHTTP 方法中使用
	exec       DynamicTemplater
	modeTime   time.Time
}

// ServeHTTP 服务HTTP
//
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	var (
		tmplread io.Reader
		modeTime time.Time
		err      error
	)

	pagePath := T.PagePath
	if T.PagePath == "" {
		pagePath = path.Clean(req.URL.Path)
	}
	filePath := filepath.Join(T.RootPath, pagePath)
	if T.ReadFile != nil {
		tmplread, modeTime, err = T.ReadFile(filePath, req.URL)
		if err != nil {
			webError(rw, fmt.Sprintf("Failed to read the ReadFile! Error: %s", err.Error()))
			return
		}
		if !modeTime.Equal(T.modeTime) {
			T.exec = nil
		}
		T.modeTime = modeTime
	} else {
		osFile, err := os.Open(filePath)
		if err != nil {
			webError(rw, fmt.Sprintf("Failed to read the Open! Error: %s", err.Error()))
			return
		}
		defer osFile.Close()
		tmplread = osFile

		// 记录文件修改时间，用于缓存文件
		osFileInfo, err := osFile.Stat()
		if err != nil {
			T.exec = nil
		} else {
			modeTime = osFileInfo.ModTime()
			if !modeTime.Equal(T.modeTime) {
				T.exec = nil
			}
			T.modeTime = modeTime
		}
	}
	if T.exec == nil {
		// 解析模板内容
		if err = T.parse(tmplread); err != nil {
			webError(rw, err.Error())
			return
		}
	}

	// 模板点
	dock := &Dot{
		R:    req,
		W:    rw,
		Site: T.Site,
	}
	defer dock.Close()

	ctx := req.Context()
	if site, ok := ctx.Value(SiteContextKey).(*Site); ok {
		dock.Site = site
	}

	dock.WithContext(ctx)

	// 执行模板内容
	body := new(bytes.Buffer)
	callName := entryname(req.URL.Path)
	if err = T.execute(callName, body, dock); err != nil {
		if !dock.isWrited() {
			webError(rw, err.Error())
			return
		}

		io.WriteString(rw, err.Error())
		log.Println(err.Error())
		return
	}

	if !dock.isWrited() {
		// 写入到浏览器页面中去
		if body.Len() != 0 {
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

	// 过滤换行符
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

// parse 解析模板
//
//	r io.Reader			模板内容
//	error				错误
func (T *ServerHandlerDynamic) parse(r io.Reader) (err error) {
	// 文件第一行，确认动态文件类型
	if T.Module == nil {
		return errors.New("vweb: the file type of the first line of the file is not recognized")
	}

	buf := bufio.NewReader(r)
	dynmicType, err := fileFirstLine(buf)
	if err != nil {
		return err
	}

	T.exec, err = T.Module(string(dynmicType))
	if err != nil {
		return err
	}

	T.exec.SetPath(T.RootPath, T.PagePath)
	return T.exec.Parse(buf)
}

// execute 执行模板
//
//	bufw *bytes.Buffer	模板返回数据
//	dock any	与模板对接接口
//	error				错误
func (T *ServerHandlerDynamic) execute(name string, bufw io.Writer, dock ...any) (err error) {
	if T.exec == nil {
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

	return T.exec.Execute(name, bufw, dock...)
}
