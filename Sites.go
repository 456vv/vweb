package vweb

import (
    "github.com/456vv/vmap/v2"
)



var DefaultSites        = NewSites()                                                        // 默认站点

type Sites struct {
	Host	*vmap.Map                                                                       // map[host]*Site
}
func NewSites() *Sites {
	return &Sites{Host:vmap.NewMap()}
}

//Site 读出站点，支持贬域名。
//	host string     网站host
//	s *Site         站点
//	ok bool         如果站点存在返回true
func (ss *Sites) Site(host string) (s *Site, ok bool) {
    var inf interface{}
	derogatoryDomain(host, func(domain string) bool {
        inf, ok = ss.Host.GetHas(domain)
        return ok
	});
	if ok {
        return inf.(*Site), ok
    }
    return nil, false
}