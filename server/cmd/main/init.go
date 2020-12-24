package main

import (
	"github.com/456vv/vweb/v2"
)

func init(){
	//给template模板增加模块包
	for name, pkg := range templatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range luteTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range yamlTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range tomlTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
	for name, pkg := range reflectxTemplatePackage() {
		vweb.ExtendTemplatePackage(name, pkg)
	}
}
