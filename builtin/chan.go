package builtin
	
import(
	"reflect"
)
type Chan struct{
	Data reflect.Value
}

//不阻塞
//TrySend(*Chan, value)
func TrySend(a interface{}, v interface{}) bool {
	if v == nil {
		panic("can't nil value to a channel")
	}
	if p, ok := a.(*Chan); ok {
		return p.Data.TrySend(reflect.ValueOf(v))
	}else if rv, ok := a.(reflect.Value); ok && rv.Kind() == reflect.Chan {
		return rv.TrySend(reflect.ValueOf(v))
	}
	return false
}

//不阻塞
//TryRecv(*Chan)
func TryRecv(a interface{}) interface{} {
	var v reflect.Value
	if p, ok := a.(*Chan); ok {
		v = p.Data
	}else if rv, ok := a.(reflect.Value); ok && rv.Kind() == reflect.Chan {
		v = rv
	}else{
		return nil
	}
	vr, _ := v.TryRecv()
	if vr.IsValid() {
		return vr.Interface()
	}
	return nil
}

//Send(*Chan, value)
func Send(a interface{}, v interface{}) {
	if v == nil {
		panic("can't nil value to a channel")
	}
	if p, ok := a.(*Chan); ok {
		p.Data.Send(reflect.ValueOf(v))
	}else if rv, ok := a.(reflect.Value); ok && rv.Kind() == reflect.Chan {
		rv.Send(reflect.ValueOf(v))
	}
}

//Recv(*Chan)
func Recv(a interface{}) interface{} {
	
	var v reflect.Value
	if p, ok := a.(*Chan); ok {
		v = p.Data
	}else if rv, ok := a.(reflect.Value); ok && rv.Kind() == reflect.Chan {
		v = rv
	}else{
		return nil
	}
	vr, ok := v.Recv()
	if ok {
		return vr.Interface()
	}
	return nil
}

//Close(*Chan)
func Close(a interface{}) {
	
	if p, ok := a.(*Chan); ok {
		p.Data.Close()
	}else if rv, ok := a.(reflect.Value); ok && rv.Kind() == reflect.Chan {
		rv.Close()
	}
}

//ChanOf(T)
func ChanOf(typ interface{}) reflect.Type {
	return reflect.ChanOf(reflect.BothDir, builtinType(typ))
}

//MakeChan(T, size)
func MakeChan(typ interface{}, buffer ...interface{}) *Chan {
	n := 0
	if len(buffer) > 0 {
		n = buffer[0].(int)
	}
	t := ChanOf(typ)
	return &Chan{Data: reflect.MakeChan(t, n)}
}
