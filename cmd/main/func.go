package main

import (
	"log"
	"os"
	"strings"

	"golang.org/x/crypto/acme/autocert"
)

func loadAutoCertHostPolicy(acm *autocert.Manager, p string) error {
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		log.Printf("(%s)文件内容为空或错误(%v)", p, err)
		return err
	}
	hosts := strings.Split(string(b), "\n")
	log.Printf("加载host文件自动申请证书列表: %v\n", hosts)
	acm.HostPolicy = autocert.HostWhitelist(hosts...)
	return nil
}
