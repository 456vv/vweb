package vweb
import(
	"testing"
	"net/http"
)

func Test_ExtendDotFuncMap(t *testing.T){
	test := map[string]map[string]interface{}{
		"A":{"a1":"av1"},
		"B":{"b1":"bv1"},
	}
	ExtendDotFuncMap(test)
	for k, v := range test {
		funcMap, ok := DotFuncMap[k]
		if !ok {
			t.Fatalf("无法增加点函数(%s)", k)
		}
		for k1, v1 := range v {
			if funcMap[k1] != v1 {
				t.Fatalf("两个值不匹配%s != %s", funcMap[k1], v1)
			}
		}
	}
}

func Test_GenerateRandomId(t *testing.T){
	rnd := make([]byte, 10)
	err := GenerateRandomId(rnd)
	if err != nil {
		t.Fatal(err)
	}
}

func Test_equalDomain(t *testing.T) {
	tests := []struct{
		host	string
		match	string
		result	bool
	}{
		{host:"www.x.com",match:"*.x.com",result:true},
		{host:"www.x.com",match:"*.*.*",result:true},
		{host:"www.x.com:900",match:"*.*.*",result:false},
		{host:"www.x.com:900",match:"*.x.*",result:false},
		{host:"www.x.com:900",match:"*.*.*:900",result:true},
		{host:"www.x.com:900",match:"*.x.*:900",result:false},
		{host:"www.x.com:900:900",match:"*.*.*:900",result:false},
		{host:"www.x.com:900:900",match:"*.*.com:900",result:false},
		{host:"111.222.444.555:900",match:"*.*.*.*:900",result:true},
		{host:"111.222.444.555:900",match:"*.*.*.555:900",result:true},
		{host:"111.222.444.555:900",match:"*.*.*.444:900",result:false},
	}
	for _,test := range tests {
		if equalDomain(test.host, test.match) != test.result {
			t.Fatalf("%s != %s", test.host, test.match)
		}
	}
}


func Test_PagePath(t *testing.T){
    var tests = []struct{
        root, path string
        index []string
        result string
    }{
        {
        root:	"./test/wwwroot/",
        path:   "http://www.abc.com/abc/1.txt",
        index:  []string{"index.html", "default.html", "index.bw"},
        result: "/abc/1.txt",
        },
        {
        root:	"./test/wwwroot/",
        path:   "http://www.123.com/../abc/",
        index:  []string{"index.html", "default.html", "index.bw"},
        result: "/abc/index.html",
        },
        {
        root:	"./test/wwwroot/",
        path:   "http://www.456.com/../abc/e.html",
        index:  []string{"index.html", "default.html", "index.bw"},
        result: "",
        },
    }


    for _, test := range tests {
        req, err :=  http.NewRequest("GET", test.path, nil)
        if err != nil {
           t.Fatalf("请求配置失败，错误：%v", err)
        }
        _, ps, err := PagePath(test.root, req.URL.Path, test.index)
        if err != nil && test.result != "" {
            t.Fatal(err)
        }else if test.result != ps {
        	t.Fatalf("\r\n返回结果：%v \r\n预测结果：%v\r\n", ps, test.result)
        }
    }
}

