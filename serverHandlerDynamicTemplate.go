package vweb
import(
    "bufio"
    //"net"
    "net/http"
    "text/template"
    "path/filepath"
    "fmt"
    "bytes"
    "io/ioutil"
    "strings"
    "errors"
    //"time"
  //  "context"
    "github.com/456vv/vmap/v2"
)



//标头-模本-处理动态页面文件
type shdtHeader struct{
    filePath        		[]string                              			                // 文件路径, map[文件名或别名]文件路径
    delimLeft,delimRight    string                                                          // 语法识别符
}

//openFile 打开文件内容
//  参：
//      rootPath  string    根目录
//      pagePath  string    文件路径
//  返：
//      map[string]string   内容，map[文件名]文件内容
//      error               错误，如果文件不能打开读取
func (h *shdtHeader) openFile(rootPath, pagePath  string) (map[string]string, error){
	var(
		dirPath		= filepath.Dir(pagePath)
		filePath	string
		fileBase	string
		pathFull	string
		fileContent = make(map[string]string)
	)
	for _, v := range h.filePath {
		if v[0] == '/' || v[0] == '\\' {
            filePath = filepath.Clean(v)
		}else{
			filePath = filepath.Join(dirPath, v)
			filePath = filepath.Clean(filePath)
        }
		pathFull = filepath.Join(rootPath, filePath)
		c, err := ioutil.ReadFile(pathFull)
		if err != nil {
			return nil, fmt.Errorf("vweb.shdtHeader.openFile: 动态嵌入模本文件读取失败(%s)", err.Error())
		}
		fileBase = filepath.Base(filePath)
		fileContent[fileBase] = string(c)
	}
	return fileContent, nil
}

//serverHandlerDynamicTemplate 模本-处理动态页面文件
type serverHandlerDynamicTemplate struct {
    rootPath, pagePath  string                                                              // 根目录, 页路径
    buffSize			int64																// 缓冲块大小
    site        		*Site																// 网站配置
    buf                 *bufio.Reader                                                       // 数据
}

//serveHTTP 服务HTTP
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *serverHandlerDynamicTemplate) serveHTTP(rw http.ResponseWriter, req *http.Request){
	var(
		err			error                               //错误
		h			shdtHeader                          //文件头
		c			string                              //内容
		
		contents	map[string]string                   //文件头嵌入的内容
        fileName    = filepath.Base(T.pagePath)      	//文件名
        t           *template.Template                  //模板
        td          *TemplateDot                        //模板点
        body        = new(bytes.Buffer)                 //缓冲区
	)

	//解析文件头和主体数据
    h, c, err = T.parse()
    if err != nil {
        goto Error
    }

    //打开文件头嵌入模板文件内容集
    contents, err = h.openFile(T.rootPath, T.pagePath)
    if err != nil {
    	goto Error
    }

    //模板文件内容载入Tmplate
    t = template.New(fileName)
    t.Delims(h.delimLeft, h.delimRight)
    t.Funcs(TemplateFuncMap)

    //解析主体内容
    c = T.format(h.delimLeft, h.delimRight, c)
    _, err = t.Parse(c)
    if err != nil {
    	goto Error
    }

    //解析子内容
    t, err = T.loadTmpl(h.delimLeft, h.delimRight, t, contents)
    if err != nil {
    	goto Error
    }


    //模板点
    td = &TemplateDot{
        R    	 	: req,
        W    		: rw,
        BuffSize	: T.buffSize,
        Site        : T.site,
        Exchange    : vmap.NewMap(),
        ec			: exitCall{},
    }
    
    //释放页面的函数
    defer td.ec.Free()

    //执行模板
    err = t.ExecuteTemplate(body, fileName, (TemplateDoter)(td))
   if err != nil && err.Error() != "Return" {
        goto Error
    }
    
    if !td.Writed {
        body.WriteTo(rw)
    }
    return

Error:
	//500 服务器遇到了意料不到的情况，不能完成客户的请求。
    http.Error(rw, fmt.Sprintf("Dynamic syntax is not supported! Error: %s", err.Error()), http.StatusInternalServerError)
}


//parse 解析模本
//	shdtHeader      模本标头
//	string          内容，动态语法
//	error           错误，如果语法无法解析
func (T *serverHandlerDynamicTemplate) parse() (shdtHeader, string, error) {
    var (
        line	[]byte
		h		= shdtHeader{
            delimLeft   : "{{",
            delimRight  : "}}",
        }
    )
    for {
        l, isPrefix, err :=  T.buf.ReadLine()
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
        	return shdtHeader{}, "", fmt.Errorf("vweb.serverHandlerDynamicTemplate.parse: 解析文件标头出错(%s)", string(line))
    	}
        key := string(bytes.Trim(line[:i], "\t "))
        i++ // skip colon
    	value := string(bytes.Trim(line[i:], "\t "))
    	if value == "" || value == "/" || value == "\\" {
            return shdtHeader{}, "", fmt.Errorf("vweb.serverHandlerDynamicTemplate.parse: 解析文件标头出错(%s)", string(line))
    	}
    	switch key {
		case "//file":
			h.filePath = append(h.filePath, value)
		case "//delimLeft":
			h.delimLeft = value
		case "//delimRight":
			h.delimRight = value
    	}
        line = []byte{}
    }
    b, err := ioutil.ReadAll(T.buf)
    if err != nil {
    	return shdtHeader{}, "", fmt.Errorf("vweb.serverHandlerDynamicTemplate.parse: 读取文件主体数据出错(%s)", err.Error())
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
        return t, errors.New("vweb.serverHandlerDynamicTemplate.loadTmpl: 父模板是 nil")
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
    var bytesBuffer = new(bytes.Buffer)
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
