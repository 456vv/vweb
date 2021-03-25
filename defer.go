package vweb

import(
    "reflect"
    "github.com/456vv/verror"
)

//execFunc 执行函数
type execFunc struct {
	fun         reflect.Value                                                               // 函数
    arg         []reflect.Value                                                             // 参数
    argVariadic bool                                                                        // 有可变参数
}
func (T *execFunc) add(call interface{}, args ... interface{}) error {
    var (
		
        fn          reflect.Value
        ft          reflect.Type
		
        fnInLen     int
        argLen      int = len(args)
        variadic    bool
    )
    if f, ok := call.(reflect.Value); ok {
	 	fn = f
    }else{
    	fn = reflect.ValueOf(call)
    }
    
    fvdirect := inDirect(fn)
    if fvdirect.Kind() != reflect.Func {
        return verror.TrackErrorf("vweb: 第一个参数不是有效的func，错误的func类型为 %s。", fvdirect.Kind())
    }
    if fvdirect.IsNil() {
    	return verror.TrackErrorf("vweb: 该函数 %s 还没被初始化，不可以使用！", fn.Type().Name())
    }
	
    ft 			= fvdirect.Type()
    fnInLen 	= ft.NumIn()
    variadic 	= ft.IsVariadic()
    fnargLen 	:= fnInLen - argLen
    if (!variadic && fnInLen != argLen) ||
        variadic && fnargLen != 1 && fnInLen > argLen {
    	return verror.TrackErrorf("vweb: 传入的参数长度与调用函数参数不符合。调用函数参数长度为（%d）,传入参数长度为（%d）。", fnInLen, argLen)
    }
	
	fnInLen = fnInLen-1			//函数参数-长度
    var argIndex reflect.Type 	//函数参数-类型
    var varArgs reflect.Value		//创建一上存放可变参数slice
    var typeErr bool
    for index, arg := range args {
    	argv := reflect.ValueOf(&arg).Elem().Elem()
    	
        //限制参数数量
        if index <= fnInLen {
        	argIndex =  ft.In(index)
        	//防止无类型nil参数
        	if argv.Kind() == reflect.Invalid {
        		argv = reflect.New(argIndex).Elem()
        	}

        	
			//1，函数参数是接口
			//2，类型相等
			//3，类型可以转换
    		if argIndex.Kind() == reflect.Interface || argIndex.Kind() == argv.Kind() && argv.Type().ConvertibleTo(argIndex) {
				//适用func(a interface{}, b ...interface{}) => call(interface{}, []interface{})
        		T.arg = append(T.arg, argv)//argv.Elem() 是将参数 interface{} 转为 原类型
            	continue
    		}
    		
			//最后一个是切片
			if index == fnInLen && variadic && (argIndex.Elem().Kind() == reflect.Interface || argv.Type().ConvertibleTo(argIndex.Elem())) {
				//适用func(a interface{}, b ...interface{}) => call(interface{}, interface{})
				varArgs = reflect.MakeSlice(argIndex, 0, 0)
				varArgs = reflect.Append(varArgs, argv)
				continue
    		}
    		
    		//参数类型不匹配
    		typeErr = true
        }
        
        //可变参数+1...
        if !typeErr {
        	if varArgs.Kind() != reflect.Invalid {
	        	if argIndex.Elem().Kind() == reflect.Interface || (argIndex.Elem().Kind() == argv.Kind() && argv.Type().ConvertibleTo(argIndex.Elem())) {
		        		//适用func(a interface{}, b ...interface{}) => call(interface{}, interface{}, interface{})
		         		varArgs = reflect.Append(varArgs, argv)
	         			continue
	        	}
        	}
        	typeErr = true
        }
	    if typeErr {
	    	return verror.TrackErrorf("vweb: 传入参数类型与调用函数参数类型不符，第(%d)个参数，函数参数类型为（%s），传入类型为（%s）。", index+1, argIndex.Kind(), argv.Kind())
	    }
    }
    //调用没有传入可变参数
    if variadic {
    	//1，函数参数-输入参数=1，表示没有设置可变参数
    	//2，判断 varArgs 上面没有初始化，否则创建一个空的可变参数
    	if (ft.NumIn()-argLen) == 1 && varArgs.Kind() == reflect.Invalid {
    		varArgs = reflect.MakeSlice(ft.In(fnInLen), 0, 0)
    	}
    	//1，仅对有效 varArgs 追加
    	if varArgs.Kind() != reflect.Invalid {
   			T.arg = append(T.arg, varArgs)
    	}
    }

    T.fun = fn
    T.argVariadic = variadic
    return nil
}
func (T *execFunc) exec() (ret []interface{}) {
	var rvs []reflect.Value
	if T.argVariadic {
		rvs = T.fun.CallSlice(T.arg)
	}else{
	    rvs = T.fun.Call(T.arg)
 	}
 	if len(rvs) == 0 {
 		return nil
 	}
 	for _, rv := range rvs {
		ret = append(ret, typeSelect(rv))
 	}
 	return
}

//ExitCall 过期函数
type ExitCall struct {
    // 记录每个用户的函数，会话超时后关闭打开的对象
    efs     []*execFunc
}

// Defer 在用户会话时间过期后，将被调用。
//	call interface{}            函数
//	args ... interface{}        参数或更多个函数是函数的参数
//	error                       错误
//  例：
//	.Defer(fmt.Println, "1", "2")
//	.Defer(fmt.Printf, "%s", "汉字")
func (T *ExitCall) Defer(call interface{}, args ... interface{}) error {
	df := new(execFunc)
	if err := df.add(call, args...); err != nil {
		return err
	}
    T.efs = append(T.efs, df)
    return nil
}


//Free 执行结束Defer
func (T *ExitCall) Free() {
	for _, ef := range T.efs {
	 	ef.exec()
	}
	T.efs = nil
}