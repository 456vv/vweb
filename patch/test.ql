//qlang

//    PKG(pkg string) map[string]interface{}                                                  // 调用包函数
//    Request() *http.Request                                                                 // 用户的请求信息
//    RequestLimitSize(l int64) *http.Request                                                 // 请求限制大小
//    Header() http.Header                                                                    // 标头
//    Response() Responser                                                                    // 数据写入响应
//    ResponseWriter() http.ResponseWriter                                                    // 数据写入响应
//    Session() Sessioner                                                                     // 用户的会话缓存
//    Global() Globaler                                                                       // 全站缓存
//    Cookie() Cookier                                                                        // 用户的Cookie
//    Swap() Swaper                                                                           // 信息交换
//    PluginRPC(name string) (PluginRPC, error)                                               // 插件RPC方法调用
//    PluginHTTP(name string) (PluginHTTP, error)                                             // 插件HTTP方法调用
//    Config() ConfigSite																	  // 网站配置

	os = R.PKG("os")
	writer,reader,error  = os.Pipe()
	fprintln(W,error)
	fprintln(W,reader.Name)
	fprintln(W,writer.Name)

    W.WriteString("\n\n")

	reflect = R.PKG("reflect")
	reflectValue = reflect.ValueOf(R.Request())
	fprintln(W,reflectValue)

	reflectValue = reflectValue.Elem()
	method = reflectValue.FieldByName("Method")
	method.SetString("POST")
	fprintln(W,method)


	a = 123
	b = ToPtr(a)
	b1 = ToPtr(b)
	b2 = ToPtr(b1)
	fprintln(W,a)
	fprintln(W,PtrTo(b))
	fprintln(W,PtrTo(PtrTo(b1)))
	fprintln(W,InDirect(reflect.ValueOf(b)))

    W.WriteString("\n\n")
	fprintln(W,R.Request())
    W.WriteString("\n\n")
	fprintln(W,R.Header())
    W.WriteString("\n\n")
	fprintln(W,R.Response())
    W.WriteString("\n\n")
	fprintln(W,R.Session())
    W.WriteString("\n\n")
	fprintln(W,R.Global())
    W.WriteString("\n\n")
	fprintln(W,R.Cookie())
    W.WriteString("\n\n")
	fprintln(W,R.Global())
    W.WriteString("\n\n")
	fprintln(W,R.Swap())
    W.WriteString("\n\n")
	fprintln(W, R.Config())
