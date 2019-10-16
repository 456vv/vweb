package vweb
import (
    "path/filepath"
    "io/ioutil"
    "net/http"
    "bytes"
    "fmt"
    "bufio"
    "github.com/456vv/vmap/v2"
    "time"
    "os"
    "errors"
    "context"
)


type executer interface{
	execute(out *bytes.Buffer, in interface{}) error
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
	LibReadFunc			func(tmplName, libname string) ([]byte, error)						// 读取库
   	exec				executer
   	modeTime			time.Time
}

//ServeHTTP 服务HTTP
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request){
	T.ServeHTTPCtx(context.Background(), rw, req)
}
//ServeHTTPCtx 服务HTTP
//	ctx context.Context		  上下文
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTPCtx(ctx context.Context, rw http.ResponseWriter, req *http.Request){
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
        Exchange    : vmap.NewMap(),
    }
    dock = dock.WithContext(context.WithValue(ctx, "Dynamic", T))
	var body = new(bytes.Buffer)
	defer func(){
		dock.Free()
        if !dock.Writed {
	        body.WriteTo(rw)
	    }
	}()
	
	//执行模板内容
	err = T.Execute(body, (TemplateDoter)(dock))
    if err != nil {
    	webError(rw, err.Error())
        return
    }
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
func (T *ServerHandlerDynamic) Parse(bufr *bytes.Buffer) error {
	if T.PagePath == "" {
    	return errors.New("vweb: ServerHandlerDynamic.PagePath is not a valid path")
	}
	
    //文件首行
    firstLine, err := bufr.ReadBytes('\n')
    if err != nil || len(firstLine) == 0 {
    	return fmt.Errorf("vweb: Dynamic content is empty! Error: %s", err.Error())
    }
    drop := 0
	if firstLine[len(firstLine)-1] == '\n' {
		drop = 1
		if len(firstLine) > 1 && firstLine[len(firstLine)-2] == '\r' {
			drop = 2
		}
		firstLine = firstLine[:len(firstLine)-drop]
	}
    switch string(firstLine) {
        case "//template":
            var shdt = &serverHandlerDynamicTemplate{
            	rootPath	: T.RootPath,
               	pagePath	: T.PagePath,
            }
            shdt.libReadFunc = T.LibReadFunc
            err := shdt.parse(bufio.NewReader(bufr))
            if err != nil {
            	return err
            }
            T.exec = shdt
        case "//qlang":
        	shdq := &serverHandlerDynamicQlang{
            	rootPath	: T.RootPath,
               	pagePath	: T.PagePath,
        	}
            shdq.libReadFunc = T.LibReadFunc
            err := shdq.parse(bufio.NewReader(bufr))
            if err != nil {
            	return err
            }
            T.exec = shdq
        default:
    		return errors.New("vweb: The file type of the first line of the file is not recognized")
    }
    return nil
}

//Execute 执行模板
//	bufw *bytes.Buffer	模板返回数据
//	dock interface{}	与模板对接接口
//	error				错误
func (T *ServerHandlerDynamic) Execute(bufw *bytes.Buffer, dock interface{}) error {
	if T.exec == nil {
		return errors.New("vweb: Parse the template content first and then call the Execute")
	}
	return T.exec.execute(bufw, dock)
}


