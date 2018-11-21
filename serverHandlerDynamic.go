package vweb
import (
    "path/filepath"
    "io/ioutil"
    "net/http"
    "bytes"
    "fmt"
    "bufio"
)


//ServerHandlerDynamic 处理动态页面文件
type ServerHandlerDynamic struct {
    RootPath, PagePath  string                                                              // 根目录, 页路径
    BuffSize			int64																// 缓冲块大小
    Site        		*Site																// 网站配置
}

//ServeHTTP 服务HTTP
//	rw http.ResponseWriter    响应
//	req *http.Request         请求
func (T *ServerHandlerDynamic) ServeHTTP(rw http.ResponseWriter, req *http.Request){
    var(
        filePath    = filepath.Join(T.RootPath, T.PagePath)
    )
    content, err := ioutil.ReadFile(filePath)
    if err != nil {
    	//500 服务器遇到了意料不到的情况，不能完成客户的请求。
    	http.Error(rw, fmt.Sprintf("Failed to read the file! Error: %s", err.Error()), http.StatusInternalServerError)
        return
    }

    bytesBuffer := bytes.NewBuffer(content)
    //文件首行
    firstLine, err := bytesBuffer.ReadBytes('\n')
    if err != nil || len(firstLine) == 0 {
    	//500 服务器遇到了意料不到的情况，不能完成客户的请求。
        http.Error(rw, fmt.Sprintf("Dynamic content is empty! Error: %s", err.Error()), http.StatusInternalServerError)
        return
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
            shdt := &serverHandlerDynamicTemplate{
                rootPath    : T.RootPath,
                pagePath    : T.PagePath,
                buffSize	: T.BuffSize,
                site        : T.Site,
                buf         : bufio.NewReader(bytesBuffer),
            }
            shdt.serveHTTP(rw, req)
        default:
    	    //500 服务器遇到了意料不到的情况，不能完成客户的请求。
            http.Error(rw, "The first line of the file is not added to the file type is set to //template", http.StatusInternalServerError)
    }
}
