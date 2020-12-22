package main
	
import (
	"reflect"
	"text/template"
    "github.com/goplusjs/reflectx"
)
	
func reflectxTemplatePackage() map[string]template.FuncMap{
return map[string]template.FuncMap{
	"reflectx":{
		"Field":reflectx.Field,
		"FieldByIndex":reflectx.FieldByIndex,
		"FieldByName":reflectx.FieldByName,
		"FieldByNameFunc":reflectx.FieldByNameFunc,
		"CanSet":reflectx.CanSet,
		"StructOf":reflectx.StructOf,
	},
}
}