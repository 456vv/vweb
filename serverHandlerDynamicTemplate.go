package vweb
import(
    "text/template"
    "path/filepath"
    "fmt"
    "strings"
    "errors"
    "os"
    "io"
    "reflect"
    "context"
)

var errTemplateNotParse =  errors.New("vweb: The template has not been parsed yet!")

//serverHandlerDynamicTemplate 模本-处理动态页面文件
type serverHandlerDynamicTemplate struct {
	P					*ServerHandlerDynamic
	rootPath			string																// 文件目录
	pagePath			string																// 文件名称
 	
 	fileName			string
	t 					*template.Template
}

func (T *serverHandlerDynamicTemplate) ParseText(name, content string) error {
	T.fileName = name
	r := strings.NewReader(content)
	return T.Parse(r)
}

func (T *serverHandlerDynamicTemplate) ParseFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	
	defer file.Close()
	T.fileName = filepath.Base(path)
	return T.Parse(file)
}

func (T *serverHandlerDynamicTemplate) SetPath(root, page string) {
	T.rootPath = root
	T.pagePath = page
    if T.fileName == "" {
    	T.fileName = filepath.Base(T.pagePath)
    }
}

func (T *serverHandlerDynamicTemplate) Parse(r io.Reader) error {
	//解析文件头和主体数据
    h, cb, err := TemplateSeparation(r)
    if err != nil {
        return err
    }
	
    libs, err := h.OpenFile(T.rootPath, T.pagePath)
    if err != nil {
        return err
    }
    //解析主体内容
    cs := T.format(h.DelimLeft, h.DelimRight, string(cb))

    //模板文件内容载入Tmplate
    t := template.New(T.fileName)
    t.Delims(h.DelimLeft, h.DelimRight)
    t.Funcs(TemplateFunc)
    _, err = t.Parse(cs)
    if err != nil {
        return err
    }
    
    //解析子内容
    T.t, err = T.loadTmpl(t, h.DelimLeft, h.DelimRight, libs)
    return err
}

func (T *serverHandlerDynamicTemplate) Execute(out io.Writer, in interface{}) error {
	if T.t == nil {
		return errTemplateNotParse
	}
    //执行模板
    if tdot, ok := in.(DotContexter); ok {
    	tdot.WithContext(context.WithValue(tdot.Context(), "Template", &ServerHandlerDynamicTemplateExtend{Template:T.t}))
    }
	return T.t.ExecuteTemplate(out, T.fileName, in)
}

//loadTmpl 模本载入
func (T *serverHandlerDynamicTemplate) loadTmpl(t *template.Template, delimLeft, delimRight string, f map[string]string) (*template.Template, error) {
    var tmpl *template.Template
    if t == nil {
        return t, errors.New("vweb: The parent template is nil")
    }
    for k, v := range f {
        tmpl = t.New(k)
        v = T.format(delimLeft, delimRight, v)
        _, err := tmpl.Parse(v)
        if err != nil {
            return nil, err
        }
    }
    return t, nil
}

//format 语法整合
func (T *serverHandlerDynamicTemplate) format(delimLeft, delimRight, c string) string {
	if delimLeft == "" {
		delimLeft = "{{"
	}
	if delimRight == "" {
		delimRight = "}}"
	}
    var bytesBuffer strings.Builder
    for _, line := range []string{"\r\n", "\n", "\r"} {
        if strings.Contains(c, line) {
            var syntax bool
            clines  := strings.Split(c, line)
            clinesL := len(clines)-1
            for k, cline := range clines {
                clineTrim 	:= strings.Trim(cline, " \t")
                leftHas     := strings.HasSuffix(clineTrim, delimLeft)
                rightHas    := strings.HasPrefix(clineTrim, delimRight)
                switch true {
                case  leftHas && rightHas:
                    //格式：\r\n    }}abcx{{
                    clineTrim   = strings.TrimPrefix(clineTrim, delimRight)
                    clineTrim   = strings.TrimSuffix(clineTrim, delimLeft)
                    //写入内容，非语法
                    bytesBuffer.WriteString(clineTrim)
                    syntax = true
                    continue
                case leftHas:
                    //格式：abcx{{
                    cline   = strings.TrimRight(cline, " \t")
                    cline   = strings.TrimSuffix(cline, delimLeft)
                    //写入内容，非语法
                    bytesBuffer.WriteString(cline)
                    syntax = true
                    continue
                case rightHas:
                    //格式：}}12345
                    cline   = strings.TrimLeft(cline, " \t")
                    cline   = strings.TrimPrefix(cline, delimRight)
                    syntax = false
                }
				
                if syntax {
                    if clineTrim == "" || strings.HasPrefix(clineTrim, "//") {
                    	//空行跳过
                    	//注册跳过
                    	continue
                    }
                    //格式：if eq 1 1
                    cline   = fmt.Sprint(delimLeft, cline, delimRight)
                }else{
                	//文本行
                    if clinesL != k  {
                        cline   = fmt.Sprint(cline, line)
                    }
                }
                bytesBuffer.WriteString(cline)
            }
            //退出，已经确定换行符，不再继续
            break
        }
    }

    if bytesBuffer.Len() != 0 {
        return bytesBuffer.String()
    }else{
        return c
    }
}

type part struct{
	input 	[]reflect.Value
	output 	[]reflect.Value
}

func (T *part) Args(i int) interface{} {
	if i == -1 {
		var ret []interface{}
		for _, in := range T.input {
			ret = append(ret, typeSelect(in))
		}
		return ret
	}
	if len(T.input) > i {
		v := T.input[i]
		return typeSelect(v)
	}
	return nil
}

func (T *part) Result(out ...interface{}){
	for _, arg := range out {
		T.output = append(T.output, reflect.ValueOf(arg))
	}
}

//这是个额外扩展，由于模板不能实现函数创建，只能在这里构造一个支持创建函数。
//在创建的函数内部，需要使用 Args 方法读取参数，使用 Result 方法返回结果。
//仅限用于template模板，自定义模板不支持
type ServerHandlerDynamicTemplateExtend struct{
	*template.Template
}

//NewFunc 构建一个新的函数，仅限在template中使用
//	func([]reflect.Value) []reflect.Value)	新的函数
func (T *ServerHandlerDynamicTemplateExtend) NewFunc(name string) (f func([]reflect.Value) []reflect.Value, err error) {
	if T.Template == nil {
		return nil, errTemplateNotParse
	}
	if T.Template.Lookup(name) == nil {
		return nil, fmt.Errorf("vweb: Template content not found {{define \"%s\"}} ... {{end}} , Calling this method does not support", name)
	}
	return func(in []reflect.Value) []reflect.Value {
		p := &part{input: in,}
		err := T.Template.ExecuteTemplate(io.Discard, name, p)
		if err != nil {
			panic(err)
		}
		return p.output
	}, nil
}

//Call 执行函数
//	f func([]reflect.Value) []reflect.Value	由NewFunc创建的函数
//	args ...interface{}						可变参数
//	[]interface{}							返回结果
func (T *ServerHandlerDynamicTemplateExtend) Call(f func([]reflect.Value) []reflect.Value, args ...interface{}) []interface{} {
	var(
		inv []reflect.Value
		ret	[]interface{}
	)
	for _, arg := range args {
		inv = append(inv, reflect.ValueOf(arg))
	}
	ef := ExecCall{}
	if err := ef.Func(f, inv); err != nil {
		panic(err)
	}
	//NewFunc 执行后返回是[]reflect.Value
	for _, result := range ef.Exec() {
		//已100%确认变量的类型为reflect.Value
		//1，查看func (T *serverHandlerDynamicTemplateExtend) NewFunc(name string) (f func([]reflect.Value) []reflect.Value, err error)
		//2，查看func (T *ExecFunc) Exec() (ret []interface{})
		for _, rv := range result.([]reflect.Value) {
			ret = append(ret, typeSelect(rv))
		}
	}
	return ret
}

//ExecuteTemplate 调用模板
//	out io.Writer	模板内容返回内容写入到out
//	name string		模板名称
//	in interface{}	模板点，带外模板的内容
//	error			执行语法错误
func (T *ServerHandlerDynamicTemplateExtend) ExecuteTemplate(out io.Writer, name string, in interface{}) error {
	if T.Template == nil {
		return errTemplateNotParse
	}
 	if T.Template.Lookup(name) == nil {
		return fmt.Errorf("vweb: Template content not found {{define \"%s\"}} ... {{end}} , Calling this method does not support", name)
	}
   //执行模板
    if tdot, ok := in.(DotContexter); ok {
    	tdot.WithContext(context.WithValue(tdot.Context(), "Template", T))
    }
	return T.Template.ExecuteTemplate(out, name, in)
}
