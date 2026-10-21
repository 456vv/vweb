package builtin

import (
	"fmt"
	"reflect"
)

type Chan struct {
	C reflect.Value
}

// 辅助函数：统一提取 channel 的 reflect.Value
func getChanValue(a any) (reflect.Value, bool) {
	if p, ok := a.(*Chan); ok {
		if p == nil {
			return reflect.Value{}, false
		}
		return inDirect(p.C), true
	}
	if c, ok := a.(Chan); ok {
		// 支持值型 Chan 包装。
		return inDirect(c.C), true
	}
	rv := asValue(a)
	// 支持 *chan T、interface 包装的 channel。
	rv = inDirect(rv)
	if rv.IsValid() && rv.Kind() == reflect.Chan {
		return rv, true
	}
	return reflect.Value{}, false
}

// 辅助函数：处理发送时的 value，特别是 nil 值的处理
func getSendValue(ch reflect.Value, v any) (reflect.Value, error) {
	if !ch.IsValid() || ch.Kind() != reflect.Chan {
		return reflect.Value{}, fmt.Errorf("invalid channel")
	}
	elem := ch.Type().Elem()
	// autoConvert 把 nil 变成零值，chan int 会收到 0 而不是失败。
	if v == nil {
		switch elem.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			return reflect.Zero(elem), nil
		default:
			return reflect.Value{}, fmt.Errorf("cannot send nil to chan %s", elem)
		}
	}
	return autoConvert(elem, v)
}

// 不阻塞
// TrySend(*Chan/reflect.Value/chan T, value)
func TrySend(a any, v any) (ok bool) {
	ch, valid := getChanValue(a)
	if !valid || !ch.IsValid() || ch.Kind() != reflect.Chan || ch.IsNil() {
		return false
	}
	// 只接收 channel 上 TrySend 会 panic，非阻塞 API 应返回 false。
	if ch.Type().ChanDir() == reflect.RecvDir {
		return false
	}
	sv, err := getSendValue(ch, v)
	if err != nil {
		return false
	}
	// 向已关闭 channel TrySend 会 panic，非阻塞语义下应返回 false。
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return ch.TrySend(sv)
}

// 不阻塞
// TryRecv 返回 (值, 是否成功接收)
// ok == false 说明 channel 为空或已关闭
func TryRecv(a any) (val any, ok bool) {
	ch, valid := getChanValue(a)
	if !valid || !ch.IsValid() || ch.Kind() != reflect.Chan || ch.IsNil() {
		return nil, false
	}
	// 只发送 channel 的 TryRecv 会 panic；与 TrySend 对称必须预检+recover。
	if ch.Type().ChanDir() == reflect.SendDir {
		return nil, false
	}
	defer func() {
		if recover() != nil {
			val, ok = nil, false
		}
	}()
	vr, recvOk := ch.TryRecv()
	// 空通道得到 invalid；已关闭通道得到元素零值且 recvOk=false。
	if !vr.IsValid() {
		return nil, false
	}
	return valueToInterface(vr), recvOk
}

// 阻塞发送
// Send(*Chan/reflect.Value/chan T, value)
func Send(a any, v any) {
	ch, ok := getChanValue(a)
	if !ok || !ch.IsValid() || ch.Kind() != reflect.Chan {
		panic("Send: expected a channel")
	}
	// 提前给出方向错误，避免 reflect 内部 panic 信息含糊。
	if ch.Type().ChanDir() == reflect.RecvDir {
		panic("Send: receive-only channel")
	}
	sv, err := getSendValue(ch, v)
	if err != nil {
		panic("Send: " + err.Error())
	}
	ch.Send(sv)
}

// 阻塞接收
// Recv 返回 (值, 通道是否未关闭)
// ok == false 说明 channel 已关闭
func Recv(a any) (any, bool) {
	ch, ok := getChanValue(a)
	if !ok || !ch.IsValid() || ch.Kind() != reflect.Chan {
		return nil, false
	}
	// 只发送 channel 无法接收，返回 (nil,false) 而不是 panic。
	if ch.Type().ChanDir() == reflect.SendDir {
		return nil, false
	}
	vr, recvOk := ch.Recv()
	// 关闭通道时应返回元素零值 + ok=false，而不是一律 (nil,false)。
	if !vr.IsValid() {
		return nil, recvOk
	}
	return valueToInterface(vr), recvOk
}

// 关闭通道
// Close(*Chan/reflect.Value/chan T)
func Close(a any) {
	ch, ok := getChanValue(a)
	if !ok || !ch.IsValid() || ch.Kind() != reflect.Chan {
		panic("Close: expected a channel")
	}
	if ch.IsNil() {
		panic("close of nil channel")
	}
	// 只接收 channel 不能 Close，给出明确错误。
	if ch.Type().ChanDir() == reflect.RecvDir {
		panic("Close: receive-only channel")
	}
	ch.Close()
}
