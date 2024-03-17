package vweb

import (
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/456vv/verror"
	"github.com/456vv/vweb/v2/builtin"
)

// ForMethod	遍历方法
//
//	x any	  类型
//	all	bool		  true不可导出一样可以打印出来
//	string			  字符串
func ForMethod(x any) string {
	t := reflect.TypeOf(x)
	var s string
	for i := 0; i < t.NumMethod(); i++ {
		tm := t.Method(i)
		s += fmt.Sprintf("%d %s	%s\t\t=	%v \n", tm.Index, tm.PkgPath, tm.Name, tm.Type)
	}
	return s
}

// ForType 遍历字段
//
//	x any	类型
//	lower bool		打印出小写字段
//	depth int		打印深度
func ForType(x any, lower bool, depth int) string {
	return forType(x, 0, lower, depth)
}

func forType(x any, floor int, lower bool, depth int) string {
	var (
		v, z reflect.Value
		tf   reflect.StructField
		s    string
		flx  = strings.Repeat("\t", floor)
		k    any
	)

	v, ok := x.(reflect.Value)
	if !ok {
		v = reflect.ValueOf(x)
	}
	rv := inDirect(v)
	rt := rv.Type()
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		if rv.IsValid() && rv.CanInterface() {
			k = rv.Interface()
		}
		s += fmt.Sprintf("%s L%d %v\t%v = %#v\r\n", flx, rv.Len(), rt.PkgPath(), v.Type(), k)
		for i := 0; i < rv.Len(); i++ {
			irv := inDirect(rv.Index(i))
			s += forType(irv, floor+1, lower, depth)
		}
	case reflect.Struct:
		for i := 0; i < rt.NumField(); i++ {
			tf = rt.Field(i)
			if tf.Name != "" && (tf.Name[0] < 65 || tf.Name[0] > 90) && !lower || (lower && floor != 0) {
				// 小写字段
				continue
			}

			z = reflect.Indirect(rv.Field(i))
			var ks string
			if z.IsValid() {
				if z.CanInterface() {
					k = z.Interface()
				}
				if z.Kind() == reflect.Slice && z.Type().Elem().Kind() == reflect.Uint8 {
					if utf8.Valid(z.Bytes()) {
						ks = fmt.Sprintf("//%s", z.Bytes())
					} else {
						ks = fmt.Sprintf("//%#v", z.Bytes())
					}
				}
			}
			s += fmt.Sprintf("%s %v	%v %v\t%v `%v` = %#v %s\r\n", flx, tf.Index, tf.PkgPath, tf.Name, tf.Type, tf.Tag, k, ks)
			if floor+1 != depth {
				s += forTypeSub(z, floor+1, lower, depth)
			}
		}
	default:
		return fmt.Sprintf("%#v", v.String())
	}
	return s
}

func forTypeSub(node reflect.Value, floor int, lower bool, depth int) (s string) {
	switch node.Kind() {
	case reflect.Struct:
		s = forType(node, floor, lower, depth)
	case reflect.Slice, reflect.Array:
		for i := 0; i < node.Len(); i++ {
			irv := inDirect(node.Index(i))
			s += forTypeSub(irv, floor, lower, depth)
		}
	}
	return s
}

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
		for i := 0; i < l; i++ {
			cv = reflect.Append(cv, reflect.ValueOf(typeSelect(v.Index(i))))
		}
		return cv.Interface()
	default:
		// Interface
		// Map
		// Struct
		// Chan
		// Func
		// Ptr
		if v.CanInterface() {
			return v.Interface()
		}
	}

	panic(fmt.Errorf("vweb: 该类型 %s，无法转换为	interface 类型", v.Kind()))
}

// InDirect 指针到内存
//
//	v reflect.Value		   映射引用为真实内存地址
//	reflect.Value		   真实内存地址
func InDirect(v reflect.Value) reflect.Value {
	return inDirect(v)
}

func inDirect(v reflect.Value) reflect.Value {
	for ; v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface; v = v.Elem() {
	}
	return v
}

// DepthField 快速深入读取字段
//
//	s any		 Struct
//	ndex ... any 字段
//	field any	 字段
//	err	error			 错误
//	例：
//	type A struct {
//	 B
//	}
//	type B struct {
//	 C
//	 F map[string]string
//	 G []string
//	}
//	type C struct {
//	 D int
//	}
//	func main(){
//	 a := A{}
//		fidld, err := DepthField(a,	"B", "C", "D")
//		fmt.Println(fidld, err)
//		//0	<nil>
//	   }
func DepthField(s any, index ...any) (field any, err error) {
	field = s
	for _, i := range index {
		field, err = depthField(field, i)
		if err != nil {
			return nil, err
		}
	}
	return field, nil
}

func depthField(s any, index any) (any, error) {
	sv := reflect.ValueOf(s)
	sid := InDirect(sv)
	var v reflect.Value
	switch sid.Kind() {
	case reflect.Struct:
		switch index := index.(type) {
		case string:
			v = sid.FieldByName(index)
		case int:
			v = sid.Field(index)
		}
	case reflect.Map:
		if sid.IsNil() {
			return nil, verror.TrackErrorf("vweb: 该字段是 nil。错误的字段名为(%#v)", index)
		}
		v = sid.MapIndex(reflect.ValueOf(index))
	case reflect.Slice, reflect.Array:
		if i, ok := index.(int); ok && sid.Len() > i {
			v = sid.Index(i)
		}
	default:
		return nil, verror.TrackErrorf("vweb: 非结构类型，无法正确读取。错误的类型为（%s）", sid.Kind())
	}
	if v.Kind() != reflect.Invalid {
		return v.Interface(), nil
	}
	return nil, verror.TrackErrorf("vweb: 该字段不是有效。错误的字段名为（%#v）", index)
}

// CopyStruct 结构字段从src 复制 dsc，不需要相同的结构。他只复制相同类型的字段。
//
//	dsc, src any									目标，源结构
//	exclude func(name string, dsc, src reflect.Value) bool	排除处理函数，返回true跳过
//	error	错误
func CopyStruct(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(dsc, src, exclude, false)
}

func CopyStructDeep(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(dsc, src, exclude, true)
}

func copyStruct(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool, deep bool) error {
	va, ok := dsc.(reflect.Value)
	if !ok {
		va = reflect.ValueOf(dsc)
	}
	vb, ok := src.(reflect.Value)
	if !ok {
		vb = reflect.ValueOf(src)
	}
	va = inDirect(va)
	vb = inDirect(vb)

	if va.Kind() != vb.Kind() || va.Kind() != reflect.Struct {
		return verror.TrackErrorf("仅支持struct类型，dsc(%s)，	src(%s)", va.Kind(), vb.Kind())
	}

	bt := vb.Type()
	for i := 0; i < bt.NumField(); i++ {

		bvf := vb.Field(i)
		if !bvf.IsValid() {
			continue
		}

		info := bt.Field(i)
		avf := va.FieldByName(info.Name)

		// 排除字段
		if exclude != nil && exclude(info.Name, avf, bvf) {
			continue
		}
		if !avf.IsValid() {
			// 目标结构不存在该字段
			continue
		}

		// 初始化指针
		avfi := inDirect(avf)
		bvfi := inDirect(bvf)
		if !avfi.IsValid() && bvfi.IsValid() {
			builtin.Init(avf)
			avfi = inDirect(avf)
		}

		afk := avfi.Kind()
		bfk := bvfi.Kind()

		// 深度复制
		if deep && afk == bfk && afk == reflect.Struct {
			copyStruct(avf, bvf, exclude, deep)
			continue
		}

		// Map
		if afk == bfk && afk == reflect.Map {
			if bvfi.IsNil() {
				// 源是空的
				continue
			}

			btf := bvfi.Type()
			atf := avfi.Type()

			if !btf.Key().ConvertibleTo(atf.Key()) || !btf.Elem().ConvertibleTo(atf.Elem()) {
				// 不可以转换
				continue
			}

			if avfi.IsNil() {
				mt := reflect.MapOf(atf.Key(), atf.Elem())
				mv := reflect.MakeMapWithSize(mt, bvfi.Len())
				avfi.Set(mv)
			}
			bfmr := bvfi.MapRange()
			for bfmr.Next() {
				key := bfmr.Key().Convert(atf.Key())
				val := bfmr.Value().Convert(atf.Elem())
				avfi.SetMapIndex(key, val)
			}
			continue
		}

		if avf.CanSet() {
			if bvf.Type().AssignableTo(avf.Type()) {
				avf.Set(bvf)
			} else if bvf.Type().ConvertibleTo(avf.Type()) {
				bvv := bvf.Convert(avf.Type())
				avf.Set(bvv)
			}
		}
	}

	return nil
}
