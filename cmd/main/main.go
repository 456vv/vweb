package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/456vv/vweb/v3/cmd/main/internal/base"
	"github.com/456vv/vweb/v3/cmd/main/internal/dynamic"
	"github.com/456vv/vweb/v3/server"
	"github.com/456vv/vweb/v3/server/config"
	"github.com/456vv/x/watch"
	"github.com/fsnotify/fsnotify"
	"golang.org/x/crypto/acme/autocert"
)

var version = "App/v2.0"

var (
	fRootDir       = flag.String("RootDir", filepath.Dir(os.Args[0]), "程序根目录")
	fConfigFile    = flag.String("ConfigFile", "config.json", "配置文件")
	fLogFile       = flag.String("LogFile", "error.log", "日志文件地址")
	fCertCache     = flag.String("CertCache", "ssl/auto", "证书缓存目录")
	fAllowCertHost = flag.String("AllowCertHost", "ssl/auto/host.txt", "允许自动申请证书文件路径")
)

func main() {
	log.Printf("程序版本：%s | %s\n", server.Version, version)

	// 文件行参数
	flag.Parse()
	if flag.NFlag() == 0 {
		flag.PrintDefaults()
		return
	}

	var err error

	// 程序根目录
	if err = os.Chdir(*fRootDir); err != nil {
		panic(err)
	}
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	log.Printf("根目录：%s\n", dir)

	// 配置文件绝对地址
	if !filepath.IsAbs(*fConfigFile) {
		*fConfigFile = filepath.Join(dir, *fConfigFile)
	}

	// 日志文件对象
	if logDir := filepath.Dir(*fLogFile); logDir != "" && logDir != "." {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			panic(err)
		}
	}
	logFile, err := os.OpenFile(*fLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Println(err)
		return
	}
	defer logFile.Close()

	// 服务器
	group := server.NewGroup()
	group.ErrorLog.SetOutput(logFile)
	group.Module = dynamic.Module
	group.CertManager = &autocert.Manager{
		Prompt:      autocert.AcceptTOS,
		RenewBefore: time.Hour * 7 * 24, // 7天
		Cache:       autocert.DirCache(*fCertCache),
		HostPolicy: func(ctx context.Context, host string) error {
			// 默认不支持，需要设置ssl/auto/host.txt
			return errors.New("auto cert error")
		},
	}
	var closeOnce sync.Once
	closeGroup := func() {
		closeOnce.Do(func() {
			if err := group.Close(); err != nil {
				log.Printf("关闭服务组出现错误：%v\n", err)
			}
		})
	}
	defer closeGroup()

	// 加载自动证书允许文件
	if err := loadAutoCertHostPolicy(group.CertManager, *fAllowCertHost); err != nil {
		log.Printf("加载自动证书允许文件失败：%v\n", err)
	}

	// 文件看守
	watcher, err := watch.NewWatch()
	if err != nil {
		log.Println(err)
		return
	}
	defer watcher.Close()

	// 加载配置文件，支持子配置
	var conf config.Config
	updateConfig := func(path string) {
		if err := conf.ParseFiles(path); err != nil {
			log.Printf("加载配置文件出现错误: %s\n", err.Error())
			return
		}
		if err := group.UpdateConfig(&conf); err != nil {
			log.Printf("更新配置出现错误: %s\n", err.Error())
			return
		}
		log.Printf("加载配置文件成功: %s\n", path)
	}
	// 主动加载配置
	updateConfig(*fConfigFile)

	// 监听配置文件
	if err := watcher.Monitor(filepath.Dir(*fConfigFile), func(e fsnotify.Event) {
		switch e.Op {
		case fsnotify.Create, fsnotify.Write, fsnotify.Remove:
			if strings.HasSuffix(e.Name, ".json") {
				updateConfig(e.Name)
			}
		default:
		}
	}); err != nil {
		log.Printf("监视配置目录出现错误：%v\n", err)
	}

	// 监听自动申请证书白名单
	if err := watcher.Monitor(filepath.Clean(*fAllowCertHost), func(event fsnotify.Event) {
		switch event.Op {
		case fsnotify.Create, fsnotify.Write:
			if err := loadAutoCertHostPolicy(group.CertManager, event.Name); err != nil {
				log.Printf("重新加载自动证书允许文件失败：%v\n", err)
			}
		default:
		}
	}); err != nil {
		log.Printf("监视证书白名单出现错误：%v\n", err)
	}

	base.StartSigHandlers()
	go func() {
		<-base.Interrupted
		log.Println("收到退出信号，正在优雅关闭...")
		closeGroup()
	}()

	if err := group.Start(); err != nil {
		log.Printf("启动失败：%s\n", err)
	}
}
