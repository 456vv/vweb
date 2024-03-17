package vweb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/456vv/verror"
)

type DynamicTemplater interface {
	SetPath(rootPath, pagePath string)    // 设置路径
	Parse(r io.Reader) (err error)        // 解析
	Execute(out io.Writer, dot any) error // 执行
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
	BuffSize     int                                                             // 缓冲块大小
	Site         *Site                                                           // 网站配置
	Context      context.Context                                                 // 上下文
	Module       map[string]DynamicTemplateFunc                                  // 支持更动态文件类型
	SaveStatic   func(filePath string, r io.Reader, l int) (int, error)          // 静态结果。仅在 .ServeHTTP 方法中使用
	ReadFile     func(filePath string, u *url.URL) (io.Reader, time.Time, error) // 读取文件。仅在 .ServeHTTP 方法中使用
	ReplaceParse func(name string, p []byte) []byte
	exec         DynamicTemplater
	modeTime     time.Time
}

// ServeHTTP 服务HTTP
//
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if T.PagePath == "" {
		T.PagePath = req.URL.Path
	}
	filePath := filepath.Join(T.RootPath, T.PagePath)

	var (
		tmplread io.Reader
		modeTime time.Time
		err      error
	)
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
		if err = T.Parse(tmplread); err != nil {
			webError(rw, err.Error())
			return
		}
	}

	// 模板点
	dock := &Dot{
		R:        req,
		W:        rw,
		BuffSize: T.BuffSize,
		Site:     T.Site,
	}
	defer dock.Close()

	ctx := T.Context
	if ctx == nil {
		ctx = req.Context()
	}
	dock.WithContext(ctx)
	body := new(bytes.Buffer)

	// 执行模板内容
	if err = T.Execute(body, dock); err != nil {
		if !dock.writed {
			webError(rw, err.Error())
			return
		}

		io.WriteString(rw, err.Error())
		log.Println(err.Error())
		return
	}

	if !dock.writed {
		// 保存静态文件
		if T.SaveStatic != nil && dock.staticPath != "" {
			br := io.TeeReader(body, rw)
			if _, err = T.SaveStatic(dock.staticPath, br, body.Len()); err != nil {
				io.WriteString(rw, err.Error())
				log.Println(err.Error())
				return
			}
		}

		// 写入到浏览器页面中去
		if body.Len() != 0 {
			body.WriteTo(rw)
		}
	}
}

// ParseText 解析模板
//
//	name, content string	模板名称, 模板内容
//	error					错误
func (T *ServerHandlerDynamic) ParseText(name, content string) error {
	T.PagePath = name
	r := strings.NewReader(content)
	return T.Parse(r)
}

// ParseFile 解析模板
//
//	path string			模板文件路径，如果为空，默认使用RootPath,PagePath字段
//	error				错误
func (T *ServerHandlerDynamic) ParseFile(path string) error {
	if path == "" {
		path = filepath.Join(T.RootPath, T.PagePath)
	} else if !filepath.IsAbs(path) {
		T.PagePath = path
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	defer file.Close()
	return T.Parse(file)
}

// Parse 解析模板
//
//	r io.Reader			模板内容
//	error				错误
func (T *ServerHandlerDynamic) Parse(r io.Reader) (err error) {
	if T.PagePath == "" {
		return verror.TrackError("vweb: ServerHandlerDynamic.PagePath is not a valid path")
	}

	buf, ok := r.(*bytes.Buffer)
	if T.ReplaceParse != nil {
		b, err := io.ReadAll(r)
		if err != nil {
			return verror.TrackErrorf("vweb: ServerHandlerDynamic.ReplaceParse failed to read data: %s", err.Error())
		}
		b = T.ReplaceParse(T.PagePath, b)
		if ok {
			buf.Write(b)
		} else {
			buf = bytes.NewBuffer(b)
		}
	} else if !ok {
		buf = bytes.NewBuffer(nil)
		buf.Grow(4096)
		buf.ReadFrom(r)
	}

	// 文件首行
	firstLine, err := buf.ReadBytes('\n')
	if err != nil || len(firstLine) == 0 {
		return verror.TrackErrorf("vweb: Dynamic content is empty! Error: %s", err.Error())
	}
	drop := 0
	if firstLine[len(firstLine)-1] == '\n' {
		drop = 1
		if len(firstLine) > 1 && firstLine[len(firstLine)-2] == '\r' {
			drop = 2
		}
		firstLine = firstLine[:len(firstLine)-drop]
	}

	dynmicType := string(firstLine)
	if T.Module == nil || len(dynmicType) < 2 {
		return errors.New("vweb: The file type of the first line of the file is not recognized")
	}
	m, ok := T.Module[strings.TrimSpace(dynmicType[2:])]
	if !ok {
		return errors.New("vweb: The file type does not support dynamic parsing")
	}
	shdt := m(T)
	shdt.SetPath(T.RootPath, T.PagePath)
	if err = shdt.Parse(buf); err != nil {
		return
	}
	T.exec = shdt
	return
}

// Execute 执行模板
//
//	bufw *bytes.Buffer	模板返回数据
//	dock any	与模板对接接口
//	error				错误
func (T *ServerHandlerDynamic) Execute(bufw io.Writer, dock any) (err error) {
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

	return T.exec.Execute(bufw, dock)
}
