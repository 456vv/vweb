package builtin

import (
	"reflect"
	"testing"
	"time"
)

func TestSendAndRecv(t *testing.T) {
	ch := Make("chan int", 1)
	Send(ch, 100)
	v, ok := Recv(ch)
	if !ok || v.(int) != 100 {
		t.Errorf("Recv failed: expected 100, true; got %v, %v", v, ok)
	}
}

func TestTrySendAndTryRecv(t *testing.T) {
	ch := Make("chan int", 1)
	if !TrySend(ch, 42) {
		t.Error("TrySend should succeed on buffered channel")
	}
	if TrySend(ch, 43) {
		t.Error("TrySend should fail on full channel")
	}
	v, ok := TryRecv(ch)
	if !ok || v.(int) != 42 {
		t.Errorf("TryRecv failed: expected 42, true; got %v, %v", v, ok)
	}
	v, ok = TryRecv(ch)
	if ok || v != nil {
		t.Errorf("TryRecv should fail on empty channel, got %v, %v", v, ok)
	}
}

func TestClose(t *testing.T) {
	ch := Make("chan int", 1)
	Send(ch, 99)
	Close(ch)
	v, ok := Recv(ch)
	if !ok || v.(int) != 99 {
		t.Errorf("Expected to read 99 from closed channel, got %v, %v", v, ok)
	}
	v, ok = Recv(ch)
	if ok {
		t.Errorf("Expected ok=false when reading empty closed channel, got %v", ok)
	}
}

func TestNilSend(t *testing.T) {
	ch := Make("chan interface", 1)
	Send(ch, nil)
	v, ok := Recv(ch)
	if !ok || v != nil {
		t.Errorf("Expected nil, true; got %v, %v", v, ok)
	}
	TrySend(ch, nil)
	v, ok = TryRecv(ch)
	if !ok || v != nil {
		t.Errorf("Expected nil, true; got %v, %v", v, ok)
	}
}

func TestNativeChannel(t *testing.T) {
	nativeCh := make(chan string, 1)
	Send(nativeCh, "hello")
	v, ok := Recv(nativeCh)
	if !ok || v.(string) != "hello" {
		t.Errorf("Expected 'hello' from native channel, got %v", v)
	}
	TrySend(nativeCh, "world")
	v, ok = TryRecv(nativeCh)
	if !ok || v.(string) != "world" {
		t.Errorf("Expected 'world' from native channel, got %v", v)
	}
	Close(nativeCh)
	_, ok = Recv(nativeCh)
	if ok {
		t.Error("Expected native channel to be closed")
	}
}

func TestGoroutineBlocking(t *testing.T) {
	ch := Make("chan int")
	go func() {
		time.Sleep(50 * time.Millisecond)
		Send(ch, 777)
	}()
	v, ok := Recv(ch)
	if !ok || v.(int) != 777 {
		t.Errorf("Expected 777, got %v", v)
	}
}

func TestTrySend_NilAndClosed(t *testing.T) {
	var nilCh chan int
	if TrySend(nilCh, 1) {
		t.Fatal("nil channel TrySend 应返回 false")
	}
	var nilChan *Chan
	if TrySend(nilChan, 1) {
		t.Fatal("nil *Chan TrySend 应返回 false")
	}
	ch := make(chan int, 1)
	close(ch)
	if TrySend(ch, 1) {
		t.Fatal("已关闭 channel TrySend 应返回 false（不 panic）")
	}
}

func TestTrySend_DirectionMismatch(t *testing.T) {
	ch := make(chan int, 1)
	var recv <-chan int = ch
	if TrySend(recv, 1) {
		t.Fatal("只接收 channel TrySend 应返回 false")
	}
	var send chan<- int = ch
	if !TrySend(send, 1) {
		t.Fatal("只发送 channel TrySend 应成功")
	}
}

func TestTryRecv_DirectionMismatch(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 1
	var send chan<- int = ch
	if _, ok := TryRecv(send); ok {
		t.Fatal("只发送 channel TryRecv 应返回 false")
	}
}

func TestTryRecv_ClosedChannel(t *testing.T) {
	ch := make(chan int, 1)
	close(ch)
	v, ok := TryRecv(ch)
	if ok {
		t.Fatal("已关闭 channel TryRecv 应返回 false")
	}
	if v != 0 {
		t.Fatalf("已关闭 channel 应返回零值，得到 %v", v)
	}
}

func TestRecv_ClosedChannel_ReturnsZeroValue(t *testing.T) {
	ch := make(chan int, 1)
	close(ch)
	v, ok := Recv(ch)
	if ok || v != 0 {
		t.Fatalf("ok=%v v=%v", ok, v)
	}
}

func TestSend_NilValuePanic(t *testing.T) {
	ch := make(chan int, 1)
	defer func() {
		if recover() == nil {
			t.Fatal("chan int 发送 nil 应 panic")
		}
	}()
	Send(ch, nil)
}

func TestSend_ClosedChannelPanic(t *testing.T) {
	ch := make(chan int, 1)
	close(ch)
	defer func() {
		if recover() == nil {
			t.Fatal("已关闭 channel 发送应 panic")
		}
	}()
	Send(ch, 1)
}

func TestClose_NilChannelPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("close nil channel 应 panic")
		}
	}()
	Close((chan int)(nil))
}

func TestClose_RecvOnlyChannelPanic(t *testing.T) {
	ch := make(chan int, 1)
	var recv <-chan int = ch
	defer func() {
		if recover() == nil {
			t.Fatal("close 只接收 channel 应 panic")
		}
	}()
	Close(recv)
}

func TestSend_NonChannelPanic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Send 非 channel 应 panic")
		}
	}()
	Send(123, 1)
}

func TestChanWrapper_Value(t *testing.T) {
	ch := make(chan int, 1)
	w := Chan{C: reflect.ValueOf(ch)}
	Send(w, 1)
	v, ok := Recv(&w)
	if !ok || v.(int) != 1 {
		t.Fatalf("Chan 包装读写失败：%v %v", v, ok)
	}
}

// TestChan_Boundary 非阻塞方向 / nil / 已关闭 / panic 语义。
func TestChan_Boundary(t *testing.T) {
	t.Run("TrySend 系列必须返回 false 而非 panic", func(t *testing.T) {
		var nilCh chan int
		var nilPC *Chan
		closed := make(chan int, 1)
		close(closed)
		recvOnly := func() <-chan int { c := make(chan int, 1); return c }()
		cases := []struct {
			name string
			ch   any
		}{
			{"nil channel", nilCh},
			{"nil *Chan", nilPC},
			{"已关闭", closed},
			{"只收通道", recvOnly},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if TrySend(c.ch, 1) {
					t.Fatal("应返回 false")
				}
			})
		}
	})

	t.Run("TryRecv 只发通道", func(t *testing.T) {
		ch := make(chan int, 1)
		ch <- 1
		var send chan<- int = ch
		if _, ok := TryRecv(send); ok {
			t.Fatal("只发通道应返回 false")
		}
	})

	t.Run("Send/Close 参数错误必须 panic", func(t *testing.T) {
		mustPanic := func(name string, fn func()) {
			t.Run(name, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Error("应 panic")
					}
				}()
				fn()
			})
		}
		mustPanic("chan int 发 nil", func() { Send(make(chan int, 1), nil) })
		mustPanic("已关闭发送", func() { c := make(chan int, 1); close(c); Send(c, 1) })
		mustPanic("非 channel", func() { Send(123, 1) })
		mustPanic("close nil", func() { Close((chan int)(nil)) })
		mustPanic("close 只收", func() { c := make(chan int, 1); var r <-chan int = c; Close(r) })
	})
}

// TestTrySend_ConversionError 值转换失败必须返回 false 且不发送。
// 覆盖 getSendValue -> autoConvert 失败路径。
func TestTrySend_ConversionError(t *testing.T) {
	ch := make(chan int, 1)
	if TrySend(ch, "not-an-int") {
		t.Fatal("字符串转 int 失败应返回 false")
	}
	select {
	case v := <-ch:
		t.Fatalf("失败时不应发送：%v", v)
	default:
	}
}

// TestTryRecv_ZeroValueVsEmpty 区分「值 0」与「空 channel」。
// TryRecv 返回 (nil, false) 与 (0, true) 不能互相混淆。
func TestTryRecv_ZeroValueVsEmpty(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 0
	v, ok := TryRecv(ch)
	if !ok || v != 0 {
		t.Fatalf("值 0 应能正确接收：v=%v ok=%v", v, ok)
	}
	v, ok = TryRecv(ch)
	if ok || v != nil {
		t.Fatalf("空 channel 应返回 (nil, false)：v=%v ok=%v", v, ok)
	}
}

// TestTryRecv_ClosedWithBuffered 关闭后仍能读取缓冲剩余值（Go 原生语义）。
func TestTryRecv_ClosedWithBuffered(t *testing.T) {
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2
	close(ch)
	for i := 1; i <= 2; i++ {
		v, ok := TryRecv(ch)
		if !ok || v.(int) != i {
			t.Fatalf("应读取剩余 %d，得到 v=%v ok=%v", i, v, ok)
		}
	}
	v, ok := TryRecv(ch)
	if ok {
		t.Fatal("关闭且耗尽后 ok 应为 false")
	}
	if v != 0 {
		t.Fatalf("耗尽后应返回零值 0，得到 %v", v)
	}
}

// TestChan_InterfaceNilSend 接口 chan 可发送 nil，Send/Recv/TrySend/TryRecv 全链路正常。
func TestChan_InterfaceNilSend(t *testing.T) {
	ch := make(chan any, 1)
	Send(ch, nil)
	v, ok := Recv(ch)
	if !ok || v != nil {
		t.Fatalf("接口 chan 应能接收 nil：v=%v ok=%v", v, ok)
	}
	if !TrySend(ch, nil) {
		t.Fatal("接口 chan TrySend nil 应成功")
	}
	v, ok = TryRecv(ch)
	if !ok || v != nil {
		t.Fatalf("接口 chan TryRecv nil 应成功：v=%v ok=%v", v, ok)
	}
}

// TestChan_WrongElemType TrySend 返回 false；Send 必须 panic（不可永久阻塞）。
func TestChan_WrongElemType(t *testing.T) {
	ch := make(chan int, 1)
	if TrySend(ch, "x") {
		t.Fatal("错误类型 TrySend 应返回 false")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r == nil {
				t.Error("Send 错误类型应 panic")
			}
		}()
		Send(ch, "x")
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Send 应快速 panic")
	}
}

// TestChan_WrapperPointerAndValue Chan 值和指针包装均可用。
func TestChan_WrapperPointerAndValue(t *testing.T) {
	ch := make(chan int, 2)
	v := Chan{C: reflect.ValueOf(ch)}
	p := &v

	Send(v, 1)
	Send(p, 2)

	got1, ok1 := Recv(v)
	got2, ok2 := Recv(p)
	if !ok1 || !ok2 || got1.(int) != 1 || got2.(int) != 2 {
		t.Fatalf("Chan 值和指针包装读写：%v/%v %v/%v", got1, ok1, got2, ok2)
	}
}
