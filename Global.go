package vweb

type Globaler interface {
    Set(key, val interface{})                                                                   // 设置
    Has(key interface{}) bool                                                                   // 检查
    Get(key interface{}) interface{}                                                            // 读取
    Del(key interface{})                                                                        // 删除
    Reset()                                                                                     // 重置
}

