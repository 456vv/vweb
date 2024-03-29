package builtin
import (
	"reflect"
	"fmt"
	"strings"
)
var builtinTypes = map[string]reflect.Type{
	"uintptr":		reflect.TypeOf(uintptr(0)),
	"int":			reflect.TypeOf(0),
	"int8":			reflect.TypeOf(int8(0)),
	"int16":		reflect.TypeOf(int16(0)),
	"int32":		reflect.TypeOf(int32(0)),
	"int64":		reflect.TypeOf(int64(0)),
	"uint":			reflect.TypeOf(uint(0)),
	"uint8":		reflect.TypeOf(uint8(0)),
	"uint16":		reflect.TypeOf(uint16(0)),
	"uint32":		reflect.TypeOf(uint32(0)),
	"uint64":		reflect.TypeOf(uint64(0)),
	"bool":			reflect.TypeOf(false),
	"float32":		reflect.TypeOf(float32(0)),
	"float64":		reflect.TypeOf(float64(0)),
	"complex64":	reflect.TypeOf(complex64(0)),
	"complex128":	reflect.TypeOf(complex128(0)),
	"string":		reflect.TypeOf(""),
	"byte":			reflect.TypeOf(byte(0)),
	"rune":			reflect.TypeOf(rune(0)),
	"interface":	reflect.TypeOf((*any)(nil)).Elem(),
	"value":		reflect.TypeOf((*reflect.Value)(nil)).Elem(),
	"type":			reflect.TypeOf((*reflect.Type)(nil)).Elem(),
	"error":        reflect.TypeOf((*error)(nil)).Elem(),
	"struct":		reflect.TypeOf((*struct{})(nil)).Elem(),
}

//格式：
//string				生成string
//string:string			生成map[string]string
//:string				生成[]string
func builtinType(typ any) reflect.Type {
	if t, ok := typ.(string); ok {
		ts := strings.Split(t,":")
		v0, ok0 := builtinTypes[ts[0]]
		
		if len(ts) == 2 {
			//带有:符号
			v1, ok1 := builtinTypes[ts[1]]
			if ts[0] == "" && ok1 {
				//[]T
				return reflect.SliceOf(v1)
			}else if ok0 && ok1 {
				//map[T]T
				return reflect.MapOf(v0, v1)
			}
		}else if ok0 {
			//单个类型
			return v0
		}
		//下面退出默认是字符类型
	}else if t, ok := typ.(reflect.Type); ok {
		return t
	}else if v, ok := typ.(reflect.Value); ok {
		return v.Type()
	}
	return reflect.TypeOf(typ)
}
func rkind(a any) reflect.Kind {
	return reflect.TypeOf(a).Kind()
}

//[string,int,string,float64,...]
//判断可转换的值是int还是float64
//这个常用于map类型
func kind2Args(args []any, idx int) reflect.Kind {
	kind := rkind(args[idx])
	for i := 2; i < len(args); i += 2 {
		if t := rkind(args[i+idx]); t != kind {
			if kind == reflect.Float64 || kind == reflect.Int {
				if t == reflect.Int {
					continue
				}
				if t == reflect.Float64 {
					//如果参数中有 int flaot 则默认选float
					kind = reflect.Float64
					continue
				}
			}
			return reflect.Invalid
		}
	}
	return kind
}
//[int,float64,...]
//判断可转换的值是int还是float64
//这个常用于array类型
func kindArgs(args []any) reflect.Kind {
	kind := rkind(args[0])
	for i := 1; i < len(args); i++ {
		if t := rkind(args[i]); t != kind {
			if kind == reflect.Float64 || kind == reflect.Int {
				if t == reflect.Int {
					continue
				}
				if t == reflect.Float64 {
					//如果参数中有 int flaot 则默认选float
					kind = reflect.Float64
					continue
				}
			}
			return reflect.Invalid
		}
	}
	return kind
}

//判断类型
func asInt(a any) int {
	switch v := a.(type) {
	case int:
		return v
	}
	panic(fmt.Sprintf("Unable to convert, type is %s", rkind(a).String()))
}

//判断类型
func asFloat(a any) float64 {
	switch v := a.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	panic(fmt.Sprintf("Unable to convert, type is %s", rkind(a).String()))
}

//判断v类型是否能转为telem类型
func autoConvert(telem reflect.Type, v any) reflect.Value {
	if v == nil {
		return reflect.Zero(telem)
	}
	val := reflect.ValueOf(v)
	tkind := telem.Kind()
	kind := val.Kind()
	if tkind == kind || tkind == reflect.Interface{
		//类型相等，不需要转换
		return val
	}
	//判断数字类型
	if convertible(kind, tkind) {
		return val.Convert(telem)
	}
	panic(fmt.Sprintf("Can't convert `%v` to `%v` automatically", val.Type(), telem))
}

//能否数字类型转换
func convertible(kind, tkind reflect.Kind) bool {
	//数字类型，kind->tkind
	if tkind >= reflect.Int && tkind <= reflect.Uintptr {
		return kind >= reflect.Int && kind <= reflect.Uintptr
	}
	//浮点类型，kind->tkind
	if tkind == reflect.Float64 || tkind == reflect.Float32 {
		return kind >= reflect.Int && kind <= reflect.Float64
	}
	return false
}

//设置map
func setMapMember(m any, args ...any) any {
	var val reflect.Value
	o := reflect.ValueOf(m)
	telem := o.Type().Elem()
	for i := 0; i < len(args); i += 2 {
		key := reflect.ValueOf(args[i])
		t := args[i+1]
		if t == nil {
			val = zeroVal
		} else {
			val = autoConvert(telem, t)
		}
		o.SetMapIndex(key, val)
	}
	return m
}

//设置struct，支持接口
func setMember(m any, args ...any) {
	o := reflect.ValueOf(m)
	for ; o.Kind() == reflect.Ptr || o.Kind() == reflect.Interface; o = o.Elem() {}
	
	if o.Kind() == reflect.Struct {
		setStructMember(o, args...)
		return
	}
	panic(fmt.Sprintf("type `%v` doesn't support `set` operator", reflect.TypeOf(m)))
}

//设置struct
func setStructMember(o reflect.Value, args ...any) {
	var field reflect.Value
	for i := 0; i < len(args); i += 2 {
		switch t := args[i].(type) {
		case string:
			field = o.FieldByName(strings.Title(t))
		case int:
			field = o.Field(t)
		}
		
		if !field.IsValid() {
			panic(fmt.Sprintf("struct `%v` doesn't has name `%v`", o.Type(), args[i]))
		}
		if !field.CanSet() {
			panic(fmt.Sprintf("struct `%v` can't set name `%v`", o.Type(), args[i]))
		}
		field.Set(autoConvert(field.Type(), args[i+1]))
	}
}

//读取struct，支持接口
func getMember(m any, key any) any {
	o := reflect.ValueOf(m)
	for ; o.Kind() == reflect.Ptr || o.Kind() == reflect.Interface; o = o.Elem() {}
	
	if o.Kind() == reflect.Struct {
		return getStructMember(o, key)
		
	}
	return nil
}

//读取struct
func getStructMember(o reflect.Value, key any) any {
	var field reflect.Value
	switch t := key.(type) {
	case string:
		field = o.FieldByName(strings.Title(t))
	case int:
		field = o.Field(t)
	}
	return typeSelect(field)
}

//追加Interface
func appendInterface(a any, vals... any) any{
	va := reflect.ValueOf(a)
	telem := va.Type().Elem()
	x := make([]reflect.Value, len(vals))
	for i, v := range vals {
		x[i] = autoConvert(telem, v)
	}
	return reflect.Append(va, x...).Interface()
}

//追加Float
func appendFloats(a []float64, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case float64:
			a = append(a, val)
		case float32:
			a = append(a, float64(val))
		case int:
			a = append(a, float64(val))
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, float64(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, float64(rv.Uint()))
        	//case reflect.Float32, reflect.Float64:
			//	a = append(a, rv.Float())
			default:
				panic("unsupported: []float64 append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

//追加Int
func appendInts(a []int, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case int:
			a = append(a, val)
		case float32:
			a = append(a, int(val))
		case float64:
			a = append(a, int(val))
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, int(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, int(rv.Uint()))
        	//case reflect.Float32, reflect.Float64:
			//	a = append(a, int(rv.Float()))
			default:
				panic("unsupported: []int append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

//追加Byte
func appendBytes(a []byte, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case byte:
			a = append(a, val)
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, byte(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, byte(rv.Uint()))
        	case reflect.Float32, reflect.Float64:
				a = append(a, byte(rv.Float()))
			default:
				panic("unsupported: []byte append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

//追加Rune
func appendRunes(a []rune, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case rune:
			a = append(a, val)
		default:
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a = append(a, rune(rv.Int()))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				a = append(a, rune(rv.Uint()))
        	case reflect.Float32, reflect.Float64:
				a = append(a, rune(rv.Float()))
			default:
				panic("unsupported: []byte append " + reflect.TypeOf(v).String())
			}
		}
	}
	return a
}

//追加String
func appendStrings(a []string, vals ...any) any {
	for _, v := range vals {
		switch val := v.(type) {
		case string:
			a = append(a, val)
		default:
			a = append(a, fmt.Sprint(val))
		}
	}
	return a
}


func typeString(a any) string {
	if a == nil {
		return "nil"
	}
	return reflect.TypeOf(a).String()
}

func panicUnsupportedOp1(op string, a any) any {
	ta := typeString(a)
	panic("unsupported operator: " + op + ta)
}

func panicUnsupportedOp2(op string, a, b any) any {
	ta := typeString(a)
	tb := typeString(b)
	panic("unsupported operator: " + ta + op + tb)
}

func panicUnsupportedFn(fn string, args ...any) any {
	targs := make([]string, len(args))
	for i, a := range args {
		targs[i] = typeString(a)
	}
	panic("unsupported function: " + fn + "(" + strings.Join(targs, ",") + ")")
}

//找出最大Int
func maxInt(args []any) (max int) {
	max = args[0].(int)
	for i := 1; i < len(args); i++ {
		if t := args[i].(int); t > max {
			max = t
		}
	}
	return
}

//找出最大Float
func maxFloat(args []any) (max float64) {
	max = asFloat(args[0])
	for i := 1; i < len(args); i++ {
		if t := asFloat(args[i]); t > max {
			max = t
		}
	}
	return
}

//找出最小Int
func minInt(args []any) (min int) {
	min = args[0].(int)
	for i := 1; i < len(args); i++ {
		if t := args[i].(int); t < min {
			min = t
		}
	}
	return
}

//找出最小Float
func minFloat(args []any) (min float64) {
	min = asFloat(args[0])
	for i := 1; i < len(args); i++ {
		if t := asFloat(args[i]); t < min {
			min = t
		}
	}
	return
}

//真实内存
func inDirect(v reflect.Value) reflect.Value {
	for ; v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface; v = v.Elem() {}
    return v
}

//判断有数据长度
func isTrue(val reflect.Value) bool {
	if !val.IsValid() {
		return false
	}
	switch val.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return val.Len() > 0
	case reflect.Bool:
		return val.Bool()
	case reflect.Complex64, reflect.Complex128:
		return val.Complex() != 0
	case reflect.Chan, reflect.Func, reflect.Ptr, reflect.Interface:
		return !val.IsNil()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return val.Int() != 0
	case reflect.Float32, reflect.Float64:
		return val.Float() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return val.Uint() != 0
	case reflect.Struct:
		return true
	}
	return false
}

//读出真实类型数据
func typeSelect(v reflect.Value) any {
    switch v.Kind() {
    case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
        return v.Int()
    case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
        return v.Uint()
    case reflect.Float32, reflect.Float64:
        return v.Float()
    case reflect.Bool:
        return v.Bool()
    case reflect.Complex64, reflect.Complex128:
        return v.Complex()
    case reflect.Invalid:
        return nil
    case reflect.String:
        return v.String()
   	case reflect.UnsafePointer:
   		return v.Pointer()
    case reflect.Slice, reflect.Array:
        if v.CanInterface() {
            return v.Interface()
        }
        
        l := v.Len()
        c := v.Cap()
        vet := reflect.SliceOf(v.Elem().Type())
        cv := reflect.MakeSlice(vet, l, c)
        for i:=0; i<l; i++ {
        	cv = reflect.Append(cv, reflect.ValueOf(typeSelect(v.Index(i))))
        }
        return cv.Interface()
    default:
    	//Interface
    	//Map
    	//Struct
    	//Chan
    	//Func
    	//Ptr
        if v.CanInterface() {
            return v.Interface()
        }
    }
    
   panic(fmt.Errorf("vweb: 该类型 %s，无法转换为 interface 类型", v.Kind()))
}

func typeConvert(av, bv reflect.Value) bool {
	if !av.CanSet() {
		return false
	}
	switch bv.Kind() {
	case reflect.Struct:
		//{} to *{}
		
		//防止av是nil值，最后得到是invalid
		typeInit(av, false)
		//转到最后一层
		for ; av.Kind() == reflect.Ptr || av.Kind() == reflect.Interface; av = av.Elem(){}
	case reflect.Interface:
		//interface -> ptr
		for ; bv.Kind() == reflect.Interface; bv = bv.Elem(){}
		//bv "接口"是 nil 值，是无效的类型
		if bv.Kind() == reflect.Invalid {return false}
	case reflect.Invalid:
		//bv 是 nil
		return false
	}
	
	avt := av.Type()
	if bv.CanConvert(avt) {
		//*{} to *{}
		t := bv.Convert(avt)
		av.Set(t)
		return true
	}
	return false
}

func typeInit(v reflect.Value, isZero bool, args ...any) {
	//无参数，仅初始化
	pv := v
	for ;v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface;{
		//创建下一层
		//1，空指针
		//2，非接口
		if v.IsNil() && v.Kind() != reflect.Interface {
			//Chan，Func，Interface，Map，Ptr，或Slice
			nv := reflect.New(v.Type().Elem())
			pv.Set( nv )
			pv = nv
			v = nv.Elem()
			continue
		}
		//1，接口类型，进入下一层
		pv = v
		v = v.Elem()
	}
	
	if isZero && v.CanSet() {
		switch v.Kind() {
		case reflect.Map:
			l := 0
			if len(args) > 0 {
				l,_ = args[0].(int)
			}
			v.Set(reflect.MakeMapWithSize(v.Type(), l))
			return
		case reflect.Slice:
			l, c := 0,0
			if len(args) > 0 {
				l,_ = args[0].(int)
				c = l
			}
			if len(args) > 1 {
				c,_ = args[1].(int)
				if c < l {
					c = l
				}
			}
			v.Set(reflect.MakeSlice(v.Type(), l, c))
			return
		case reflect.Func:
			if len(args) > 0 {
				f,_ := args[0].(func([]reflect.Value) []reflect.Value)
				v.Set(reflect.MakeFunc(v.Type(), f))
				return
			}
		case reflect.Chan:
			l := 0
			if len(args) == 1 {
				l,_ = args[0].(int)
			}
			v.Set(reflect.MakeChan(v.Type(), l))
			return
		}
		v.Set(reflect.Zero(v.Type()))
	}
}
