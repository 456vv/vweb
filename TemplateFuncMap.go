package vweb
import(
    //"strings"
    "reflect"
    "fmt"
    "unsafe"
)

/*
注：如果你要使用更高级的功能，需要更新标准库源代码。
*/


func templateFuncMapError(v interface{}) error {
    if errs, ok := v.([]reflect.Value); ok {
        l   := len(errs)
        if l==0 {return nil}
        err := errs[l-1]
        if err.CanInterface() {
        	inf := err.Interface()
            if e, ok := inf.(error); ok {
                return e
            }else if inf == nil {
            	return nil
            }
        }
        return fmt.Errorf("Error: 判断最后一个参数不是错误类型。%s", err)
    }else if err, ok := v.(error); ok {
        return err
    }
    return nil
}

// 模板函数映射
var TemplateFuncMap      = map[string]interface{}{
	"Return":func(){},
    "ForMethod": ForMethod,
    "ForType": ForType,
    "TypeSelect": TypeSelect,
    "InDirect": InDirect,
    "DepthField": DepthField,
	"ReflectField":func(inf interface{}, i int) reflect.Value {return reflect.Indirect(reflect.ValueOf(inf)).Field(i)},
	"ReflectFieldByName":func(inf interface{}, name string) reflect.Value {return reflect.Indirect(reflect.ValueOf(inf)).FieldByName(name)},
	"ReflectFieldByIndex":func(inf interface{}, index []int) reflect.Value {return reflect.Indirect(reflect.ValueOf(inf)).FieldByIndex(index)},
	"ReflectMethod":func(inf interface{}, i int) reflect.Value {return reflect.Indirect(reflect.ValueOf(inf)).Method(i)},
	"ReflectMethodByName":func(inf interface{}, name string) reflect.Value {return reflect.Indirect(reflect.ValueOf(inf)).MethodByName(name)},
	"ReflectIndex":func(inf interface{}, i int) reflect.Value {return reflect.Indirect(reflect.ValueOf(inf)).Index(i)},
	"_reflectValue_":func(s []reflect.Value, v ...reflect.Value) []reflect.Value {return append(s, v...)},
	"GoCall":func(call interface{}, args ...interface{}){
		var (
			callv 	= reflect.ValueOf(call)
			inv 	[]reflect.Value
		)
		for arg := range args {
			inv = append(inv, reflect.ValueOf(arg))
		}
		go callv.Call(inv)
	},
	"GoCallSlice":func(call interface{}, args ...interface{}) {
		var (
			callv 	= reflect.ValueOf(call)
			inv 	[]reflect.Value
		)
		for arg := range args {
			inv = append(inv, reflect.ValueOf(arg))
		}
		go callv.CallSlice(inv)
	},
	"PtrTo":func(inf interface{}) interface{} {v := reflect.Indirect(reflect.ValueOf(inf));return TypeSelect(v)},
    "ToPtr":func(inf interface{}) interface{} {return &inf},
	"Nil":func() interface{} {return nil},
	"NotNil":func(inf interface{}) bool {return inf != nil},
	"IsNil":func(inf interface{}) bool {return inf == nil},
    "StringToByte": func(s string) []byte {return []byte(s)},
    "StringToRune": func(s string) []rune {return []rune(s)},
    "RuneToString": func(r []rune) string {return string(r)},
    "ByteToString": func(b []byte) string {return string(b)},
    "_Append_": func(s []interface{}, v ...interface{}) interface{} {return append(s, v...)},
    "Pointer":func(inf interface{}) unsafe.Pointer {return unsafe.Pointer(reflect.ValueOf(inf).Pointer())},
    "Uintptr":func(pointer unsafe.Pointer) uintptr {return uintptr(pointer)},
    "_Uintptr_": func(s []uintptr, v ...uintptr) []uintptr {return append(s, v...)},
    "_Uintptr": func(s uintptr) *uintptr {return &s},
    "Uintptr_": func(s *uintptr) uintptr {return *s},
    "SetUintptr": func(s *uintptr, v uintptr) *uintptr {*s = v;return s},
    "Byte":func(pointer unsafe.Pointer) *byte {return (*byte)(unsafe.Pointer(pointer))},
    "_Byte_": func(s []byte, v ...byte) []byte {return append(s, v...)},
    "_Byte": func(s byte) *byte {return &s},
    "Byte_": func(s *byte) byte {return *s},
    "SetByte": func(s *byte, v byte) *byte {*s = v;return s},
    "Rune":func(pointer unsafe.Pointer) *rune {return (*rune)(unsafe.Pointer(pointer))},
    "_Rune_": func(s []rune, v ...rune) []rune {return append(s, v...)},
    "_Rune": func(s rune) *rune {return &s},
    "Rune_": func(s *rune) rune {return *s},
    "SetRune": func(s *rune, v rune) *rune {*s = v;return s},
    "String":func(pointer unsafe.Pointer) *string {return (*string)(unsafe.Pointer(pointer))},
    "_String_": func(s []string, v ...string) []string {return append(s, v...)},
    "_String": func(s string) *string {return &s},
    "String_": func(s *string) string {return *s},
    "SetString": func(s *string, v string) *string {*s = v;return s},
    "Int":func(pointer unsafe.Pointer) *int {return (*int)(unsafe.Pointer(pointer))},
    "_Int_": func(s []int, i ...int) []int {return append(s, i...)},
    "_Int": func(i int) *int {return &i},
    "Int_": func(i *int) int {return *i},
    "SetInt": func(i *int, v int) *int {*i = v;return i},
    "Int8":func(pointer unsafe.Pointer) *int8 {return (*int8)(unsafe.Pointer(pointer))},
    "_Int8_": func(s []int8, i ...int8) []int8 {return append(s, i...)},
    "Int16":func(pointer unsafe.Pointer) *int16 {return (*int16)(unsafe.Pointer(pointer))},
    "_Int16_": func(s []int16, i ...int16) []int16 {return append(s, i...)},
    "Int32":func(pointer unsafe.Pointer) *int32 {return (*int32)(unsafe.Pointer(pointer))},
    "_Int32_": func(s []int32, i ...int32) []int32 {return append(s, i...)},
    "_Int32": func(i int32) *int32 {return &i},
    "Int32_": func(i *int32) int32 {return *i},
    "SetInt32": func(i *int32, v int32) *int32 {*i = v;return i},
    "Int64":func(pointer unsafe.Pointer) *int64 {return (*int64)(unsafe.Pointer(pointer))},
    "_Int64_": func(s []int64, i ...int64) []int64 {return append(s, i...)},
    "_Int64": func(i int64) *int64 {return &i},
    "Int64_": func(i *int64) int64 {return *i},
    "SetInt64": func(i *int64, v int64) *int64 {*i = v;return i},
    "Uint":func(pointer unsafe.Pointer) *uint {return (*uint)(unsafe.Pointer(pointer))},
    "_Uint_": func(s []uint, i ...uint) []uint {return append(s, i...)},
    "_Uint": func(i uint) *uint {return &i},
    "Uint_": func(i *uint) uint {return *i},
    "SetUint": func(i *uint, v uint) *uint {*i = v;return i},
    "Uint8":func(pointer unsafe.Pointer) *uint8 {return (*uint8)(unsafe.Pointer(pointer))},
    "_Uint8_": func(s []uint8, i ...uint8) []uint8 {return append(s, i...)},
    "Uint16":func(pointer unsafe.Pointer) *uint16 {return (*uint16)(unsafe.Pointer(pointer))},
    "_Uint16_": func(s []uint16, i ...uint16) []uint16 {return append(s, i...)},
    "Uint32":func(pointer unsafe.Pointer) *uint32 {return (*uint32)(unsafe.Pointer(pointer))},
    "_Uint32_": func(s []uint32, i ...uint32) []uint32 {return append(s, i...)},
    "_Uint32": func(i uint32) *uint32 {return &i},
    "Uint32_": func(i *uint32) uint32 {return *i},
    "SetUint32": func(i *uint32, v uint32) *uint32 {*i = v;return i},
    "Uint64":func(pointer unsafe.Pointer) *uint64 {return (*uint64)(unsafe.Pointer(pointer))},
    "_Uint64_": func(s []uint64, i ...uint64) []uint64 {return append(s, i...)},
    "_Uint64": func(i uint64) *uint64 {return &i},
    "Uint64_": func(i *uint64) uint64 {return *i},
    "SetUint64": func(i *uint64, v uint64) *uint64 {*i = v;return i},
    "Float32":func(pointer unsafe.Pointer) *float32 {return (*float32)(unsafe.Pointer(pointer))},
    "_Float32_": func(s []float32, f ...float32) []float32 {return append(s, f...)},
    "_Float32": func(f float32) *float32 {return &f},
    "Float32_": func(f *float32) float32 {return *f},
    "SetFloat32": func(f *float32, v float32) *float32 {*f = v;return f},
    "Float64":func(pointer unsafe.Pointer) *float64 {return (*float64)(unsafe.Pointer(pointer))},
    "_Float64_": func(s []float64, f ...float64) []float64 {return append(s, f...)},
    "_Float64": func(f float64) *float64 {return &f},
    "Float64_": func(f *float64) float64 {return *f},
    "SetFloat64": func(f *float64, v float64) *float64 {*f = v;return f},
    "Complex64":func(pointer unsafe.Pointer) *complex64 {return (*complex64)(unsafe.Pointer(pointer))},
    "_Complex64_": func(s []complex64, c ...complex64) []complex64 {return append(s, c...)},
    "_Complex64": func(c complex64) *complex64 {return &c},
    "Complex64_": func(c *complex64) complex64 {return *c},
    "SetComplex64": func(c *complex64, v complex64) *complex64 {*c = v;return c},
    "Complex128":func(pointer unsafe.Pointer) *complex128 {return (*complex128)(unsafe.Pointer(pointer))},
    "_Complex128_": func(s []complex128, c ...complex128) []complex128 {return append(s, c...)},
    "_Complex128": func(c complex128) *complex128 {return &c},
    "Complex128_": func(c *complex128) complex128 {return *c},
    "SetComplex128": func(c *complex128, v complex128) *complex128 {*c = v;return c},
    "Error": func(v interface{}) bool {
		return templateFuncMapError(v) != nil
    },
    "NotError": func(v interface{}) bool {
       return templateFuncMapError(v) == nil
    },
    "Compute": func(x interface{}, symbol string, y interface{}) (i interface{}, err error) {
        xx := reflect.ValueOf(x)
        yy := reflect.ValueOf(y)
        xx = InDirect(xx)
        yy = InDirect(yy)
        if xx.Kind() != yy.Kind() {
            return 0, fmt.Errorf("Compute: 两个类型不相等？%v != %v", xx.Kind(), yy.Kind())
        }
        switch xx.Kind() {
        case reflect.String:
        	XS := xx.String()
            YS := yy.String()
            var XYS string
            switch symbol {
                case "+":XYS = XS+YS
                default:
                    err = fmt.Errorf("Compute: 该类型不支持的算法(%s)？", symbol)
            }
            return XYS, err
        case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
            XI := xx.Int()
            YI := yy.Int()
            var XYI int64
            switch symbol {
                case "+":XYI = XI+YI
                case "-":XYI = XI-YI
                case "*":XYI = XI*YI
                case "/":XYI = XI/YI
                case "%":XYI = XI%YI
                case "&":XYI = XI&YI
                case "|":XYI = XI|YI
                case "^":XYI = XI^YI
                case "&^":XYI = XI&^YI
                default:
                    err = fmt.Errorf("Compute: 该类型不支持的算法(%s)？", symbol)
            }
            return XYI, err
        case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
            XU := xx.Uint()
            YU := yy.Uint()
            var XYU uint64
            switch symbol {
                case "+":XYU = XU+YU
                case "-":XYU = XU-YU
                case "*":XYU = XU*YU
                case "/":XYU = XU/YU
                case "%":XYU = XU%YU
                case "&":XYU = XU&YU
                case "|":XYU = XU|YU
                case "^":XYU = XU^YU
                case "&^":XYU = XU&^YU
                case "<<":XYU = XU<<YU
                case ">>":XYU = XU>>YU
                default:
                    err = fmt.Errorf("Compute: 该类型不支持的算法(%s)？", symbol)
            }
            return XYU, err
        case reflect.Float32, reflect.Float64:
            XF := xx.Float()
            YF := yy.Float()
            var XYF float64
            switch symbol {
                case "+":XYF = XF+YF
                case "-":XYF = XF-YF
                case "*":XYF = XF*YF
                case "/":XYF = XF/YF
                default:
                    err = fmt.Errorf("Compute: 该类型不支持的算法(%s)？", symbol)
            }
            return XYF, err
        case reflect.Uintptr:
            XP := xx.UnsafeAddr()
            YP := yy.UnsafeAddr()
            var XYP uintptr
            switch symbol {
                case "+":XYP = XP+YP
                case "-":XYP = XP-YP
                case "*":XYP = XP*YP
                case "/":XYP = XP/YP
                default:
                    err = fmt.Errorf("Compute: 该类型不支持的算法(%s)？", symbol)
            }
            return XYP, err
       	default:
       		 return nil, fmt.Errorf("Compute: 这是不符合计算的类型(%v)？", xx.Kind())
        }
    },
}


