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
		}, {
			forward: &SiteForward{ExcludePath: []string{"/A/B/C/index.html"}, Path: []string{"/a/b/c/index.html"}, RePath: "/A/B/C/index.html"},
			upath:   "/a/b/c/index.html",
			rpath:   "/A/B/C/index.html",
			re:      true,
		}, {
			forward: &SiteForward{ExcludePath: []string{}, Path: []string{"/(\\w)/(\\w)/(\\w)/index.html"}, RePath: "/$1/$2/$3/index.html"},
			upath:   "/a/b/c/index.html",
			rpath:   "/a/b/c/index.html",
			re:      true,
		},
	}
	for index, test := range tests {
		rpath, re, err := test.forward.Rewrite(test.upath)
		if err != nil {
			t.Fatal(err)
		}
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
