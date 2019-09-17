package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"path/filepath"

	"github.com/456vv/vweb"
	"github.com/fsnotify/fsnotify"
    "github.com/456vv/vbody"
    "github.com/456vv/vcipher"
)

const version = "v1.5.3"
var (
	//版本管理不完整，有bug。需要这样解决。
	_ = (*vcipher.Cipher)(nil)
	_ = (*vbody.Reader)(nil)
)

var (
	fBackstage			= flag.Bool("Backstage", false, "后台启动进程")
	fRootDir			= flag.String("RootDir", filepath.Dir(os.Args[0]), "程序根目录")
	
	fConfigFile 		= flag.String("ConfigFile", "./config.json", "配置文件地址")
	fLogFile 			= flag.String("LogFile", "./error.log", "日志文件地址")
	fTickRefreshConfig	= flag.Int("TickRefreshConfig", 60, "定时刷新配置文件,(单位 秒)")
	fRecoverSession		= flag.Int("RecoverSession", 1200000, "定时刷新Session会话,(单位 毫秒)")
)

//dotFuncMap.go Watch.go -ConfigFile config.json
func main() {
	log.Printf("程序版本：%s/%s\n", vweb.Version, version)
	
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
	
	//设置默认池的Session刷新时间
	vweb.DefaultSitePool.SetRecoverSession(time.Duration(*fRecoverSession)*time.Millisecond)

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
	defer sg.Close()
	
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
	
    if !*fBackstage {
		time.Sleep(time.Second)
		go func() {
			defer sg.Close()
			log.Println("V WEB Server 启动了")
			var in0 string
			for err == nil {
				log.Println("输入任何字符，并回车可以退出 V WEB Server!")
				fmt.Scan(&in0)
				if in0 != "" {
					log.Println("V WEB Server 退出了")
					return
				}
			}
		}()
	}
	err = sg.Start()
	if err != nil {
		log.Printf("启动失败：%s\n", err)
	}
}

