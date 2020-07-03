package server

import (
	"testing"
    "io/ioutil"
    "os"
    "bytes"
)

func Test_ConfigSiteForwards(t *testing.T){
	tests := []struct{
		forward 	*ConfigSiteForwards
		upath		string
		rpath		string
		re			bool
	}{
		{
		forward: &ConfigSiteForwards{ExcludePath:[]string{"/"}},
		upath: "/",
		rpath: "/",
		re: false,
		},{
		forward: &ConfigSiteForwards{ExcludePath:[]string{"/A/B/C/index.html"}, Path:[]string{"/a/b/c/index.html"}, RePath:"/A/B/C/index.html"},
		upath: "/a/b/c/index.html",
		rpath: "/A/B/C/index.html",
		re: true,
		},{
		forward: &ConfigSiteForwards{ExcludePath:[]string{}, Path:[]string{"/(\\w)/(\\w)/(\\w)/index.html"}, RePath:"/$1/$2/$3/index.html"},
		upath: "/a/b/c/index.html",
		rpath: "/a/b/c/index.html",
		re: true,
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


func Test_ConfigFileParse(t *testing.T){
    conf := &Config{}
    err := ConfigFileParse(conf, "./test/config.json")
    if(err != nil){
        t.Fatal(err)
    }
}

func Test_ConfigDataParse(t *testing.T){
    osFile, err := os.Open("./test/config.json")
    if err != nil {
    	t.Fatal(err)
    }

    b, err := ioutil.ReadAll(osFile)
    if err != nil {
    	t.Fatal(err)
    }

    buf := bytes.NewBuffer(b)
    conf    := &Config{}
    err = ConfigDataParse(conf, buf)
    if(err != nil){
        t.Fatal(err)
    }
}