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

	// 3. 预分配 reflect.Value 切片
	var prepArgs []reflect.Value
	if isVariadic {
		fixed := numIn - 1
		prepArgs = make([]reflect.Value, numIn)
		for i := range fixed {
			v, err := convertArg(args[i], ft.In(i), i)
			if err != nil {
				return err
			}
			prepArgs[i] = v
		}

		// 可变参数：对齐 Go 原生语义
		//   - 零个可变参数 → nil 切片（reflect.Zero）
		//   - 至少一个可变参数 → MakeSlice 逐元素填充
		sliceType := ft.In(fixed)
		nVar := argLen - fixed
		if nVar == 0 {
			prepArgs[fixed] = reflect.Zero(sliceType)
		} else {
			elemType := sliceType.Elem()
			sliceVal := reflect.MakeSlice(sliceType, nVar, nVar)
			for i := range nVar {
				v, err := convertArg(args[fixed+i], elemType, fixed+i)
				if err != nil {
					return err
				}
				sliceVal.Index(i).Set(v)
			}
			prepArgs[fixed] = sliceVal
		}
	} else {
		prepArgs = make([]reflect.Value, argLen)
		for i := range argLen {
			v, err := convertArg(args[i], ft.In(i), i)
			if err != nil {
				return err
			}
			prepArgs[i] = v
		}
	}

	T.fun = fn
	T.arg = prepArgs
	return nil
}

// [FIX-Bug1] convertArg 抽出原 Func() 内的单参数类型转换逻辑，
// 便于固定参数与可变参数两条路径复用，避免逻辑重复。
func convertArg(a any, targetType reflect.Type, idx int) (reflect.Value, error) {
	if a == nil {
		return reflect.Zero(targetType), nil
	}
	argV := reflect.ValueOf(a)
	argT := argV.Type()

	if argT.AssignableTo(targetType) {
		return argV, nil
	}
	// 数字↔字符串的 reflect.Convert 语义是 rune，不是文本，显式拒绝。
	if argT.Kind() == reflect.String || targetType.Kind() == reflect.String {
		if argT.Kind() != targetType.Kind() {
			return reflect.Value{}, fmt.Errorf("vweb: arg[%d] 拒绝字符串与%s间的隐式转换", idx, argT.Kind())
		}
		return argV.Convert(targetType), nil
	}
	if argV.CanConvert(targetType) {
		return argV.Convert(targetType), nil
	}
	return reflect.Value{}, fmt.Errorf("vweb: arg[%d] type mismatch: cannot convert %s to %s", idx, argT, targetType)
}

// Exec 执行函数
func (T *ExecCall) Exec() []any {
	if !T.fun.IsValid() {
		return nil
	}

	ft := T.fun.Type()
	var rvs []reflect.Value
	if ft.IsVariadic() {
		rvs = T.fun.CallSlice(T.arg)
	} else {
		rvs = T.fun.Call(T.arg)
	}
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

// 注册回调。
// 参数 call: 函数；args: 参数。
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

// 执行队列中全部回调。
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
