package main

import (
    "github.com/fsnotify/fsnotify"
	"github.com/456vv/vweb/v2"
    "github.com/456vv/vweb/v2/server"
    "github.com/456vv/x/watch"
    "github.com/456vv/x/ticker"
    "github.com/456vv/x/vweb_dynamic"
    _ "github.com/456vv/x/vweb_lib"
    "path/filepath"
    "os"
    "flag"
    "log"
    "time"
)

var version = "App/v1.0.0"

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
	if err := os.MkdirAll(filepath.Dir(*fLogFile), 0777); err != nil {
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
	serverGroup.ErrorLog.SetOutput(logFile)
	serverGroup.DynamicTemplate = map[string]vweb.DynamicTemplateFunc{
		"ank": vweb.DynamicTemplateFunc(func(D *vweb.ServerHandlerDynamic) vweb.DynamicTemplater {return &vweb_dynamic.Anko{}}),
		"yaegi": vweb.DynamicTemplateFunc(func(D *vweb.ServerHandlerDynamic) vweb.DynamicTemplater {return &vweb_dynamic.Yaegi{}}),
		"template": vweb.DynamicTemplateFunc(func(D *vweb.ServerHandlerDynamic) vweb.DynamicTemplater {return &vweb_dynamic.Template{}}),
	}
	defer serverGroup.Close()
	
	tick := ticker.NewTicker(time.Duration(*fTickRefreshConfig) * time.Second)
	defer tick.Stop()
	refererConfog := tick.Func(func(){
		_, ok, err := serverGroup.LoadConfigFile(*fConfigFile)
		if err != nil {
			log.Printf("加载配置文件错误：%s\n", err)
		}
		if ok {
			log.Printf("配置文件成功: %s\n", *fConfigFile)
		}
	})
	
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
			refererConfog()
		default:
		}
	})
	
	refererConfog()
	if err := serverGroup.Start(); err != nil {
		log.Printf("启动失败：%s\n", err)
	}
}