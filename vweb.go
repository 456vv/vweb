package vweb

import(
	"sync/atomic"
)


//其它
const (
    Version                 string = "VWEB/v1.0.12"                     					// 版本号
    defaultDataBufioSize    int64 = 32*1024           										// 默认数据缓冲32MB
)

var DotFuncMap      = make(map[string]map[string]interface{})               				// 点函数映射


//响应完成设置
type atomicBool int32
func (T *atomicBool) isTrue() bool 	{ return atomic.LoadInt32((*int32)(T)) != 0 }
func (T *atomicBool) isFalse() bool	{ return atomic.LoadInt32((*int32)(T)) != 1 }
func (T *atomicBool) setTrue() bool	{ return !atomic.CompareAndSwapInt32((*int32)(T), 0, 1)}
func (T *atomicBool) setFalse() bool{ return !atomic.CompareAndSwapInt32((*int32)(T), 1, 0)}

//随机数的可用字符
const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._"