package vweb
import(
    "bufio"
    "text/template"
    "path/filepath"
    "fmt"
    "bytes"
    "io/ioutil"
    "strings"
    "errors"
    "os"
    "reflect"
    "sync"
    "context"
)

var errTemplateNotParse =  errors.New("vweb: The template has not been parsed yet!")

//标头-模本-处理动态页面文件
type shdtHeader struct{
    filePath        		[]string                              			                // 文件路径, map[文件名或别名]文件路径
    delimLeft,delimRight    string                                                          // 语法识别符
    rFile					func(file string) ([]byte, error)								// 打开文件
}

//openFile 打开文件内容
//	dirPath  string    	目录
//	map[string]string   内容，map[文件名]文件内容
//	error               错误，如果文件不能打开读取
func (h *shdtHeader) openIncludeFile(rootPath, pagePath string) (map[string]string, error){
	var(
		dirPath		= filepath.Dir(pagePath)
		filePath	string
		fileBase	string
		fileContent = make(map[string]string)
		c			[]byte
		err 		error
	)
	for _, v := range h.filePath {
		if h.rFile != nil {
			c, err = h.rFile(v)
		}else{
			if v[0] == '/' || v[0] == '\\' {
	            filePath = filepath.Clean(v)
			}else{
				filePath = filepath.Join(dirPath, v)
				filePath = filepath.Clean(filePath)
	        }
	       	filePath = filepath.Join(rootPath, filePath)
			c, err = ioutil.ReadFile(filePath)
		}
		if err != nil {
			return nil, fmt.Errorf("vweb: Dynamically embedded template file read failed(%s)", err.Error())
		}
		fileBase = filepath.Base(filePath)
		fileContent[fileBase] = string(c)
	}
	return fileContent, nil
}


//serverHandlerDynamicTemplate 模本-处理动态页面文件
type serverHandlerDynamicTemplate struct {
	rootPath			string																// 文件目录
	pagePath			string																// 文件名称
	libReadFunc			func(tmplName, libName string) ([]byte, error)
 	
 	fileName			string
	t 					*template.Template
}
func (T *serverHandlerDynamicTemplate) parseText(content, name string) error {
	T.fileName = name
	r := bufio.NewReader(strings.NewReader(content))
	return T.parse(r)
}
func (T *serverHandlerDynamicTemplate) parseFile(path string) error {
	//文件名
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	r := bufio.NewReader(bytes.NewBuffer(b))
	T.fileName = filepath.Base(path)
	return T.parse(r)
}
func (T *serverHandlerDynamicTemplate) parse(r *bufio.Reader) (err error) {
	var(
		h			shdtHeader                          //文件头
		c			string                              //内容
		
		libs		map[string]string                   //文件头嵌入的内容
	)
	
	//解析文件头和主体数据
    h, c, err = T.separation(r)
    if err != nil {
        return
    }
    
    //打开文件头嵌入模板文件内容集
    if T.libReadFunc != nil {
	    h.rFile=func(libName string)([]byte, error){
	    	return T.libReadFunc(T.fileName, libName)
	    }
    }
    libs, err = h.openIncludeFile(T.rootPath, T.pagePath)
    if err != nil {
        return
    }

    //模板文件内容载入Tmplate
    t := template.New(T.fileName)
    t.Delims(h.delimLeft, h.delimRight)
    t.Funcs(TemplateFuncMap)

    //解析主体内容
    c = T.format(h.delimLeft, h.delimRight, c)
    _, err = t.Parse(c)
    if err != nil {
        return
    }
    
    //解析子内容
    T.t, err = T.loadTmpl(h.delimLeft, h.delimRight, t, libs)
    return
}

//Execute 执行模板
//	out *bytes.Buffer	模板中返回的内容
//	in interface{}		模板中调用的接口
//	error				执行失败
func (T *serverHandlerDynamicTemplate) execute(out *bytes.Buffer, in interface{}) error {
	if T.t == nil {
		return errTemplateNotParse
	}
    //执行模板
    if tdot, ok := in.(Contexter); ok {
    	tdot.WithContext(context.WithValue(tdot.Context(), "Template", &serverHandlerDynamicTemplateExtend{t:T.t}))
    }
	return T.t.ExecuteTemplate(out, T.fileName, in)
}

//separation 解析模本,头，内容
//	shdtHeader      模本标头
//	string          内容，动态语法
//	error           错误，如果语法无法解析
func (T *serverHandlerDynamicTemplate) separation(buf *bufio.Reader) (shdtHeader, string, error) {
    var (
        line	[]byte
		h		= shdtHeader{
            delimLeft   : "{{",
            delimRight  : "}}",
        }
    )
    for {
        l, isPrefix, err :=  buf.ReadLine()
        if err != nil {
            return shdtHeader{}, "", err
        }
        //空行后面是内容
        if len(l) == 0 {
            break
        }
        line = append(line, l...)
        if isPrefix {
            continue
        }
        //清除字符前面 //
        i := bytes.IndexByte(line, '=')
        if i < 0 {
        	return shdtHeader{}, "", fmt.Errorf("vweb: Error parsing file header(%s)", string(line))
    	}
        key := string(bytes.Trim(line[:i], "\t "))
        i++ // skip colon
    	value := string(bytes.Trim(line[i:], "\t "))
    	if value == "" || value == "/" || value == "\\" {
            return shdtHeader{}, "", fmt.Errorf("vweb: Error parsing file header(%s)", string(line))
    	}
    	switch key {
		case "//file":
			h.filePath = append(h.filePath, value)
		case "//delimLeft":
			h.delimLeft = value
		case "//delimRight":
			h.delimRight = value
    	}
        line = line[:0]
    }
    b, err := ioutil.ReadAll(buf)
    if err != nil {
    	return shdtHeader{}, "", fmt.Errorf("vweb: Error reading file body data(%s)", err.Error())
    }
    return h, string(b), nil
}

//loadTmpl 模本载入
//	delimLeft, delimRight string  语法识别符
//	t *template.Template  模本对象
//	f map[string]string   模本内容，map[文件名]文件内容，这是文件头嵌入的模本文件内容。
//	*template.Template    模本对象
//	error                 错误
func (T *serverHandlerDynamicTemplate) loadTmpl(delimLeft, delimRight string, t *template.Template, f map[string]string) (*template.Template, error) {
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
//	delimLeft string    语法识别符(左)
//	delimRight string   语法识别符（右）
//	c string            语法内容
//	string              整理后的语法
func (T *serverHandlerDynamicTemplate) format(delimLeft, delimRight, c string) string {
    var bytesBuffer strings.Builder
    for _, line := range []string{"\r\n", "\n", "\r"} {
        if strings.Contains(c, line) {
            var syntax bool
            clines  := strings.Split(c, line)
            clinesL := len(clines)-1
            for k, cline := range clines {
                clineTrim := strings.Trim(cline, " \t")
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
                    if clineTrim == "" || strings.HasPrefix(clineTrim, "//") {continue}
                    cline   = fmt.Sprint(delimLeft, cline, delimRight)
                }else{
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
	wait	sync.WaitGroup
}
func (T *part) Args(i int) reflect.Value {
	if len(T.input) >= i {
		return reflect.Value{}
	}
	return T.input[i]
}
func (T *part) Result(out ...reflect.Value){
	T.output = append(T.output, out...)
	T.wait.Done()
}

//这是个额外扩展，由于模板不能实现函数创建，只能在这里构造一个支持创建函数。
//需要结合 reflect.MakeFunc(typ Type, fn func(args []Value) (results []Value)) Value 来使用
type serverHandlerDynamicTemplateExtend struct{
	t *template.Template
}

//NewFunc 构建一个新的函数，仅限在template中使用
//	func([]reflect.Value) []reflect.Value)	新的函数
func (T *serverHandlerDynamicTemplateExtend) NewFunc(name string) (f func([]reflect.Value) []reflect.Value, err error) {
	if T.t == nil {
		return nil, errTemplateNotParse
	}
	if T.t.Lookup(name) == nil {
		return nil, fmt.Errorf("vweb: Template content not found {{define \"%s\"}} ... {{end}} , Calling this method does not support", name)
	}
	return func(in []reflect.Value) []reflect.Value {
		p := &part{input: in,}
		p.wait.Add(1)
		err := T.t.ExecuteTemplate(ioutil.Discard, name, p)
		if err != nil {
			panic(err)
		}
		p.wait.Wait()
		return p.output
	}, nil
}