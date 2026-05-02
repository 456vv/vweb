package builtin

import (
	"reflect"
	"testing"
	"time"
)

func TestMakeChan(t *testing.T) {
	// 测试无缓冲
	c1 := MakeChan("int")
	if c1.Data.Type().Elem().Kind() != reflect.Int {
		t.Errorf("Expected chan int, got %v", c1.Data.Type())
	}

	// 测试有缓冲，兼容不同整型
	c2 := MakeChan("string", 10)
	if c2.Data.Cap() != 10 {
		t.Errorf("Expected cap 10, got %d", c2.Data.Cap())
	}

	c3 := MakeChan("string", int64(5)) // 测试非 int 类型的 buffer
	if c3.Data.Cap() != 5 {
		t.Errorf("Expected cap 5, got %d", c3.Data.Cap())
	}
}

func TestSendAndRecv(t *testing.T) {
	ch := MakeChan("int", 1)

	// 测试阻塞发送 (其实有缓冲不阻塞)
	Send(ch, 100)

	// 测试阻塞接收
	v, ok := Recv(ch)
	if !ok || v.(int) != 100 {
		t.Errorf("Recv failed: expected 100, true; got %v, %v", v, ok)
	}
}

func TestTrySendAndTryRecv(t *testing.T) {
	ch := MakeChan("int", 1)

	// 测试非阻塞发送
	ok := TrySend(ch, 42)
	if !ok {
		t.Error("TrySend should succeed on buffered channel")
	}

	// 缓冲区已满，非阻塞发送应失败
	ok = TrySend(ch, 43)
	if ok {
		t.Error("TrySend should fail on full channel")
	}

	// 测试非阻塞接收
	v, ok := TryRecv(ch)
	if !ok || v.(int) != 42 {
		t.Errorf("TryRecv failed: expected 42, true; got %v, %v", v, ok)
	}

	// 缓冲区已空，非阻塞接收应失败
	v, ok = TryRecv(ch)
	if ok || v != nil {
		t.Errorf("TryRecv should fail on empty channel, got %v, %v", v, ok)
	}
}

func TestClose(t *testing.T) {
	ch := MakeChan("int", 1)
	Send(ch, 99)
	Close(ch)

	// 关闭后仍可读出缓冲的数据
	v, ok := Recv(ch)
	if !ok || v.(int) != 99 {
		t.Errorf("Expected to read 99 from closed channel, got %v, %v", v, ok)
	}

	// 读空后应返回 false
	v, ok = Recv(ch)
	if ok {
		t.Errorf("Expected ok=false when reading empty closed channel, got %v", ok)
	}
}

func TestNilSend(t *testing.T) {
	// 测试发送 nil 给 chan any
	ch := MakeChan("interface", 1) // 假设 "interface" 被解析为 any

	// 这是一个盲区修复的测试：原来这里会 panic
	Send(ch, nil)

	v, ok := Recv(ch)
	if !ok || v != nil {
		t.Errorf("Expected nil, true; got %v, %v", v, ok)
	}

	// 测试非阻塞发送 nil
	TrySend(ch, nil)
	v, ok = TryRecv(ch)
	if !ok || v != nil {
		t.Errorf("Expected nil, true; got %v, %v", v, ok)
	}
}

func TestNativeChannel(t *testing.T) {
	// 测试原生 Go channel (另一个盲区修复)
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
	ch := MakeChan("int") // 无缓冲

	go func() {
		time.Sleep(50 * time.Millisecond)
		Send(ch, 777)
	}()

	// 此时 Recv 应该阻塞等待
	v, ok := Recv(ch)
	if !ok || v.(int) != 777 {
		t.Errorf("Expected 777, got %v", v)
	}
}
