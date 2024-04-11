package dynamic

import (
	"github.com/456vv/vweb/v2"
	"github.com/456vv/x/vweb_dynamic"
)

func Module() map[string]vweb.DynamicTemplateFunc {
	module := map[string]vweb.DynamicTemplateFunc{
		"yaegi": vweb.DynamicTemplateFunc(func(D *vweb.ServerHandlerDynamic) vweb.DynamicTemplater {
			return &vweb_dynamic.Yaegi{}
		}),
		"template": vweb.DynamicTemplateFunc(func(D *vweb.ServerHandlerDynamic) vweb.DynamicTemplater {
			return &vweb_dynamic.Template{}
		}),
		"igop": vweb.DynamicTemplateFunc(func(D *vweb.ServerHandlerDynamic) vweb.DynamicTemplater {
			return &vweb_dynamic.Igop{}
		}),
	}
	return module
}
