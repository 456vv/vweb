package vweb
import(
    "testing"
    "bufio"
    "log"
    "bytes"
    "text/template"
)

func Test_serverHandlerDynamicTemplate_parse(t *testing.T) {
	var tests = []struct{
		content 	[]byte
        err         bool
	}{
		{content:[]byte("//file=./2.tmpl\r\n"+
						"//file=./3.tmpl\r\n"+
						"//file=/5.tmpl\r\n"+
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"+
						"\r\n"+
						"1234567890"),
		},{err:true, content:[]byte("//file=./2.tmpl\r\n"+
						"//file:./3.tmpl\r\n"+//不正确
						"//file=/5.tmpl\r\n"+
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"+
						"\r\n"+
						"1234567890"),
		},{content:[]byte("//file=./2.tmpl\r\n"+
						"//File=./3.tmpl\r\n"+ //被忽略了
						"//file=/5.tmpl\r\n"+
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"+
						"\r\n"+
						"1234567890"),
		},{err:true, content:[]byte("//file=./2.tmpl\r\n"+
						"//file=./3.tmpl\r\n"+
						"//file=\r\n"+ //不正确
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"+
						"\r\n"+
						"1234567890"),
		},{content:[]byte("file=./2.tmpl\r\n"+//不正确
						"file=./3.tmpl\r\n"+//不正确
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"+
						"\r\n"+
						"1234567890"),
		},{content:[]byte("//file=./2.tmpl\r\n"+
						"//file=./3.tmpl\r\n"+
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"+
						"\r\n"),//无内容
		},{err:true, content:[]byte("//file=./2.tmpl\r\n"+
						"//file=./3.tmpl\r\n"+
						"//delimLeft={{\r\n"+
						"//delimRight=}}\r\n"),//不正确格式,无内容
		},
	}
	for _, v := range tests {
		shdt := serverHandlerDynamicTemplate{
		    rootPath: "./test/wwwroot",
		    pagePath: "/template/t.bw",
		}
		bytesBuffer := bytes.NewBuffer(v.content)
		bufioReader := bufio.NewReader(bytesBuffer)

		shdt.buf = bufioReader
		_, _, err := shdt.parse()
        if err != nil && !v.err {
        	t.Fatal(err)
        }
	}
}


func Test_shdtHeader_openFile(t *testing.T) {
    var(
        rootPath = "./test/wwwroot"
        pagePath = "/template/t.bw"
    )
    var tests = []struct{
        shdth   shdtHeader
        length  int
    }{
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "./3.tmpl", "/5.tmpl"},},length: 3},
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "./3.tmpl", "/6.tmpl"},},length: 0},// "/6.tmpl" 该文件不存在
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "/../3.tmpl", "/5.tmpl"},},length: 0},// "/../3.tmpl" 等于 "/3.tmpl" ，该文件不存在
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "./../5.tmpl", "/5.tmpl"},},length: 2},// "./../5.tmpl" 等于 "/5.tmpl"
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "../5.tmpl", "/5.tmpl"},},length: 2},// "../5.tmpl" 等于 "/5.tmpl"
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "../5.tmpl", "/"},},length: 0},// "/" 表示是根目录 "./test/wwwroot"，不是文件。
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "../5.tmpl", "../../"},},length: 0},// "../../" 表示是根目录 "./test/wwwroot"，因为不能跨越根目录。同时也不是一个有效的文件。
        {shdth:shdtHeader{filePath: []string{"./2.tmpl", "3.tmpl", "t.bw"},},length: 3},
    }
    for _, v := range tests {
        m, err :=v.shdth.openFile(rootPath, pagePath)
        if len(m) != v.length{
            log.Println(m, err)
        }
    }

}

func Test_serverHandlerDynamicTemplate_format(t *testing.T) {
    var tests = []struct{
        shdth   shdtHeader
        content string
        result  string
    }{
        {
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : "{{\r\n.\r\n}}1234{{\r\n.\r\n}}",
        result  : "{{.}}1234{{.}}",
        },{
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : "{{\r\n.\r\n}}1234\r\n{{.}}",
        result  : "{{.}}1234\r\n{{.}}",
        },{
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : "{{\r\n.\r\n}}1234{{.}}",
        result  : "{{.}}1234{{.}}",
        },{
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : "{{\r\n.\r\n}}\r\n1234\r\n{{.}}",
        result  : "{{.}}\r\n1234\r\n{{.}}",
        },{
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : "111{{\r\n.\r\n}}3333",
        result  : "111{{.}}3333",
        },

    }
    shdt := &serverHandlerDynamicTemplate{}
    for _, v := range tests {
        content := shdt.format(v.shdth.delimLeft, v.shdth.delimRight, v.content)
        if content != v.result {
            log.Println(content)
        }
    }
}


func Test_serverHandlerDynamicTemplate_loadTmpl(t *testing.T) {
    var tests = []struct{
        shdth   shdtHeader
        content map[string]string
        result  string
        err     bool
    }{
        {
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : map[string]string{"1.tmpl":"{{define \"1.tmpl\"}}1111111{{end}}", "2.tmpl":"{{define \"2.tmpl\"}}222222{{end}}",},
        },{
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : map[string]string{"1.tmpl":"{{define \"1.tmpl\"}}1111111{{end}}", "2.tmpl":"{{define \"2.tmpl\"}}222222",},
        err     : true,
        },{
        shdth   : shdtHeader{delimLeft:"{{", delimRight:"}}"},
        content : map[string]string{"1.tmpl":"{{define \"1.tmpl\"}}1111111{{end}}", "2.tmpl":"222222222",},
        },
    }
    shdt := serverHandlerDynamicTemplate{}
    for _, v := range tests {
        t := template.New("test")
        t.Delims(v.shdth.delimLeft, v.shdth.delimRight)
        t, err := shdt.loadTmpl(v.shdth.delimLeft, v.shdth.delimRight, t, v.content)

        if err != nil && !v.err {
            log.Printf("加载模板(%s)", v.content)
            log.Printf("加载模板失败，错误：%v\r\n", err)
        }
        if err != nil {continue}
        ts := t.Templates()
        if len(ts) != len(v.content) {
            log.Println(t)
        }
    }
}







