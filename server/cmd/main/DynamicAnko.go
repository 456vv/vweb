package main

import (
	"bufio"
	"io/ioutil"
	"bytes"
	"path/filepath"
	"strings"
	"os"
	"errors"
	"github.com/mattn/anko/env"
	"github.com/mattn/anko/parser"
	"github.com/mattn/anko/vm"
	"github.com/mattn/anko/ast"
	"github.com/456vv/vweb/v2"
)

type serverHandlerDynamicAnko struct{
	rootPath			string																// 文件目录
	pagePath			string																// 文件名称
	env					*env.Env
 	fileName			string
 	stmt				ast.Stmt
}
func (T *serverHandlerDynamicAnko) ParseText(content, name string) error {
	T.fileName = name
	r := bufio.NewReader(strings.NewReader(content))
	return T.Parse(r)
}
func (T *serverHandlerDynamicAnko) ParseFile(path string) error {
	//文件名
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	r := bufio.NewReader(bytes.NewBuffer(b))
	T.fileName = filepath.Base(path)
	return T.Parse(r)
}
func (T *serverHandlerDynamicAnko) SetPath(root, page string){
	T.rootPath = root
	T.pagePath = page
    if T.fileName == "" {
    	T.fileName = filepath.Base(T.pagePath)
    }
}
func (T *serverHandlerDynamicAnko) Parse(r *bufio.Reader) (err error) {
	
	contact, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	
	script := string(contact)
	T.stmt, err = parser.ParseSrc(script)
	if err != nil {
		if pe, ok := err.(*parser.Error); ok {
			pe.Filename = filepath.Join(T.rootPath, T.pagePath)
		}
		return err
	}
	return nil
}

func (T *serverHandlerDynamicAnko) Execute(out *bytes.Buffer, in interface{}) (err error) {
	if T.stmt == nil {
		return errors.New("The template has not been parsed and is not available!")
	}
	if T.env == nil {
		T.env = env.NewEnv()
	}
	env := T.env.NewEnv()

	var retn interface{}
    if tdot, ok := in.(vweb.DotContexter); ok {
		retn, err = vm.RunContext(tdot.Context(), env, nil, T.stmt)
    }else{
    	retn, err = vm.Run(env, nil, T.stmt)
    }
	if err != nil {
		return err
	}
	if out != nil && retn != nil {
		if sv, ok := retn.(string); ok {
			out.WriteString(sv)
		}
	}
	return nil
}
