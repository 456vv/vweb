package main

import (
	"sync"
	"log"
	"bufio"
	"bytes"
	"path/filepath"
	"fmt"
	"errors"
	"reflect"
    "github.com/456vv/vweb/v2"
	"github.com/goplus/gop"
	"github.com/goplus/gop/exec/bytecode"
	"github.com/goplus/gop/ast"
	"github.com/goplus/gop/cl"
	"github.com/goplus/gop/parser"
	"github.com/goplus/gop/token"
	"github.com/goplus/gop/exec.spec"
	_ "github.com/goplus/gop/lib/builtin"
    _ "github.com/456vv/goplus_lib"
)

func execmerrorError(_ int, p *gop.Context) {
	args := p.GetArgs(1)
	ret0 := args[0].(error).Error()
	p.Ret(1, ret0)
}
var gopulusOnce sync.Once
type serverHandlerDynamicGoPlus struct{
	rootPath			string																// 文件目录
	pagePath			string																// 文件名称
 	name				string
 	
 	ctx					*bytecode.Context
 	fset				*token.FileSet
 	pkgs				map[string]*ast.Package //存放每个文件的代码
 	mainFn				exec.FuncInfo
 	inited				bool
}
func (T *serverHandlerDynamicGoPlus) init(){
	if T.inited {
		return
	}
	if cl.CallBuiltinOp == nil {
		cl.CallBuiltinOp = bytecode.CallBuiltinOp
	}
	if T.fset == nil {
		T.fset = token.NewFileSet()
	}
	if T.pkgs == nil {
		T.pkgs = make(map[string]*ast.Package)
	}
	gopulusOnce.Do(func(){
		gopI := bytecode.FindGoPackage("").(*bytecode.GoPackage)
		if gopI == nil {
			gopI = bytecode.NewGoPackage("")
		}
		gopI.RegisterFuncs(gopI.Func("(error).Error", (error).Error, execmerrorError))
		
		for name, fn := range vweb.TemplateFunc {
			tfn := reflect.TypeOf(fn)
			switch tfn.Kind() {
			case reflect.Func:
				fnc := func(name string, tfn reflect.Type, fn interface{}) func(arity int, p *gop.Context) {
					return func(arity int, p *gop.Context){
						args := p.GetArgs(arity)
						log.Printf("calling %s(%v)\n", name, args)
						retn, err := vweb.ExecFunc(log.Println, args...)
						if err != nil {
							log.Printf("callied %s(%v) error: %s\n", name, args, err)
						}
						p.Ret(arity, retn...)
					}
				}(name, tfn, fn)
				gopI.RegisterFuncvs(gopI.Funcv(name, fn, fnc))
			default:
				log.Printf("导入内置函数，无法识别 %s 类型\n", tfn.Kind().String())
			}
		}
	})
	T.inited = true
}

func (T *serverHandlerDynamicGoPlus) ParseText(name, content string) error {
	T.init()
	if T.name == "" {
		T.name = name
	}
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
	if T.name == "" {
		T.name = filepath.Base(path)
	}
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
    T.name = filepath.Base(T.pagePath)
}

func (T *serverHandlerDynamicGoPlus) Parse(r *bufio.Reader) (err error) {
	T.init()
	pkgs, err := parser.Parse(T.fset, T.name, r, 0)
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















