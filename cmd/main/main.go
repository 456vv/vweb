package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/456vv/vweb/v2"
	"github.com/456vv/vweb/v2/cmd/main/internal/base"
	"github.com/456vv/vweb/v2/cmd/main/internal/dynamic"
	"github.com/456vv/vweb/v2/server"
	"github.com/456vv/x/ticker"
	"github.com/456vv/x/watch"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/crypto/acme/autocert"
)

var version = "App/v1.0"

var (
	fRootDir           = flag.String("RootDir", filepath.Dir(os.Args[0]), "程序根目录")
	fConfigFile        = flag.String("ConfigFile", "./config.json", "配置文件")
	fLogFile           = flag.String("LogFile", "./error.log", "日志文件地址")
	fTickRefreshConfig = flag.Int("TickRefreshConfig", 60, "定时刷新配置文件,(单位 秒)")
)

func main() {
	log.Printf("程序版本：%s | %s\n", server.Version, version)

	// 文件行参数
	flag.Parse()
	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		return
	}

	var (
		err      error
		exitCall vweb.ExitCall
	)

	// 非法结束退出
	base.StartSigHandlers()
	go func() {
		<-base.Interrupted
		exitCall.Free()
	}()
	defer exitCall.Free()

	// 程序根目录
	if err = os.Chdir(*fRootDir); err != nil {
		panic(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	log.Printf("根目录：%s\n", dir)

	// 日志文件对象
	if err := os.MkdirAll(filepath.Dir(*fLogFile), 0o644); err != nil {
		panic(err)
	}
	logFile, err := os.OpenFile(*fLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0o755)
	if err != nil {
		log.Println(err)
		return
	}
	exitCall.Defer(logFile.Close)

	// 服务器
	group := server.NewGroup()
	group.ErrorLog.SetOutput(logFile)
	group.DynamicModule = dynamic.Module()
	group.CertManager = &autocert.Manager{
		Prompt:      autocert.AcceptTOS,
		RenewBefore: time.Hour * 7 * 24, // 7天
		Cache:       autocert.DirCache("ssl/auto"),
		HostPolicy: func(ctx context.Context, host string) error {
			// 默认不支持，需要设置ssl/auto/host.txt
			return errors.New("auto cert error")
		},
	}
	exitCall.Defer(group.Close)

	// 加载自动证书允许文件
	loadAutoCertHostPolicy(group.CertManager, "ssl/auto/host.txt")

	tick := ticker.NewTicker(time.Duration(*fTickRefreshConfig) * time.Second)
	exitCall.Defer(tick.Stop)

	// 定时加载配置文件
	refererConfog := tick.Func(func() {
		ok, err := group.LoadConfigFile(*fConfigFile)
		if err != nil {
			log.Printf("加载配置文件错误：%s\n", err)
			return
		}
		if ok {
			log.Printf("加载配置文件成功\n")
		}
	})

	// 文件看守
	watcher, err := watch.NewWatch()
	if err != nil {
		log.Println(err)
		return
	}
	exitCall.Defer(watcher.Close)

	// 监听配置文件
	watcher.Monitor(*fConfigFile, func(event fsnotify.Event) {
		switch event.Op {
		case fsnotify.Create, fsnotify.Write:
			refererConfog()
		default:
		}
	})

	// 监听自动申请证书白名单
	watcher.Monitor("ssl/auto/host.txt", func(event fsnotify.Event) {
		switch event.Op {
		case fsnotify.Create, fsnotify.Write:
			loadAutoCertHostPolicy(group.CertManager, event.Name)
		default:
		}
	})

	refererConfog()
	if err := group.Start(); err != nil {
		log.Printf("启动失败：%s\n", err)
	}
	// 非法结束进程，留给另一个线程处理退出
	time.Sleep(time.Second)
}
