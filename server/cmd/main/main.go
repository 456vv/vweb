package main
	
import (
    "github.com/fsnotify/fsnotify"
	"github.com/456vv/vweb/v2"
    "github.com/456vv/vweb/v2/server"
    "github.com/456vv/vweb/v2/server/watch"
    "path/filepath"
    "os"
    "flag"
    "log"
    "time"
)

const version = "App/v2.5.0"

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
			return &serverHandlerDynamicAnko{}
		}),
		//"gop": vweb.DynamicTemplateFunc(func() vweb.DynamicTemplater {
		//	return &serverHandlerDynamicGoPlus{}
		//}),
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