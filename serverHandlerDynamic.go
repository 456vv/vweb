package vweb
import (
    "path/filepath"
    "io/ioutil"
    "net/http"
    "bytes"
    "fmt"
    "bufio"
    "time"
    "os"
    "errors"
    "context"
    "runtime"
    "github.com/456vv/verror"
    "io"
)


type DynamicTemplate interface{
    ParseFile(path string) error																					// 解析文件
    ParseText(content, name string) error																			// 解析文本
    SetPath(rootPath, pagePath string)																				// 设置路径
    Parse(r *bufio.Reader) (err error)																				// 解析
    Execute(out *bytes.Buffer, dot interface{}) error																// 执行
}

//web错误调用
func webError(rw http.ResponseWriter, v ...interface{}) {
   	//500 服务器遇到了意料不到的情况，不能完成客户的请求。
    http.Error(rw, fmt.Sprint(v...), http.StatusInternalServerError)
}

//ServerHandlerDynamic 处理动态页面文件
type ServerHandlerDynamic struct {
	//必须的
	RootPath			string																// 根目录
    PagePath  			string																// 主模板文件路径

    //可选的
    BuffSize			int64																// 缓冲块大小
    Site        		*Site																// 网站配置
	Context				context.Context														// 上下文
	Plus				map[string]DynamicTemplate											// 支持更动态文件类型
   	exec				DynamicTemplate
   	modeTime			time.Time
}

//ServeHTTP 服务HTTP
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request){

	if T.PagePath == "" {
		T.PagePath = req.URL.Path
	}
	var filePath = filepath.Join(T.RootPath, T.PagePath)

	osFile, err := os.Open(filePath)
	if err != nil {
	    webError(rw, fmt.Sprintf("Failed to read the file! Error: %s", err.Error()))
	    return
	}
	defer osFile.Close()

	//记录文件修改时间，用于缓存文件
	osFileInfo, err := osFile.Stat()
	if err != nil {
		T.exec = nil
	}else{
		modeTime := osFileInfo.ModTime()
		if !modeTime.Equal(T.modeTime) {
			T.exec = nil
		}
		T.modeTime = modeTime
	}

	if T.exec == nil {
	    var content, err = ioutil.ReadAll(osFile)
	    if err != nil {
	    	webError(rw, fmt.Sprintf("Failed to read the file! Error: %s", err.Error()))
	        return
	    }

	    //解析模板内容
		err = T.Parse(bytes.NewBuffer(content))
	    if err != nil {
	    	webError(rw, err.Error())
	        return
	    }
	}

    //模板点
    var dock = &TemplateDot{
        R    	 	: req,
        W    		: rw,
        BuffSize	: T.BuffSize,
        Site        : T.Site,
    }

    ctx := T.Context
    if ctx == nil {
    	ctx = req.Context()
    }
    dock.WithContext(context.WithValue(ctx, "Dynamic", T))
	var body = new(bytes.Buffer)
	defer func(){
		dock.Free()
		if err != nil {
			if !dock.Writed {
				webError(rw, err.Error())
				return
			}
			io.WriteString(rw, err.Error())
			fmt.Println(err.Error())
			return
		}
		if !dock.Writed {
			body.WriteTo(rw)
		}
	}()

	//执行模板内容
	err = T.Execute(body, (TemplateDoter)(dock))
}

//ParseText 解析模板
//	content, name string	模板内容，模板名称
//	error					错误
func (T *ServerHandlerDynamic) ParseText(content, name string) error {
	T.PagePath = name
	r := bytes.NewBufferString(content)
	return T.Parse(r)
}

//ParseFile 解析模板
//	path string			模板文件路径，如果为空，默认使用RootPath,PagePath字段
//	error				错误
func (T *ServerHandlerDynamic) ParseFile(path string) error {

	if path == "" {
		path = filepath.Join(T.RootPath, T.PagePath)
	}else if !filepath.IsAbs(path) {
		T.PagePath = path
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	r := bytes.NewBuffer(b)
	return T.Parse(r)
}

//Parse 解析模板
//	bufr *bytes.Reader	模板内容
//	error				错误
func (T *ServerHandlerDynamic) Parse(bufr *bytes.Buffer) (err error) {
	if T.PagePath == "" {
    	return verror.TrackError("vweb: ServerHandlerDynamic.PagePath is not a valid path")
	}

    //文件首行
    firstLine, err := bufr.ReadBytes('\n')
    if err != nil || len(firstLine) == 0 {
    	return verror.TrackErrorf("vweb: Dynamic content is empty! Error: %s", err.Error())
    }
    drop := 0
	if firstLine[len(firstLine)-1] == '\n' {
		drop = 1
		if len(firstLine) > 1 && firstLine[len(firstLine)-2] == '\r' {
			drop = 2
		}
		firstLine = firstLine[:len(firstLine)-drop]
	}

	dynmicType := string(firstLine)
    switch dynmicType {
    case "//template":
        var shdt = &serverHandlerDynamicTemplate{}
		shdt.SetPath(T.RootPath, T.PagePath)
        err = shdt.Parse(bufio.NewReader(bufr))
        if err != nil {
        	return
        }
        T.exec = shdt
    default:
    	if T.Plus == nil || len(dynmicType) < 3 {
    		return errors.New("vweb: The file type of the first line of the file is not recognized")
    	}
		if shdt, ok := T.Plus[dynmicType[2:]]; ok {
			shdt.SetPath(T.RootPath, T.PagePath)
			err = shdt.Parse(bufio.NewReader(bufr))
	        if err != nil {
	        	return
	        }
	       T.exec = shdt
		}
    }
    return
}

//Execute 执行模板
//	bufw *bytes.Buffer	模板返回数据
//	dock interface{}	与模板对接接口
//	error				错误
func (T *ServerHandlerDynamic) Execute(bufw *bytes.Buffer, dock interface{}) (err error) {
	if T.exec == nil {
		return errors.New("vweb: Parse the template content first and then call the Execute")
	}
	defer func (){
		if e := recover(); e != nil{
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			err = fmt.Errorf("vweb: Dynamic code execute error。%v\n%s", e, buf)
		}
	}()

	return T.exec.Execute(bufw, dock)
}


