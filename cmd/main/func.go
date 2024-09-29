package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/456vv/vweb/v2/server/config"
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

func parseSubconf(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, v)
	return err
}

func loadSubConf(path string, conf *config.Config) error {
	if strings.HasSuffix(path, ".site.json") {
		var site []config.Site
		if err := parseSubconf(path, &site); err != nil {
			return err
		}
	V:
		for i, v := range conf.Sites.Site {
			if len(site) == 0 {
				break V
			}

		V1:
			for i1, v1 := range site {
				if v.Identity == v1.Identity {
					// 先替换
					conf.Sites.Site[i] = v1

					// 删除已替换的
					p := i1 + 1
					if p >= len(site) {
						p = len(site)
					}
					site = append(site[:i1], site[p:]...)
					break V1
				}
			}

		}

		conf.Sites.Site = append(conf.Sites.Site, site...)
	}
	if strings.HasSuffix(path, ".listen.json") {
		var listen map[string]config.Listen
		if err := parseSubconf(path, &listen); err != nil {
			return err
		}
		for k, v := range listen {
			conf.Servers.Listen[k] = v
		}
	}
	return nil
}

func loadConfog(path string, conf *config.Config) error {
	err := conf.ParseFile(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	// 加载子配置文件
	dirs, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, v := range dirs {
		if err = loadSubConf(filepath.Join(dir, v.Name()), conf); err != nil {
			return err
		}
	}
	return nil
}
