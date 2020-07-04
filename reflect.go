package vweb
import (
    "reflect"
    "fmt"
    "github.com/456vv/verror"
    //"unsafe"
)



//ForMethod 遍历方法
//	x interface{}     类型
//	all bool		  true不可导出一样可以打印出来
//	string            字符串
func ForMethod(x interface{}) string {
    var t = reflect.TypeOf(x)
    var s string
    for i:=0; i<t.NumMethod(); i++ {
        tm := t.Method(i)
        s += fmt.Sprintf("%d %s %s\t\t= %v \n", tm.Index, tm.PkgPath, tm.Name, tm.Type)
   }
   return s
}

//ForType 遍历字段
//	x interface{}     类型
//	all bool		  true不可导出一样可以打印出来
//	string            字符串
func ForType(x interface{}, all bool) string {
    return forType(x, "", "", 0, all)
}
func forType(x interface{}, str string, flx string, floor int, all bool) string {
    var (
        v, z reflect.Value
        f reflect.StructField
        t reflect.Type
        k interface{}
        s string
    )
    v, ok := x.(reflect.Value)
    if !ok {
		v = reflect.ValueOf(x)
    }
    v = inDirect(v)
    if v.Kind() != reflect.Struct {
        s += fmt.Sprintf("无法解析(%s): %#v\r\n", v.Kind(), x)
        return s
    }
    t = v.Type()
    for i := 0; i < t.NumField(); i++ {
        f = t.Field(i)
        if f.Name != "" && !all && (f.Name[0]  < 65 || f.Name[0] > 90) {
        	continue
        }
        z = inDirect(v.Field(i))
        if z.IsValid(){
	        k = z
	        if z.CanInterface() {
	        	k = typeSelect(z)
	        }
        }
        s += fmt.Sprintf("%s %v %v %v\t%v `%v` = %v\r\n", flx+str, f.Index, f.PkgPath, f.Name, f.Type, f.Tag, k)
        if z.Kind() == reflect.Struct{
        	floor++
            s += forType(z, str, flx+"  ", floor, all)
        }
    }
    return s
}

func typeSelect(v reflect.Value) interface{} {
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
   return nil
}

//InDirect 指针到内存
//	v reflect.Value        映射引用为真实内存地址
//	reflect.Value          真实内存地址
func InDirect(v reflect.Value) reflect.Value {
    return inDirect(v)
}
func inDirect(v reflect.Value) reflect.Value {
	for ; v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface; v = v.Elem() {}
    return v
}

//DepthField 快速深入读取字段
//  s interface{}        Struct
//  ndex ... interface{} 字段
//  field interface{}    字段
//  err error            错误
//  例：
//  type A struct {
//   B
//  }
//  type B struct {
//   C
//   F map[string]string
//   G []string
//  }
//  type C struct {
//   D int
//  }
//  func main(){
//   a := A{}
//      fidld, err := DepthField(a, "B", "C", "D")
//      fmt.Println(fidld, err)
//      //0 <nil>
//     }
func DepthField(s interface{}, index ... interface{}) (field interface{}, err error) {
    field = s
    for _, i := range index {
    	field, err = depthField(field, i)
        if err != nil {
        	return nil, err
        }
    }
	return field, nil
}

func depthField(s interface{}, index interface{}) (interface{}, error) {
    sv := reflect.ValueOf(s)
    sid := InDirect(sv)
    var reflectValue reflect.Value
    switch sid.Kind() {
    case reflect.Struct:
    	reflectValue = sid.FieldByName(index.(string))
    case reflect.Map:
    	if sid.IsNil() {
     		return nil, verror.TrackErrorf("vweb: 该字段是 nil。错误的字段名为（%#v）", index)
  	 	}
    	reflectValue = sid.MapIndex(reflect.ValueOf(index))
    case reflect.Slice, reflect.Array:
    	if sid.Len() > index.(int) {
    		reflectValue = sid.Index(index.(int))
    	}
    default:
    	return nil, verror.TrackErrorf("vweb: 非结构类型，无法正确读取。错误的类型为（%s）", sid.Kind())
    }
    if reflectValue.Kind() == reflect.Invalid {
    	return nil, verror.TrackErrorf("vweb: 该字段不是有效。错误的字段名为（%#v）", index)
    }
    return reflectValue.Interface(), nil
}

//CopyStruct 结构字段从src 复制 dsc，不需要相同的结构。他只复制相同类型的字段。
//	dsc, src interface{}									目标，源结构
//	handle func(name string, dsc, src reflect.Value) bool	排除处理函数
//	error	错误
func CopyStruct(dsc, src interface{}, handle func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(dsc, src, handle, false)
}

func CopyStructDeep(dsc, src interface{}, handle func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(dsc, src, handle, true)
}

func copyStruct(dsc, src interface{}, handle func(name string, dsc, src reflect.Value) bool, deep bool) error {

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
		return verror.TrackErrorf("仅支持struct类型，dsc(%s)， src(%s)", va.Kind(), vb.Kind())
	}
	
	bt := vb.Type()
	for i:=0;i<bt.NumField();i++{
		
		info := bt.Field(i)
		bvf := vb.Field(i)
		if !bvf.IsValid() {
			continue
		}
		
		avf := va.FieldByName(info.Name)
		
		//排除字段
		if handle != nil && handle(info.Name, avf, bvf) {
			continue
		}
		if !avf.IsValid() {
			//目标结构不存在该字段
			continue
		}
		
		//不可导出的字段，不处理
		//avfn := avf.Type().Name()
        //if avfn != "" && (avfn[0]  < 65 || avfn[0] > 90) {
        //	continue
        //}
		
		//初始化指针
		avfi := inDirect(avf)
		bvfi := inDirect(bvf)
		if !avfi.IsValid() && bvfi.IsValid() {
			avfe := avf
			for ;avfe.Kind() == reflect.Ptr;{
				if avfe.IsNil() {
					//Chan，Func，Interface，Map，Ptr，或Slice
					avfe.Set(reflect.New(avfe.Type().Elem()))
				}
				avfe = avfe.Elem() 
			}
			if !avfe.IsValid() {
				avfe.Set(reflect.Zero(avfe.Type()))
			}
		}
		
		afk := avfi.Kind()
		bfk := bvfi.Kind()

		//深度复制
		if deep && afk == bfk && afk == reflect.Struct {
			copyStruct(avf, bvf, handle, deep)
			continue
		}
		
		//Map
		if afk == bfk && afk == reflect.Map {
			if bvfi.IsNil() {
				//源是空的
				continue
			}
			
			btf := bvfi.Type()
			atf := avfi.Type()
			
			if !btf.Key().ConvertibleTo(atf.Key()) || !btf.Elem().ConvertibleTo(atf.Elem()) {
				//不可以转换
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
			}else if bvf.Type().ConvertibleTo(avf.Type()) {
				bvv := bvf.Convert(avf.Type())
				avf.Set(bvv)
			}
		}
	}
	
	return nil
}
