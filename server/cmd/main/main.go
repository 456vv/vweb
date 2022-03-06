package	main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/456vv/vweb/v2"
	"github.com/456vv/vweb/v2/server"
	"github.com/456vv/vweb/v2/server/cmd/main/internal/base"
	"github.com/456vv/vweb/v2/server/config"
	"github.com/456vv/x/ticker"
	"github.com/456vv/x/vweb_dynamic"
	"github.com/456vv/x/watch"
	"github.com/fsnotify/fsnotify"
)

var	version	= "App/v1.0.0"

var	(
	fRootDir = flag.String("RootDir", filepath.Dir(os.Args[0]),	"程序根目录")

	fConfigFile		   = flag.String("ConfigFile", "./config.json",	"配置文件地址")
	fLogFile		   = flag.String("LogFile",	"./error.log", "日志文件地址")
	fTickRefreshConfig = flag.Int("TickRefreshConfig", 60, "定时刷新配置文件,(单位 秒)")
)

func main()	{
	log.Printf("程序版本：%s	| %s | %s\n", vweb.Version,	server.Version,	version)

	//文件行参数
	flag.Parse()
	if flag.NFlag()	== 0 {
		flag.PrintDefaults()
		return
	}

	var	(
		err		 error
		exitCall vweb.ExitCall
	)

	//非法结束退出
	base.StartSigHandlers()
	go func() {
		<-base.Interrupted
		exitCall.Free()
	}()
	defer exitCall.Free()

	//程序根目录
	if err = os.Chdir(*fRootDir); err != nil {
		panic(err)
	}
	dir, err :=	os.Getwd()
	if err != nil {
		panic(err)
	}
	log.Printf("根目录：%s\n", dir)

	//日志文件对象
	if err := os.MkdirAll(filepath.Dir(*fLogFile), 0777); err != nil {
		panic(err)
	}
	logFile, err :=	os.OpenFile(*fLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0755)
	if err != nil {
		log.Println(err)
		return
	}
	exitCall.Defer(logFile.Close)

	//服务器
	serverGroup	:= server.NewServerGroup()
	serverGroup.ErrorLog.SetOutput(logFile)
	serverGroup.DynamicTemplate	= vweb_dynamic.DefaultPlus()
	exitCall.Defer(serverGroup.Close)

	tick :=	ticker.NewTicker(time.Duration(*fTickRefreshConfig)	* time.Second)
	exitCall.Defer(tick.Stop)
	refererConfog := tick.Func(func() {
		var	cErr = "加载配置文件错误：%s\n"

		var	c =	new(config.Config)
		if err := c.ParseFile(*fConfigFile); err !=	nil	{
			log.Printf(cErr, err)
			return
		}

		if err := serverGroup.UpdateConfig(c); err != nil {
			log.Printf(cErr, err)
		}
	})

	//文件看守
	watcher, err :=	watch.NewWatch()
	if err != nil {
		log.Println(err)
		return
	}
	exitCall.Defer(watcher.Close)

	//监听配置文件
	watcher.Monitor(*fConfigFile, func(event fsnotify.Event) {
		switch event.Op	{
		case fsnotify.Create, fsnotify.Write:
			refererConfog()
		default:
		}
	})

	refererConfog()
	if err := serverGroup.Start(); err != nil {
		log.Printf("启动失败：%s\n",	err)
	}
	//非法结束进程，留给另一个线程处理退出
	time.Sleep(time.Second)
}
