package vweb

import (
	"testing"
    "io/ioutil"
    "os"
    "bytes"
)

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