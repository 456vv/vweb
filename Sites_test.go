package vweb

import (
	"testing"
   "github.com/456vv/vmap/v2"
)


func Test_NewSites(t *testing.T){
	tests := []struct{
		host	string
		domain	string
		result	bool
	}{
		{host:"a.b.c:88", domain:"*.b.c:88", result:true},
		{host:"www.baidu.com:83", domain:"*.baidu.com:83", result:true},
		{host:"a.baidu.com:83", domain:"*.baidu.com:83", result:true},
		{host:"www.google.com:82", domain:"*.google.com:82", result:true},
		{host:"a.google.com:82", domain:"*.google.com:82", result:true},
		{host:"b.google.com:82", domain:"*.b.c:82", result:false},
	}
	for _,test := range tests {
		sites := &Sites{Host:vmap.NewMap()}
    	sites.Host.Set(test.domain, NewSite())
    	_, ok := sites.Site(test.host)
    	if ok != test.result {
    		t.Fatalf("error host(%s) domain(%s)", test.host, test.domain)
    	}
	}
}
