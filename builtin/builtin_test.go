package builtin
	
import (
	"testing"
	"reflect"
)

type toTI interface{
	M() int
}
type toT1 struct{
	i int
	T *int
}
func (T *toT1) M() int {return T.i}
type toT2 struct{
	i int
	T *int
}
func (T *toT2) M() int {return T.i}


func Test_typeInit(t *testing.T) {
	fns := []func()bool{
		func()bool{
			var t1 *toT1
			typeInit(reflect.ValueOf(&t1).Elem(), false)
			return t1 != nil
		},
		func()bool{
			var t1 toTI = (*toT1)(nil)
			typeInit(reflect.ValueOf(&t1).Elem(), false)
			t2,ok := t1.(*toT1)
			if !ok {
				return false
			}
			return t2 != nil
		},
		func()bool{
			var t1 func()
			typeInit(reflect.ValueOf(&t1).Elem(), true, func([]reflect.Value)[]reflect.Value{
				return nil
			})
			return t1 != nil
		},
		func()bool{
			var t1 map[string]string
			typeInit(reflect.ValueOf(&t1).Elem(), true, 10)
			return t1 != nil
		},func()bool{
			var t1 chan bool
			typeInit(reflect.ValueOf(&t1).Elem(), true, 1)
			t1<-true
			select{
			case <-t1:
				return true
			default:
				return false
			}
		},
	}
	for index, fn := range fns {
		if !fn() {
			t.Fatalf("error in %d", index)
		}
	}
}

func Test_Convert(t *testing.T) {

	fns :=[]func()bool{
		func() bool{
			t1 := (*toT1)(nil)
			t2 := (*toT2)(nil)
			if !Convert(&t1, &t2) {return false}
			
			return t1 == nil && reflect.TypeOf(&t1).Elem().String() == "*builtin.toT1"
		},
		func() bool{
			t1 := &toT1{i:1}
			var t2 any = nil
			//nil 不可以转换到 struct
			//t1 没有改变
			if Convert(&t1, &t2) {return false}
			return t1.i == 1
		},
		func() bool{
			t1 := (*toT1)(nil)
			//nil 不可以转换,dst 依然是 (*toT1)(nil)
			if Convert(&t1, nil) {return false}
			return t1 == nil
		},
		func() bool{
			t1 := (*int)(nil)
			t2 := &toT2{i:2}
			//两个类型不一至，不可以转换
			if Convert(&t1, &t2) {return false}
			return t1 == nil
		},
		func() bool{
			t1 := (*toT1)(nil)
			t2 := toT2{i:2}
			//**t1 和 t2
			if !Convert(&t1, t2) {return false}
			return t1 != nil && t1.i == 2
		},
		func() bool{
			t1 := (*toT1)(nil)
			t2 := &toT2{i:2}
			//**t1 和 **t2
			if !Convert(&t1, t2) {return false}
			return t1 != nil && t1.i == 2
		},
		func() bool{
			var t1 toTI = (*toT1)(nil)
			t2 := &toT2{i:2}
			//将t2转为t1同接口
			if !Convert(&t1, t2) {return false}
			return t1.M() == 2
		},
		func() bool{
			var t1 toTI = &toT1{i:1}
			var t2 = (*toT2)(nil)
			//仅是将 toT2 和 nil 转到 toTI 接口
			if !Convert(&t1, &t2) {return false}
			_, ok := t1.(*toT2)
			return ok
		},
	}
	for index, fn := range fns {
		if !fn() {
			t.Fatalf("errot in %d", index)
		}
	}
}

func Test_Set(t *testing.T){
	var t1 *toT1
	Init(&t1)
	vi := Value("int")
	vi.Elem().Set(reflect.ValueOf(1))
	Set(t1, "T", vi.Interface())
	if *t1.T != 1 {
		t.Fatal("error")
	}
}