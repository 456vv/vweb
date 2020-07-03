package vweb

import (
	"regexp"
	"github.com/456vv/verror"
	"strings"
	"strconv"
)

type Forward struct {
	Path		[]string																	// 多种路径匹配
	ExcludePath []string																	// 排除多种路径匹配
	RePath		string																		// 重写路径
}

func (T *Forward) Rewrite(upath string) (rpath string, rewrited bool, err error) {

    //满足路径
    var regExp *regexp.Regexp

    //排除路径
	for _, ep := range T.ExcludePath {
    	//非正则
        if ep == upath {
        	rpath = upath
        	return
        }

    	//正则
    	regExp, err = regexp.Compile(ep)
        if err != nil {
			err = verror.TrackErrorf("vweb: 是错误正则re2(%s)", ep)
			return
        }

        _, complete := regExp.LiteralPrefix()
        if !complete {
        	regExp.Longest()
        	if regExp.MatchString(upath) {
        		rpath = upath
        		return
        	}
        }
    }

	//包含路径
	regExp = nil
    for _, ep := range T.Path {
    	//非正则
        if rewrited = (ep == upath); rewrited {
        	break
        }
    	//正则
    	regExp, err = regexp.Compile(ep)
        if err != nil {
			err = verror.TrackErrorf("vweb: 是错误正则re2(%s)", ep)
			return
        }
        _, complete := regExp.LiteralPrefix()
        if !complete {
        	regExp.Longest()
        	rewrited = regExp.MatchString(upath)
        	if rewrited {
        		break
        	}
        }
        regExp = nil
    }

    //修改路径地址
    if rewrited {
    	if regExp != nil {
			var findAllSubmatch [][]string = regExp.FindAllStringSubmatch(upath, 1)
			if len(findAllSubmatch) != 0 {
    			rpath = T.RePath
				submatch := findAllSubmatch[0]	//使用第一个匹配
				for i, match := range submatch {
					rpath = strings.Replace(rpath, "$"+strconv.Itoa(i), match, -1)
				}
			}
    	}else{
    		rpath = strings.Replace(T.RePath, "$0", upath, -1)
    	}
    	return rpath, true, nil
    }
	return upath, false, nil
}

