package main

import (
	"errors"
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/acme/autocert"
)

// loadAutoCertHostPolicy 加载自动申请证书白名单。
func loadAutoCertHostPolicy(acm *autocert.Manager, p string) error {
	if acm == nil {
		return errors.New("autocert.Manager 不能为 nil")
	}

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
		line = strings.TrimSpace(line) // 同时处理 \r、\t、前后空白
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
	acm.HostPolicy = autocert.HostWhitelist(hosts...)
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
