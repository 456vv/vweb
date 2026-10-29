package vweb

import (
	"fmt"
	"reflect"
	"sync"
)

// ExecCall 执行函数包装器
type ExecCall struct {
	fun reflect.Value
	arg []reflect.Value
}

// Func 初始化函数和参数
func (T *ExecCall) Func(call any, args ...any) error {
	var fn reflect.Value
	if v, ok := call.(reflect.Value); ok {
		fn = v
	} else {
		fn = reflect.ValueOf(call)
	}

	// 1. 快速检查：是否为函数且非空
	if fn.IsValid() && fn.Kind() == reflect.Pointer {
		if fn.IsNil() {
			// 区分"传了 nil 指针"和"传了非函数类型"。
			return fmt.Errorf("vweb: function pointer is nil")
		}
		fn = fn.Elem()
	}
	if !fn.IsValid() {
		return fmt.Errorf("vweb: call parameter is invalid reflect.Value")
	}
	if fn.Kind() != reflect.Func {
		return fmt.Errorf("vweb: call parameter is not a func, got %s (only single-level pointer to func is supported)", fn.Kind())
	}
	if fn.IsNil() {
		return fmt.Errorf("vweb: function is nil")
	}

	ft := fn.Type()
	numIn := ft.NumIn()
	isVariadic := ft.IsVariadic()
	argLen := len(args)

	// 2. 参数长度校验
	if isVariadic {
		if argLen < numIn-1 {
			return fmt.Errorf("vweb: not enough arguments, need at least %d, got %d", numIn-1, argLen)
		}
	} else {
		if argLen != numIn {
			return fmt.Errorf("vweb: argument count mismatch, need %d, got %d", numIn, argLen)
		}
	}

	// 3. 预分配 reflect.Value 切片，减少多次 append 的内存分配
	// 我们统一使用 Call() 而不是 CallSlice()，这样逻辑更清晰，性能差异微乎其微
	prepArgs := make([]reflect.Value, argLen)

	for i := range argLen {
		var targetType reflect.Type
		if isVariadic && i >= numIn-1 {
			targetType = ft.In(numIn - 1).Elem() // 可变参数的元素类型
		} else {
			targetType = ft.In(i)
		}

		if args[i] == nil {
			// 处理 nil 参数
			prepArgs[i] = reflect.Zero(targetType)
		} else {
			argV := reflect.ValueOf(args[i])
			argT := argV.Type()

			// 类型检查与转换
			if argT.AssignableTo(targetType) {
				prepArgs[i] = argV
			} else if argT.ConvertibleTo(targetType) {
				prepArgs[i] = argV.Convert(targetType)
			} else {
				return fmt.Errorf("vweb: arg[%d] type mismatch: cannot convert %s to %s", i, argT, targetType)
			}
		}
	}

	T.fun = fn
	T.arg = prepArgs
	return nil
}

// Exec 执行函数
func (T *ExecCall) Exec() []any {
	if !T.fun.IsValid() {
		return nil
	}

	// 无论是否是可变参数函数，只要参数已经展开平铺，都可以直接用 Call
	rvs := T.fun.Call(T.arg)
	if len(rvs) == 0 {
		return nil
	}
	ret := make([]any, len(rvs))
	for i, rv := range rvs {
		ret[i] = rv.Interface()
	}
	return ret
}

// ExitCall 任务管理
type ExitCall struct {
	efs    []*ExecCall
	m      sync.Mutex
	closed bool
}

// 使用对象池减少 ExecCall 频繁创建销毁的开销
var execCallPool = sync.Pool{
	New: func() any {
		return &ExecCall{}
	},
}

func (T *ExitCall) Defer(call any, args ...any) error {
	// 从池中获取对象
	df := execCallPool.Get().(*ExecCall)
	if err := df.Func(call, args...); err != nil {
		df.arg = nil
		df.fun = reflect.Value{}
		execCallPool.Put(df)
		return err
	}

	// 持锁二次校验 closed，避免与 Free 竞争导致 Defer 泄漏。
	T.m.Lock()
	if T.closed {
		T.m.Unlock()
		df.arg = nil
		df.fun = reflect.Value{}
		execCallPool.Put(df)
		return fmt.Errorf("vweb: exit call is already freed")
	}
	T.efs = append(T.efs, df)
	T.m.Unlock()
	return nil
}

func (T *ExitCall) Free() {
	T.m.Lock()
	if T.closed {
		T.m.Unlock()
		return
	}
	T.closed = true
	calls := T.efs
	T.efs = nil // 尽早释放引用
	T.m.Unlock()

	// 倒序执行
	for i := len(calls) - 1; i >= 0; i-- {
		// 单个 Defer 函数 panic 不应中断后续 Defer 执行，
		// 且必须保证 ExecCall 放回池中，否则对象池持续泄露。
		func(c *ExecCall) {
			defer func() {
				if r := recover(); r != nil {
					// 恢复执行，避免因用户回调 panic 中断清理流程
				}
				c.arg = nil
				c.fun = reflect.Value{}
				execCallPool.Put(c)
			}()
			c.Exec()
		}(calls[i])
	}
}
