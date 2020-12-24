package main

import (
	"bufio"
	"bytes"
	"path/filepath"
	"fmt"
	"errors"
	"reflect"
	"text/template"
	"github.com/goplus/gop/exec/bytecode"
	"github.com/goplus/gop/ast"
	"github.com/goplus/gop/cl"
	"github.com/goplus/gop/parser"
	"github.com/goplus/gop/token"
	"github.com/goplus/gop/exec.spec"
	_ "github.com/goplus/gop/lib/builtin"
    _ "github.com/456vv/goplus_lib"
)

type serverHandlerDynamicGoPlus struct{
	rootPath			string																// 文件目录
	pagePath			string																// 文件名称
 	fileName			string
 	
 	Env					template.FuncMap
 	ctx					*bytecode.Context
 	fset				*token.FileSet
 	pkgs				map[string]*ast.Package //存放每个文件的代码
 	mainFn				exec.FuncInfo
 	inited				bool
 	i					*bytecode.GoPackage
}
func (T *serverHandlerDynamicGoPlus) init(){
	if cl.CallBuiltinOp == nil {
		cl.CallBuiltinOp = bytecode.CallBuiltinOp
	}
	if T.fset == nil {
		T.fset = token.NewFileSet()
	}
	if T.pkgs == nil {
		T.pkgs = make(map[string]*ast.Package)
	}
	T.i = bytecode.FindGoPackage("").(*bytecode.GoPackage)
	if T.i == nil {
		T.i = bytecode.NewGoPackage("")
	}
	T.inited = true
}
func (T *serverHandlerDynamicGoPlus) ParseText(name, content string) error {
	T.init()
	pkgs, err := parser.Parse(T.fset, name, content, 0)
	if err != nil {
		return err
	}
	for n, p := range pkgs {
		T.pkgs[n]=p
	}
	return nil
}
func (T *serverHandlerDynamicGoPlus) ParseFile(path string) error {
	T.init()
	src, err := parser.ParseFile(T.fset, path, nil, 0)
	if err != nil {
		return err
	}
	name := src.Name.Name
	pkg, found := T.pkgs[name]
	if !found {
		pkg = &ast.Package{
			Name:  name,
			Files: make(map[string]*ast.File),
		}
		T.pkgs[name] = pkg
	}
	pkg.Files[path] = src
	return nil
}
func (T *serverHandlerDynamicGoPlus) ParseDir(dir string) error {
	T.init()
	pkgs, err := parser.ParseDir(T.fset, dir, nil, 0)
	if err != nil {
		return err
	}
	for n, p := range pkgs {
		T.pkgs[n]=p
	}
	return nil
}
func (T *serverHandlerDynamicGoPlus) SetPath(root, page string){
	T.rootPath = root
	T.pagePath = page
    if T.fileName == "" {
    	T.fileName = filepath.Base(T.pagePath)
    }

}
func (T *serverHandlerDynamicGoPlus) Parse(r *bufio.Reader) (err error) {
	T.init()
	pkgs, err := parser.Parse(T.fset, T.fileName, r, 0)
	if err != nil {
		return err
	}
	for n, p := range pkgs {
		T.pkgs[n]=p
	}
	return nil
}
func (T *serverHandlerDynamicGoPlus) Execute(out *bytes.Buffer, in interface{}) (err error) {
	if !T.inited {
		return errors.New("The template has not been parsed and is not available!")
	}
	

	
	//第一次
	if T.ctx == nil {
		
		//设置入口标识
		tpkg := getPkg(T.pkgs)
		pkgAct := cl.PkgActClAll
		entrance := "init"
		if tpkg.Name == "main" {
			entrance = tpkg.Name
			pkgAct = cl.PkgActClMain
		}
		//组装一个模块
		builder := bytecode.NewBuilder(nil)
		pkg, err := cl.NewPackage(builder.Interface(), tpkg, T.fset, pkgAct)
		if err != nil {
			return err
		}
		
		//判断模块有没有入口
		kind, sym, ok := pkg.Find(entrance)
		if !ok || kind != cl.SymFunc {
			return fmt.Errorf("function %s.%s() not found", tpkg.Name, entrance)
		}
		
		//创建代码上下文
		code := builder.Resolve()
		T.ctx = bytecode.NewContext(code)
		
		T.mainFn = sym.(*cl.FuncDecl).Compile()
	}
	
	ch := make(chan bool)
	T.ctx.Go(0, func(goctx *bytecode.Context){
		defer close(ch)
		
		if T.mainFn.NumIn() == 1 {
			//非可变参数
			T.mainFn.Args(reflect.TypeOf(in))
			goctx.Push(in)
		}
		goctx.Call(T.mainFn)
		
		if T.mainFn.NumOut() == 1 {
			if v,ok := goctx.Get(-1).(string); ok {
				out.WriteString(v)
			}
		}
	})
	
	<-ch
	return nil
}

func getPkg(pkgs map[string]*ast.Package) *ast.Package {
	for _, pkg := range pkgs {
		return pkg
	}
	return nil
}















