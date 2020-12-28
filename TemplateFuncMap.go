package vweb
import(
    "text/template"
    "reflect"
    "fmt"
   "github.com/456vv/vweb/v2/builtin"
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

func callMethod(f interface{}, name string, args ...interface{}) ([]interface{}, error) {
	
	var isMethodFunc bool
	vfn := reflect.ValueOf(f)
	if vfn.Kind() == reflect.Ptr {
		//func (T *A) B(){}
		if vfn.NumMethod() > 0  {
			t := vfn.MethodByName(name)
			if t.Kind() == reflect.Func {
				vfn = t
				isMethodFunc = true
			}
		}else{
			vfn = inDirect(vfn)
		}
	}
	if vfn.Kind() == reflect.Struct {
		if vfn.NumMethod() > 0  {
			//func (T A) C(){}
			t := vfn.MethodByName(name)
			if t.Kind() == reflect.Func {
				vfn = t
				isMethodFunc = true
			}
		}
	}
	if !isMethodFunc {
		return nil, fmt.Errorf("vweb: the `%s` method was not found in `%v`", name, f)
	}
	return call(vfn, args...)
}

func call(f interface{}, args ...interface{}) ([]interface{}, error){
	ef := execFunc{}
	if err := ef.add(f, args...); err != nil {
		return nil, err
	}
	return ef.exec(), nil
}

// 模板函数映射
var TemplateFunc = template.FuncMap{
	"Import": func(pkgName string) template.FuncMap {return dotPackage[pkgName]},
    "ForMethod": ForMethod,
    "ForType": ForType,
    "InDirect": InDirect,
    "DepthField": DepthField,
    "CopyStruct": CopyStruct,
    "CopyStructDeep": CopyStructDeep,
    "GoTypeTo":builtin.GoTypeTo,
    "GoTypeInit":builtin.GoTypeInit,
    "Value":builtin.Value,						//Value(v) reflect.Value
	"_Value_":func(s []reflect.Value, v ...reflect.Value) []reflect.Value {return append(s, v...)},
	"Call":call,
	"CallMethod":callMethod,
	"Defer":func(f interface{}, args ...interface{}) func() {return func(){call(f, args...)}},
	"DeferMethod":func(f interface{}, name string, args ...interface{}) func() {return func(){callMethod(f, name, args...)}},
	"Go":func(f func()){go f()},
	"PtrTo":func(inf interface{}) interface{} {v := reflect.Indirect(reflect.ValueOf(inf));return typeSelect(v)},
    "ToPtr":func(inf interface{}) interface{} {return &inf},
	"Nil":func() interface{} {return nil},
	"NotNil":func(inf interface{}) bool {return inf != nil},
	"IsNil":func(inf interface{}) bool {return inf == nil},
    "Bytes": builtin.Bytes,
    "Runes": builtin.Runs,
    "Append": builtin.Append,			//Append([]T, value...)
    "Pointer":builtin.Pointer,
    "Uintptr":builtin.Uintptr,
    "_Uintptr_": func(s []uintptr, v ...uintptr) []uintptr {if s==nil && len(v)==0 {return make([]uintptr, 0, 0)};return append(s, v...)},
    "_Uintptr": func(s uintptr) *uintptr {return &s},
    "Uintptr_": func(s *uintptr) uintptr {return *s},
    "SetUintptr": func(s *uintptr, v uintptr) *uintptr {*s = v;return s},
    "Byte":builtin.Byte,
    "_Byte_": func(s []byte, v ...byte) []byte {if s==nil && len(v)==0 {return make([]byte, 0, 0)};return append(s, v...)},
    "_Byte": func(s byte) *byte {return &s},
    "Byte_": func(s *byte) byte {return *s},
    "SetByte": func(s *byte, v byte) *byte {*s = v;return s},
    "Rune":builtin.Rune,
    "_Rune_": func(s []rune, v ...rune) []rune {if s==nil && len(v)==0 {return make([]rune, 0, 0)};return append(s, v...)},
    "_Rune": func(s rune) *rune {return &s},
    "Rune_": func(s *rune) rune {return *s},
    "SetRune": func(s *rune, v rune) *rune {*s = v;return s},
    "String":builtin.String,
    "_String_": func(s []string, v ...string) []string {if s==nil && len(v)==0 {return make([]string, 0, 0)};return append(s, v...)},
    "_String": func(s string) *string {return &s},
    "String_": func(s *string) string {return *s},
    "SetString": func(s *string, v string) *string {*s = v;return s},
    "Bool":builtin.Bool,
    "Int":builtin.Int,
    "_Int_": func(s []int, v ...int) []int {if s==nil && len(v)==0 {return make([]int, 0, 0)};return append(s, v...)},
    "_Int": func(i int) *int {return &i},
    "Int_": func(i *int) int {return *i},
    "SetInt": func(i *int, v int) *int {*i = v;return i},
    "Int8":builtin.Int8,
    "_Int8_": func(s []int8, v ...int8) []int8 {if s==nil && len(v)==0 {return make([]int8, 0, 0)};return append(s, v...)},
    "Int16":builtin.Int16,
    "_Int16_": func(s []int16, v ...int16) []int16 {if s==nil && len(v)==0 {return make([]int16, 0, 0)};return append(s, v...)},
    "Int32":builtin.Int32,
    "_Int32_": func(s []int32, v ...int32) []int32 {if s==nil && len(v)==0 {return make([]int32, 0, 0)};return append(s, v...)},
    "_Int32": func(i int32) *int32 {return &i},
    "Int32_": func(i *int32) int32 {return *i},
    "SetInt32": func(i *int32, v int32) *int32 {*i = v;return i},
    "Int64":builtin.Int64,
    "_Int64_": func(s []int64, v ...int64) []int64 {if s==nil && len(v)==0 {return make([]int64, 0, 0)};return append(s, v...)},
    "_Int64": func(i int64) *int64 {return &i},
    "Int64_": func(i *int64) int64 {return *i},
    "SetInt64": func(i *int64, v int64) *int64 {*i = v;return i},
    "Uint":builtin.Uint,
    "_Uint_": func(s []uint, v ...uint) []uint {if s==nil && len(v)==0 {return make([]uint, 0, 0)};return append(s, v...)},
    "_Uint": func(i uint) *uint {return &i},
    "Uint_": func(i *uint) uint {return *i},
    "SetUint": func(i *uint, v uint) *uint {*i = v;return i},
    "Uint8":builtin.Uint8,
    "_Uint8_": func(s []uint8, v ...uint8) []uint8 {if s==nil && len(v)==0 {return make([]uint8, 0, 0)};return append(s, v...)},
    "Uint16":builtin.Uint16,
    "_Uint16_": func(s []uint16, v ...uint16) []uint16 {if s==nil && len(v)==0 {return make([]uint16, 0, 0)};return append(s, v...)},
    "Uint32":builtin.Uint32,
    "_Uint32_": func(s []uint32, v ...uint32) []uint32 {if s==nil && len(v)==0 {return make([]uint32, 0, 0)};return append(s, v...)},
    "_Uint32": func(i uint32) *uint32 {return &i},
    "Uint32_": func(i *uint32) uint32 {return *i},
    "SetUint32": func(i *uint32, v uint32) *uint32 {*i = v;return i},
    "Uint64":builtin.Uint64,
    "_Uint64_": func(s []uint64, v ...uint64) []uint64 {if s==nil && len(v)==0 {return make([]uint64, 0, 0)};return append(s, v...)},
    "_Uint64": func(i uint64) *uint64 {return &i},
    "Uint64_": func(i *uint64) uint64 {return *i},
    "SetUint64": func(i *uint64, v uint64) *uint64 {*i = v;return i},
    "Float32":builtin.Float32,
    "_Float32_": func(s []float32, v ...float32) []float32 {if s==nil && len(v)==0 {return make([]float32, 0, 0)};return append(s, v...)},
    "_Float32": func(f float32) *float32 {return &f},
    "Float32_": func(f *float32) float32 {return *f},
    "SetFloat32": func(f *float32, v float32) *float32 {*f = v;return f},
    "Float64":builtin.Float64,
    "_Float64_": func(s []float64, v ...float64) []float64 {if s==nil && len(v)==0 {return make([]float64, 0, 0)};return append(s, v...)},
    "_Float64": func(f float64) *float64 {return &f},
    "Float64_": func(f *float64) float64 {return *f},
    "SetFloat64": func(f *float64, v float64) *float64 {*f = v;return f},
    "Complex64":builtin.Complex64,
    "_Complex64_": func(s []complex64, v ...complex64) []complex64 {if s==nil && len(v)==0 {return make([]complex64, 0, 0)};return append(s, v...)},
    "_Complex64": func(c complex64) *complex64 {return &c},
    "Complex64_": func(c *complex64) complex64 {return *c},
    "SetComplex64": func(c *complex64, v complex64) *complex64 {*c = v;return c},
    "Complex128":builtin.Complex128,
    "_Complex128_": func(s []complex128, v ...complex128) []complex128 {if s==nil && len(v)==0 {return make([]complex128, 0, 0)};return append(s, v...)},
    "_Complex128": func(c complex128) *complex128 {return &c},
    "Complex128_": func(c *complex128) complex128 {return *c},
    "SetComplex128": func(c *complex128, v complex128) *complex128 {*c = v;return c},
    "Type":builtin.Type,						//Type(v) reflect.Type
    "Panic":builtin.Panic,						//Panic(v)
    "Make":builtin.Make,						//Make([]T, length, cap)|Make([T]T, length)|Make(Chan, length)
    "MapFrom":builtin.MapFrom,					//MapFrom(M, T1,V1, T2, V2, ...)
    "SliceFrom":builtin.SliceFrom,				//SliceFrom(S, 值0, 值1,...)
    "Delete":builtin.Delete,					//Delete(map[T]T, "key")
    "Set":builtin.Set,							//Set([]T, 位置0,值1, 位置1,值2, 位置2,值3)|Set(map[T]T, 键名0,值1, 键名1,值2, 键名2,值3)|Set(struct{}, 名称0,值1, 名称1,值2, 名称2,值3)
    "Get":builtin.Get,							//Get(map[T]T/[]T/struct{}/string/number, key)
    "Len":builtin.Len,							//Len([]T/string/map[T]T)
    "Cap":builtin.Cap,							//Cap([]T)
    "GetSlice":builtin.GetSlice,				//GetSlice([]T, 1, 5)
    "GetSlice3":builtin.GetSlice3,				//GetSlice3([]T, 1, 5, 7)
    "Copy":builtin.Copy,						//Copy([]T, []T)
    "Compute": builtin.Compute,					//Compute(1, "+", 2)
    "Inc":builtin.Inc,							//Inc returns a+1
    "Dec":builtin.Dec,							//Dec returns a-1
    "Neg":builtin.Neg,							//Neg returns -a
    "Mul":builtin.Mul,							//Mul returns a*b
    "Quo":builtin.Quo,							//Quo returns a/b
    "Mod":builtin.Mod,							//Mod returns a%b
    "Add":builtin.Add,							//Add returns a+b
    "Sub":builtin.Sub,							//Sub returns a-b
    "BitLshr":builtin.BitLshr,					//BitLshr returns a << b
    "BitRshr":builtin.BitRshr,					//BitRshr returns a >> b
    "BitXor":builtin.BitXor,					//BitXor returns a ^ b
    "BitAnd":builtin.BitAnd,					//BitAnd returns a & b
    "BitOr":builtin.BitOr,						//BitOr returns a | b
    "BitNot":builtin.BitNot,					//BitNot returns ^a
    "BitAndNot":builtin.BitAndNot,				//BitAndNot returns a &^ b
    "Or":builtin.Or,							//Or returns 1 || true
    "And":builtin.And,							//And returns 1 && true
    "Not":builtin.Not,							//Not returns !a
    "LT":builtin.LT,							//LT returns a < b
    "GT":builtin.GT,							//GT returns a > b
    "LE":builtin.LE,							//LE returns a <= b
    "GE":builtin.GE,							//GE returns a >= b
    "EQ":builtin.EQ,							//EQ returns a == b
    "NE":builtin.NE,							//NE returns a != b
    "TrySend":builtin.TrySend,					//TrySend(*Chan, value)	不阻塞
    "TryRecv":builtin.TryRecv,					//TryRecv(*Chan, value)	不阻塞
    "Send":builtin.Send,						//Send(*Chan, value)
    "Recv":builtin.Recv,						//Recv(*Chan)
    "Close":builtin.Close,						//Close(*Chan)
    "ChanOf":builtin.ChanOf,					//ChanOf(T)
    "MakeChan":builtin.MakeChan,				//MakeChan(T, size)
    "Max":builtin.Max,							//Max(a1, a2 ...)
    "Min":builtin.Min,							//Min(a1, a2 ...)
    "Error": func(v interface{}) bool {
		return templateFuncMapError(v) != nil
    },
    "NotError": func(v interface{}) bool {
       return templateFuncMapError(v) == nil
    },
}


