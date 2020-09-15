package main
	
import (
    "github.com/fsnotify/fsnotify"
    "path/filepath"
    "os"
    "flag"
    "log"
    "time"
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
	_ "github.com/mattn/anko/packages" //加入默认包
)

const version = "App/v2.0.3"

var _ *fsnotify.Op
var _ = builtin.GoTypeTo
var _ = vcipher.AES
var _ *verifycode.Color
var _ *vforward.Addr
var _ *vbody.Reader


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
	
	//给template模板增加模块包
	for name, pkg := range templatePackage {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	
	//增加anko 模块包
	parser.EnableErrorVerbose()	//解析错误详细信息
	e := env.NewEnv()
	core.Import(e) 			//加载内置的一些函数
	for name, fn := range vweb.TemplateFunc {
		e.Define(name, fn)
	}
	//for name, pkg := range templatePackage {
	//	fns, ok := env.Packages[name]
	//	if !ok {
	//		fns = make(map[string]reflect.Value)
	//		env.Packages[name] = fns
	//	}
	//	for n, f := range pkg {
	//		fns[n] = reflect.ValueOf(f)
	//	}
	//}
	
	//服务器
	serverGroup := server.NewServerGroup()
	serverGroup.DynamicTemplate = map[string]vweb.DynamicTemplateFunc{
		"ank": vweb.DynamicTemplateFunc(func() vweb.DynamicTemplater {
			return &serverHandlerDynamicAnko{env: e}
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