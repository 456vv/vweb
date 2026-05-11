package dynamic

import (
	"errors"

	"github.com/456vv/vweb/v3"
	"github.com/456vv/x/vweb_dynamic"
)

func Module(name string) (vweb.DynamicTemplater, error) {
	switch name {
	case "yaegi":
		return &vweb_dynamic.Yaegi{}, nil
	case "igop", "ixgo":
		return &vweb_dynamic.Ixgo{}, nil
	case "wasm-01000000", "wasm-02000000":
		return &vweb_dynamic.Wazero{}, nil
	}
	return nil, errors.New("vweb: the file type does not support dynamic parsing")
}
