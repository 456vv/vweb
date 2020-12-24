package main
	
import (
    "github.com/fsnotify/fsnotify"
    "path/filepath"
    "os"
    "flag"
    "log"
    "time"
    "reflect"
	"github.com/456vv/vcipher"
	"github.com/456vv/verifycode"
    "github.com/456vv/vforward"
    "github.com/456vv/vbody"
    "github.com/456vv/vweb/v2"
    "github.com/456vv/vweb/v2/builtin"
    "github.com/456vv/vweb/v2/server"
    "github.com/456vv/vweb/v2/server/watch"
	"github.com/mattn/anko/core"
	"github.com/mattn/anko/env"
	"github.com/mattn/anko/parser"
	"github.com/goplus/gop"
	"github.com/goplus/gop/exec/bytecode"
)

const version = "App/v2.5.0"

var _ *fsnotify.Op
var _ = builtin.GoTypeTo
var _ = vcipher.AES
var _ *verifycode.Color
var _ *vforward.Addr
var _ *vbody.Reader

var anko_env *env.Env
func init(){
		
	//给template模板增加模块包
	for name, pkg := range templatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range luteTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range yamlTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range tomlTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range reflectxTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}

	//增加gop 模块包
	//导入 builtin 包已经创建，现在只需要查找
	//"github.com/goplus/gop/lib/builtin"
	gopI := bytecode.FindGoPackage("").(*bytecode.GoPackage)
	if gopI == nil {
		gopI = bytecode.NewGoPackage("")
	}
	
	//增加anko 模块包
	parser.EnableErrorVerbose()	//解析错误详细信息
	anko_env = env.NewEnv()
	core.Import(anko_env) 		//加载内置的一些函数
	
	//增加内置函数
	for name, fn := range vweb.TemplateFunc {
		//anko
		anko_env.Define(name, fn)
		
		//gop
		tfn := reflect.TypeOf(fn)
		switch tfn.Kind() {
		case reflect.Func:
			fnc := func(name string, tfn reflect.Type, fn interface{}) func(arity int, p *gop.Context) {
				isVariadic := tfn.IsVariadic()
				numIn := tfn.NumIn()
				return func(arity int, p *gop.Context){
					args := p.GetArgs(arity)
					if isVariadic && len(args) != numIn {
						args = append(args, []interface{}{})
					}
					log.Printf("calling %s(%v)\n", name, args)
					retn, err := vweb.ExecFunc(fn, args...)
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

}

var (
	fRootDir			= flag.String("RootDir", filepath.Dir(os.Args[0]), "程序根目录")
	
	fConfigFile 		= flag.String("ConfigFile", "./config.json", "配置文件地址")
	fLogFile 			= flag.String("LogFile", "./error.log", "日志文件地址")
	fTickRefreshConfig	= flag.Int("TickRefreshConfig", 60, "定时刷新配置文件,(单位 秒)")
)

func main(){
	log.Printf("程序版本：%s | %s | %s\n", vweb.Version, server.Version, version)
	
	//文件行参数
	flag.Parse()
	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		return
	}
	
	var err error
	
	//程序根目录
	if err = os.Chdir(*fRootDir); err != nil {
		panic(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	log.Printf("根目录：%s\n", dir)
	
	//日志文件对象
	if err = os.MkdirAll(filepath.Dir(*fLogFile), 0777); err != nil {
		panic(err)
	}
	logFile, err := os.OpenFile(*fLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0755)
	if err != nil {
		log.Println(err)
		return
	}
	defer logFile.Close()
	
	//服务器
	serverGroup := server.NewServerGroup()
	serverGroup.DynamicTemplate = map[string]vweb.DynamicTemplateFunc{
		"ank": vweb.DynamicTemplateFunc(func() vweb.DynamicTemplater {
			return &serverHandlerDynamicAnko{Env: anko_env}
		}),
		"gop": vweb.DynamicTemplateFunc(func() vweb.DynamicTemplater {
			return &serverHandlerDynamicGoPlus{Env: vweb.TemplateFunc}
		}),
	}
	defer serverGroup.Close()
	
	//设置
	serverGroup.ErrorLog.SetOutput(logFile)
	_, ok, err := serverGroup.LoadConfigFile(*fConfigFile)
	if err != nil {
		log.Printf("加载配置文件错误：%s\n", err)
	}
	if ok {
		log.Printf("配置文件成功: %s\n", *fConfigFile)
	}
	timeTicker := time.NewTicker(time.Duration(*fTickRefreshConfig) * time.Second)
	defer timeTicker.Stop()
	go func(){
		for _ = range timeTicker.C {
			_, ok, err := serverGroup.LoadConfigFile(*fConfigFile)
			if err !=  nil {
				log.Printf("配置文件错误：%s\n", err)
				continue
			}
			if ok {
				log.Println("配置文件更新!")
			}
		}
	}()
	
	//文件看守
	watcher, err := watch.NewWatch()
	if err != nil {
		log.Println(err)
		return
	}
	defer watcher.Close()
	
	//监听配置文件
	watcher.Monitor(*fConfigFile, func(event fsnotify.Event) {
		switch event.Op {
		case fsnotify.Create, fsnotify.Write:
			_,ok, err := serverGroup.LoadConfigFile(*fConfigFile)
			if err !=  nil {
				log.Printf("配置文件错误：%s\n", err)
				return
			}
			if ok {
				log.Println("配置文件更新!")
			}
		default:
		}
	})

	err = serverGroup.Start()
	if err != nil {
		log.Printf("启动失败：%s\n", err)
	}
}