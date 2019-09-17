package vweb

import(
	"github.com/456vv/vmap/v2"
	"time"
)

type Swaper interface {
	New(key interface{}) *vmap.Map
	GetNewMap(key interface{}) *vmap.Map
	GetNewMaps(key ...interface{}) *vmap.Map
	Len() int
	Set(key, val interface{})
    SetExpired(key interface{}, d time.Duration)
    SetExpiredCall(key interface{}, d time.Duration, f func(interface{}))
    Has(key interface{}) bool
    Get(key interface{}) interface{}
    GetHas(key interface{}) (val interface{}, ok bool)
    GetOrDefault(key interface{}, def interface{}) interface{}
    Index(key ...interface{}) interface{}
    IndexHas(key ...interface{}) (interface{}, bool)
    Del(key interface{})
    Dels(keys []interface{})
    ReadAll() interface{}
    Reset()
    Copy(from *vmap.Map, over bool) 
    WriteTo(mm interface{}) (err error)
    ReadFrom(mm interface{}) error
    MarshalJSON() ([]byte, error)
    UnmarshalJSON(data []byte) error
   	String() string
}
