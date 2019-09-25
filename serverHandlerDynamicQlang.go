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
	_ "github.com/456vv/qlang/lib/builtin" // 导入 builtin 包
	qcl "github.com/456vv/qlang/cl"
	"github.com/456vv/qlang/exec"
	"github.com/456vv/qlang/spec"
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
	
	//filePath := filepath.Join(T.rootPath, T.pagePath)
	
	cl := qcl.New()
	ctx := exec.NewContextEx(cl.GlobalSymbols())
	ctx.Stack = exec.NewStack()
	ctx.Code = cl.Code()
	
	//库默认路径
	//cl.SetLibs(filepath.Dir(filePath)+"|"+T.rootPath)
	
	//库加载函数
	qcl.ReadFile = func(file string) ([]byte, error){
		//include
		if T.libReadFunc != nil {
			return T.libReadFunc(T.fileName, file)
		}
		
		return ioutil.ReadFile(T.defaultLibPath(file))
	}
	qcl.FindEntry = func(file string, libs []string) (string, error){
		//import
		//这个方法要保留，因为这样才不会调用内置的findEntry函数
		//调用了这个方法，会调用上面.ReadFile打开文件
		return file, nil
	}
	T.start = ctx.Code.Len()
	T.end = cl.Cl(cotnext, T.pagePath)
	T.pctx = ctx
	cl.Done()
	return nil
}
func (T *serverHandlerDynamicQlang) defaultLibPath(libName string) string {
	
	var (
		dirPath		= filepath.Dir(T.pagePath)
		filePath	string
	 ) 
	if libName[0] == '/' || libName[0] == '\\' {
		filePath = filepath.Clean(libName)
	}else{
		filePath = filepath.Join(dirPath, libName)
		filePath = filepath.Clean(filePath)
    }
    return filepath.Join(T.rootPath, filePath)
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
