package vweb

import (
	"fmt"
	"bufio"
	"io/ioutil"
	"path/filepath"
	"path"
	"os"
	"bytes"
	"errors"
	"strings"
	_ "github.com/qiniu/qlang/lib/builtin" // 导入 builtin 包
	qcl "github.com/qiniu/qlang/cl"
	"github.com/qiniu/qlang/exec"
	"github.com/qiniu/qlang/spec"
)


func init(){
	spec.Import("", TemplateFuncMap)
}

type serverHandlerDynamicQlang struct {
	rootPath			string																// 文件目录
	pagePath			string																// 文件名称
	libReadFunc			func(tmplName, libName string) ([]byte, error)
 	
 	fileName			string
 	start				int
 	end					int
	pctx 				*exec.Context
}

func (T *serverHandlerDynamicQlang) parseText(content, name string) error {
	T.fileName = name
	r := bufio.NewReader(strings.NewReader(content))
	return T.parse(r)
}

func (T *serverHandlerDynamicQlang) parseFile(src string) error {
	//文件名
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	r := bufio.NewReader(bytes.NewBuffer(b))
	T.fileName = path.Base(src)
	return T.parse(r)
}

//解析
func (T *serverHandlerDynamicQlang) parse(r *bufio.Reader) (err error){
	defer func() {
		if e := recover(); e != nil {
			switch v := e.(type) {
			case string:
				err = errors.New(v)
			case error:
				err = v
			default:
				panic(e)
			}
		}
	}()
	
	cotnext, err := ioutil.ReadAll(r)
	if err != nil {
		return fmt.Errorf("vweb: Parsing dynamic file failed! %s", err.Error())
	}
	
	filePath := filepath.Join(T.rootPath, T.pagePath)
	
	cl := qcl.New()
	ctx := exec.NewContextEx(cl.GlobalSymbols())
	ctx.Stack = exec.NewStack()
	ctx.Code = cl.Code()
	
	//库默认路径
	cl.SetLibs(filepath.Dir(filePath)+"|"+T.rootPath)
	
	//库加载函数
	if T.libReadFunc != nil {
		qcl.ReadFile = func(file string) ([]byte, error){
			//include
			return T.libReadFunc(T.fileName, file)
		}
		qcl.FindEntry = func(file string, libs []string) (string, error){
			//import
			b, err := T.libReadFunc(T.fileName, file)
			return string(b), err
		}
	}
	T.start = ctx.Code.Len()
	T.end = cl.Cl(cotnext, filePath)
	T.pctx = ctx
	cl.Done()
	return nil
}

//执行
func (T *serverHandlerDynamicQlang) execute(out *bytes.Buffer, in interface{}) (err error) {
	if T.pctx == nil {
		return errors.New("vweb: The template has not been parsed yet!")
	}
	defer func() {
		if e := recover(); e != nil {
			switch v := e.(type) {
			case string:
				err = errors.New(v)
			case error:
				err = v
			default:
				panic(e)
			}
		}
	}()
	T.pctx.ResizeVars()
	T.pctx.ResetVars(nil)
	T.pctx.SetVar("W", out)
	T.pctx.SetVar("R", in)
	exec.NewFunction(nil, T.start, T.end, nil, nil, false).ExtCall(T.pctx)
	return
}
