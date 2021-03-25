package vweb

import (
	"os"
	"io"
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
)

//标头-模本-处理动态页面文件
type TemplateHeader struct{
    File        		[]string                              			                // 文件路径, map[文件名或别名]文件路径
    DelimLeft,DelimRight	string                                                      // 语法识别符
}
//打开文件内容
//	rootPath  string    	根目录
//	pagePath  string		页面路径
//	map[string][]byte   内容，map[文件名]文件内容
//	error               错误，如果文件不能打开读取
func (T *TemplateHeader) OpenFile(rootPath, pagePath string) (map[string]string, error){
	var(
		dirPath		= filepath.Dir(pagePath)
		filePath	string
		fileBase	string
		fileContent = make(map[string]string)
		c			[]byte
		err 		error
	)
	for _, v := range T.File {
		if v[0] == '/' || v[0] == '\\' {
            filePath = filepath.Clean(v)
		}else{
			filePath = filepath.Join(dirPath, v)
        }
       	filePath = filepath.Join(rootPath, filePath)
		c, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("vweb: Dynamically embedded template file read failed(%s)", err.Error())
		}
		fileBase = filepath.Base(filePath)
		fileContent[fileBase] = string(c)
	}
	return fileContent, nil
}

//解析模本,头，内容
//	TemplateHeader      模本标头
//	[]byte          内容，动态语法
//	error           错误，如果语法无法解析
func TemplateSeparation(r io.Reader) (*TemplateHeader, []byte, error) {
	
	buf, ok := r.(*bufio.Reader)
	if !ok {
		buf = bufio.NewReader(r)
	}

	
    var (
        line	[]byte
		h		= &TemplateHeader{}
    )
    
    
    for {
        l, isPrefix, err :=  buf.ReadLine()
        if err != nil {
            return nil, nil, err
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
        	return nil, nil, fmt.Errorf("vweb: Error parsing file header(%s)", string(line))
    	}
    	
        key := string(bytes.Trim(line[:i], "\t "))
        i++ // 跳过=符号
    	value := string(bytes.Trim(line[i:], "\t "))
    	if value == "" || value == "/" || value == "\\" {
            return nil, nil, fmt.Errorf("vweb: Error parsing file header(%s)", string(line))
    	}
    	
    	switch key {
		case "//file":
			h.File = append(h.File, value)
		case "//delimLeft":
			h.DelimLeft = value
		case "//delimRight":
			h.DelimRight = value
    	}
        line = line[:0]
    }
    
    b, err := io.ReadAll(buf)
    if err != nil {
    	return nil, nil, fmt.Errorf("vweb: Error reading file body data(%s)", err.Error())
    }
    return h, b, nil
}
