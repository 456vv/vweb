package main

import (
	"reflect"
	"text/template"
    "github.com/456vv/vweb/v2/builtin"
    "github.com/pelletier/go-toml"
)

func tomlTemplatePackage() map[string]template.FuncMap{
return map[string]template.FuncMap{
	"toml":{
		"OrderAlphabetical":toml.OrderAlphabetical,
		"OrderPreserve":toml.OrderPreserve,
		"Marshal":toml.Marshal,
		"Unmarshal":toml.Unmarshal,
		"Decoder":func(a ...interface{}) (retn *toml.Decoder) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewDecoder":toml.NewDecoder,
		"Encoder":func(a ...interface{}) (retn *toml.Encoder) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewEncoder":toml.NewEncoder,
		"Position":func(a ...interface{}) (retn *toml.Position) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SetOptions":func(a ...interface{}) (retn *toml.SetOptions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Tree":func(a ...interface{}) (retn *toml.Tree) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Load":toml.Load,
		"LoadBytes":toml.LoadBytes,
		"LoadFile":toml.LoadFile,
		"LoadReader":toml.LoadReader,
		"TreeFromMap":toml.TreeFromMap,
	},
}
}