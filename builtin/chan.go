package builtin

import (
	"reflect"
)

type Chan struct {
	C reflect.Value
}

// 辅助函数：统一提取 channel 的 reflect.Value
func getChanValue(a any) (reflect.Value, bool) {
	if p, ok := a.(*Chan); ok {
		return p.C, true
	} else if rv, ok := a.(reflect.Value); ok && rv.Kind() == reflect.Chan {
		return rv, true
	} else {
		// 支持原生 Go channel (例如: make(chan int))
		rv := reflect.ValueOf(a)
		if rv.IsValid() && rv.Kind() == reflect.Chan {
			return rv, true
		}
	}
	return reflect.Value{}, false
}

// 辅助函数：处理发送时的 value，特别是 nil 值的处理
func getSendValue(ch reflect.Value, v any) reflect.Value {
	if v == nil {
		// 如果发送的是 nil，则生成 channel 元素类型的零值
		return reflect.Zero(ch.Type().Elem())
	}
	return reflect.ValueOf(v)
}

// 不阻塞
// TrySend(*Chan/reflect.Value/chan T, value)
func TrySend(a any, v any) bool {
	ch, ok := getChanValue(a)
	if !ok {
		return false
	}
	return ch.TrySend(getSendValue(ch, v))
}

// 不阻塞
// TryRecv 返回 (值, 是否成功接收)
// ok == false 说明 channel 为空或已关闭
func TryRecv(a any) (any, bool) {
	ch, ok := getChanValue(a)
	if !ok {
		return nil, false
	}
	vr, recvOk := ch.TryRecv()
	if recvOk && vr.IsValid() && vr.CanInterface() {
		return vr.Interface(), true
	}
	return nil, false
}

// 阻塞发送
// Send(*Chan/reflect.Value/chan T, value)
func Send(a any, v any) {
	ch, ok := getChanValue(a)
	if !ok {
		panic("Send: expected a channel")
	}
	ch.Send(getSendValue(ch, v))
}

// 阻塞接收
// Recv 返回 (值, 通道是否未关闭)
// ok == false 说明 channel 已关闭
func Recv(a any) (any, bool) {
	ch, ok := getChanValue(a)
	if !ok {
		return nil, false
	}
	vr, recvOk := ch.Recv()
	if recvOk && vr.IsValid() && vr.CanInterface() {
		return vr.Interface(), true
	}
	return nil, false
}

// 关闭通道
// Close(*Chan/reflect.Value/chan T)
func Close(a any) {
	ch, ok := getChanValue(a)
	if !ok {
		panic("Close: expected a channel")
	}
	ch.Close()
}
