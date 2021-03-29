package vweb
	
import(
	"testing"
	"bytes"
)



func Test_TemplateSeparation1(t *testing.T) {
	buf := bytes.NewBufferString("//file=a1.v\n//file=b1.v\n\r\n111111")
	fh, content, err := TemplateSeparation(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(fh.File) != 2 {
		t.Fatalf("引入文件长度不正确")
	}
	if len(content) != 6 {
		t.Fatalf("内容长度不正确")
	}
}
func Test_TemplateSeparation2(t *testing.T) {
	buf := bytes.NewBufferString("\r\n111111")
	fh, content, err := TemplateSeparation(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(fh.File) != 0 {
		t.Fatalf("引入文件长度不正确")
	}
	if len(content) != 6 {
		t.Fatalf("内容长度不正确")
	}
}

func Test_TemplateHeader_OpenFile(t *testing.T){
	th := &TemplateHeader{
		File:[]string{
			"a1.v",
			"b1.v",
		},
	}
	fbs, err := th.OpenFile("./test/wwwroot", "/template/index.v")
	if err != nil {
		t.Fatal(err)
	}
	if len(fbs) != 2 {
		t.Fatal("读取模板头文件数量不正确")
	}
}