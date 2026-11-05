package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
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

	// 加载配置文件，支持子配置文件。
	var reloadMu sync.Mutex
	updateConfig := func() {
		reloadMu.Lock()
		defer reloadMu.Unlock()

		var next config.Config
		// 始终从主配置重建全部子配置，子配置删除后不会残留在旧对象中。
		if err := next.ParseFiles(*fConfigFile); err != nil {
			log.Printf("加载配置文件出现错误: %s\n", err)
			return
		}
		if err := group.UpdateConfig(&next); err != nil {
			log.Printf("更新配置出现错误: %s\n", err)
			return
		}
		log.Printf("加载配置文件成功: %s\n", *fConfigFile)
	}
	updateConfig()

	if err := watcher.Monitor(filepath.Dir(*fConfigFile), func(e fsnotify.Event) {
		// fsnotify.Op 是位掩码，Write|Chmod 等组合值不是独立 case。
		if e.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
			return
		}
		name := filepath.Base(e.Name)
		// 只接受主配置和子配置。目录中无关 JSON/YAML 不能触发覆盖。
		if name != filepath.Base(*fConfigFile) && !watchedSubConfig(name) {
			return
		}
		updateConfig()
	}); err != nil {
		log.Printf("监视配置目录出现错误：%v\n", err)
	}

	if err := watcher.Monitor(filepath.Clean(*fAllowCertHost), func(event fsnotify.Event) {
		// 修改原因：与配置监视相同，按位判断实际发生的事件。
		// 这不解决 HostPolicy 字段本身的并发替换问题。
		if event.Op&(fsnotify.Create|fsnotify.Write) == 0 {
			return
		}
		if err := loadAutoCertHostPolicy(group.CertManager, event.Name); err != nil {
			log.Printf("重新加载自动证书允许文件失败：%v\n", err)
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
