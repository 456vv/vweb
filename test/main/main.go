package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/456vv/vweb"
	"github.com/fsnotify/fsnotify"
    "github.com/456vv/vbody"
    "github.com/456vv/vcipher"
)

var (
	//版本管理不完整，有bug。需要这样解决。
	_ = vcipher.AES
	_ = vbody.NewReader
)

var fConfigFile 		= flag.String("ConfigFile", "./config.json", "配置文件地址")
var fLogFile 			= flag.String("LogFile", "./error.log", "日志文件地址")
var fTickRefreshConfig	= flag.Int("TickRefreshConfig", 60, "定时刷新配置文件,(单位 秒)")
var fRecoverSession		= flag.Int("RecoverSession", 1200000, "定时刷新Session会话,(单位 毫秒)")

//dotFuncMap.go Watch.go -ConfigFile config.json
func main() {
	log.Printf("程序版本：%s/%s\n", vweb.Version, "v1.5")

	//os.Chdir("../")
	//文件行参数
	flag.Parse()
	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		return
	}
	var err error
	
	//设置默认池的Session刷新时间
	vweb.DefaultSitePool.SetRecoverSession(time.Duration(*fRecoverSession)*time.Millisecond)

	//日志文件对象
	logFile, err := os.OpenFile(*fLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0755)
	if err != nil {
		log.Println(err)
		return
	}
	defer logFile.Close()

	//文件看守
	watcher, err := NewWatch()
	if err != nil {
		log.Println(err)
		return
	}
	defer watcher.Close()

	//服务器
	vweb.ExtendDotFuncMap(dotFuncMap)
	sg := vweb.NewServerGroup()

	//设置
	sg.ErrorLog.SetOutput(logFile)
	sg.LoadConfigFile(*fConfigFile)
	timeTicker := time.NewTicker(time.Duration(*fTickRefreshConfig)*time.Second)
	go func(){
		for _ = range timeTicker.C {
			_,ok, err := sg.LoadConfigFile(*fConfigFile)
			if err !=  nil {
				log.Printf("加载配置文件错误：%s\n", err)
				continue
			}
			if ok {
				log.Println("配置文件更新")
			}
		}
	}()

	//监听配置文件
	watcher.Monitor(*fConfigFile, func(event fsnotify.Event) {
		switch event.Op {
		case fsnotify.Create, fsnotify.Write:
			_,ok, err := sg.LoadConfigFile(*fConfigFile)
			if err !=  nil {
				log.Printf("加载配置文件错误：%s\n", err)
				return
			}
			if ok {
				log.Println("配置文件更新")
			}
		default:
		}
	})

	time.Sleep(time.Second)
	go func() {
		defer sg.Close()
		log.Println("V WEB Server 启动了")

		var in0 string
		for err == nil {
			log.Println("输入任何字符，并回车可以退出 V WEB Server!")
			fmt.Scan(&in0)
			if in0 != "" {
				return
			}
		}
	}()
	err = sg.Start()
	if err != nil {
		log.Printf("启动失败：%s\n", err)
	}
	log.Println("V WEB Server 退出了")
}

