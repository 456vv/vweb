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
