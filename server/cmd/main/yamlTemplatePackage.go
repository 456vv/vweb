package main
	
import (
	"reflect"
	"text/template"
    "github.com/456vv/vweb/v2/builtin"
    "gopkg.in/yaml.v2"
)
	
func yamlTemplatePackage() map[string]template.FuncMap{
return map[string]template.FuncMap{
	"yaml":{
		"FutureLineWrap":yaml.FutureLineWrap,
		"Marshal":yaml.Marshal,
		"Unmarshal":yaml.Unmarshal,
		"UnmarshalStrict":yaml.UnmarshalStrict,
		"Decoder":func(a ...interface{}) (retn *yaml.Decoder) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewDecoder":yaml.NewDecoder,
		"Encoder":func(a ...interface{}) (retn *yaml.Encoder) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewEncoder":yaml.NewEncoder,
		"MapItem":func(a ...interface{}) (retn *yaml.MapItem) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"MapSlice":func(a ...interface{}) (retn *yaml.MapSlice) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"TypeError":func(a ...interface{}) (retn *yaml.TypeError) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
}
}