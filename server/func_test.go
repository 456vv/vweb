package server

import(
	"testing"
)
	
func Test_derogatoryDomain(t *testing.T){
	derogatoryDomain("www.leihe.com", func(host string) bool {
		t.Log(host)
		return false
	})
}