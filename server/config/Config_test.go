package config

import (
	"os"
	"testing"
)

func Test_SiteForward(t *testing.T) {
	tests := []struct {
		forward *SiteForward
		upath   string
		rpath   string
		re      bool
	}{
		{
			forward: &SiteForward{ExcludePath: []string{"/"}},
			upath:   "/",
			rpath:   "/",
			re:      false,
		}, { // 路径被排除了，不重定向
			forward: &SiteForward{ExcludePath: []string{"/a/index.html"}, Path: []string{"/a/index.html"}, RePath: "/b/index.html"},
			upath:   "/a/index.html",
			rpath:   "/a/index.html",
			re:      false,
		}, { // 路径不匹配，不重定向
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/a/index.html"}, RePath: "/b/index.html"},
			upath:   "/c/index.html",
			rpath:   "/c/index.html",
			re:      false,
		}, { // 全部匹配，重定向
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/a/index.html"}, RePath: "/b/index.html"},
			upath:   "/a/index.html",
			rpath:   "/b/index.html",
			re:      true,
		}, { // 正则匹配，重定向
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/(\\w)/index.html"}, RePath: "/$1/b/index.html"},
			upath:   "/a/index.html",
			rpath:   "/a/b/index.html",
			re:      true,
		},
	}
	for index, test := range tests {
		fr, err := test.forward.Compile()
		if err != nil {
			t.Fatal(err)
		}
		rpath, re := fr.Rewrite(test.upath)
		if test.re != re || test.rpath != rpath {
			t.Fatalf("error %d", index)
		}
	}
}

func Test_FileParse(t *testing.T) {
	conf := &Config{}
	err := conf.ParseFile("./test/config.json")
	if err != nil {
		t.Fatal(err)
	}
}

func Test_DataParse(t *testing.T) {
	osFile, err := os.Open("./test/config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer osFile.Close()

	conf := &Config{}
	err = conf.ParseReader(osFile)
	if err != nil {
		t.Fatal(err)
	}
}
