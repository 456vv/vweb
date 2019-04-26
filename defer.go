package vweb

import(
    "reflect"
    "fmt"
)

//deferFunc 过期函数
type deferFunc struct {
	fun         reflect.Value                                                               // 函数
    arg         []reflect.Value                                                             // 参数
    argVariadic bool                                                                        // 有可变参数
}

type exitCall struct {
    expCall     []*deferFunc                                                                // 记录每个用户的函数，会话超时后关闭打开的对象
}

// Defer 在用户会话时间过期后，将被调用。
//	call interface{}            函数
//	args ... interface{}        参数或更多个函数是函数的参数
//	error                       错误
//  例：
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (s *exitCall) Defer(call interface{}, args ... interface{}) error {
    var (
        df          = new(deferFunc)
        dfarg       reflect.Value

        fn          reflect.Value = reflect.ValueOf(call)
        ft          reflect.Type

        fnInLen     int
        argLen      int = len(args)
        argIndex    reflect.Type
        variadic    bool
    )

    fvdirect := InDirect(fn)
    if fvdirect.Kind() != reflect.Func {
        return fmt.Errorf("vweb: 第一个参数不是有效的func，错误的func类型为 %s。", fvdirect.Kind())
    }

    ft 			= fvdirect.Type()
    fnInLen 	= ft.NumIn()
    variadic 	= ft.IsVariadic()
    fnargLen 	:= fnInLen - argLen
    if (!variadic && fnInLen != argLen) ||
        variadic && fnInLen > argLen && fnargLen != 1 {
    	return fmt.Errorf("vweb: 传入的参数长度与调用函数参数不符合。调用函数参数长度为（%d）,传入参数长度为（%d）。", fnInLen, argLen)
    }

    fil := fnInLen-1
    for index, arg := range  args {
    	argv := reflect.ValueOf(arg)

        var typeErr bool
        if index <= fil {
            argIndex =  ft.In(index)
            if argIndex.Kind() == reflect.Interface || argIndex.Kind() == argv.Kind() && argv.Type().ConvertibleTo(argIndex) {
                if index == fil && argLen != fnInLen {
                	return fmt.Errorf("vweb: 传入的参数数量超过了调用函数支持的数量。调用函数参数数量为（%d），传入参数数量为（%d）",  fnInLen, argLen)
                }
            	df.arg = append(df.arg, argv)
                continue
            }else{
                // 在类型不配置情况下，可能是可变参数。只是可能！如何处理？
                // 1，当前位置 != 调用参数最后一位置 = 错误
                // 2，当前位置也是调用参数最后一位置 != reflect.Slice = 错误
                // 3，上面都匹配，可是这个函数不带有可变参数 = 错误
                if index != fil || argIndex.Kind() != reflect.Slice || !variadic {
                	typeErr = true
                }else{
                	dfarg = reflect.MakeSlice(argIndex, 0, 0)
                }
            }
        }

        aik := argIndex.Kind()
        avk := argv.Kind()

        if !typeErr {
            aik = argIndex.Elem().Kind()
            if aik == avk || aik == reflect.Interface {
                dfarg = reflect.Append(dfarg, argv)
            }else{
            	typeErr = true
            }
        }
        if typeErr {
        	return fmt.Errorf("vweb: 传入参数类型与调用函数参数类型不符，第(%d)个参数，调用函数参数类型为（%s），传入参数类型为（%s）。", index+1, aik, avk)
        }
    }

    if dfarg.Kind() != reflect.Invalid {
        df.arg = append(df.arg, dfarg)
    }

    df.fun = fn
    df.argVariadic = variadic
    s.expCall = append(s.expCall, df)
    return nil
}

//Free 执行结束Defer
func (s *exitCall) Free() {
	for _, execCall := range s.expCall {
		if execCall.argVariadic {
			execCall.fun.CallSlice(execCall.arg)
		}else{
		    execCall.fun.Call(execCall.arg)
	 	}
	}
	s.expCall = nil
}