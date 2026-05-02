package dynamic

import (
	"errors"

	"github.com/456vv/vweb/v3"
)

func Module(name string) (vweb.DynamicTemplater, error) {
	return nil, errors.New("vweb: the file type does not support dynamic parsing")
}
