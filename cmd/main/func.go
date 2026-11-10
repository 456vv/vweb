package main

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"golang.org/x/crypto/acme/autocert"
)

// newDynamicHostPolicy 返回一个读取 policy 最新值的 autocert.HostPolicy。
// policy 由调用方持有，通常是 main 中的局部变量，通过闭包共享。
func newDynamicHostPolicy(policy *atomic.Pointer[autocert.HostPolicy]) autocert.HostPolicy {
	return func(ctx context.Context, host string) error {
		if p := policy.Load(); p != nil {
			return (*p)(ctx, host)
		}
		// 默认不支持，需要设置 ssl/auto/host.txt
		return errors.New("auto cert error")
	}
}

// loadAutoCertHostPolicy 加载自动申请证书白名单，并把结果写入 policy。
func loadAutoCertHostPolicy(policy *atomic.Pointer[autocert.HostPolicy], p string) error {
	b, err := os.ReadFile(p)
	if err != nil {
		log.Printf("(%s)读取失败(%v)", p, err)
		return err
	}
	if len(b) == 0 {
		log.Printf("(%s)文件内容为空", p)
		return errors.New("host 白名单文件内容为空")
	}

	lines := strings.Split(string(b), "\n")
	hosts := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		hosts = append(hosts, line)
	}
	if len(hosts) == 0 {
		log.Printf("(%s)文件中没有有效的 host", p)
		return errors.New("host 白名单文件中没有有效的 host")
	}

	log.Printf("加载host文件自动申请证书列表: %v\n", hosts)
	whitelist := autocert.HostWhitelist(hosts...)
	policy.Store(&whitelist)
	return nil
}

// watchedSubConfig 的后缀集合与 config.isSubConfigName 保持一致。
func watchedSubConfig(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".site.json"),
		strings.HasSuffix(lower, ".listen.json"),
		strings.HasSuffix(lower, ".site.toml"),
		strings.HasSuffix(lower, ".listen.toml"),
		strings.HasSuffix(lower, ".site.yaml"),
		strings.HasSuffix(lower, ".listen.yaml"),
		strings.HasSuffix(lower, ".site.yml"),
		strings.HasSuffix(lower, ".listen.yml"):
		return true
	default:
		return false
	}
}
