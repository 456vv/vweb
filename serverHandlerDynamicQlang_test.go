package vweb
import(
    "testing"
    "bytes"
    "path/filepath"
    //"errors"
    //"fmt"
)


func Test_serverHandlerDynamicQlang_parseText(t *testing.T) {
	
	var err error
	tests := []struct{
		context string
		name	string
		result	string
		dot		interface{}
	}{
		{context:`a=R.A;W.WriteString(a)`, name:"./test/qlang/main.ql", result:"a", dot:&struct{A string}{A:"a"}},
		{context:`import "bar";W.WriteString(bar.bb)`, name:"./test/wwwroot/qlang/main.ql", result:"1", dot:""},
		{context:`include "bar/main.ql";W.WriteString(bb);`, name:"/test/wwwroot/qlang/main.ql", result:"1", dot:""},
		{context:`include "/test/wwwroot/qlang/bar/main.ql";W.WriteString(bb);`, name:"/test/wwwroot/qlang/main.ql", result:"1", dot:""},
	}
	body := bytes.NewBuffer(nil)
	rootPath, err := filepath.Abs("./")
	if err != nil {
		t.Fatal(err)
	}
	for index, test := range tests{
		shdq := serverHandlerDynamicQlang{
			rootPath: rootPath,
			pagePath: test.name,
			//libReadFunc:func(tname, lname string) ([]byte, error){
			//	fmt.Println(tname, lname)
			//	return nil, errors.New("error")
			//},
		}
		body.Reset()
		
		err = shdq.parseText(test.context, test.name)
		if err != nil {
			t.Fatalf("%d %v", index, err)
		}
		
		err = shdq.execute(body, test.dot)
		if err != nil {
			t.Fatalf("%d %v", index, err)
		}
		
		result := string(body.Bytes())
		if result != test.result {
			t.Fatalf("%d %s != %s", index, result, test.result)
		}
		
		//测试是否可以多次调用execute
		body.Reset()
		err = shdq.execute(body, test.dot)
		if err != nil {
			t.Fatalf("%d %v", index, err)
		}
		
		result = string(body.Bytes())
		if result != test.result {
			t.Fatalf("%d %s != %s", index, result, test.result)
		}
	}
	
}
