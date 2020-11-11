package vweb

import (
    "fmt"
    "time"
    "net/http"
    "github.com/456vv/vmap/v2"
    "github.com/456vv/verror"
)

type manageSession struct{
	s		Sessioner
	recent	time.Time
}

// Sessions集
type Sessions struct{
    Expired         time.Duration                                       // 保存session时间长
    Name            string                                              // 标识名称。用于Cookie
    Size            int                                                 // 会话ID长度。用于Cookie
    Salt            string                                              // 加盐，由于计算机随机数是伪随机数。（可默认为空）。用于Cookie
    ActivationID    bool                                                // 为true，保持会话ID。意思就是会话ID过期了，可以激活再次使用。用于Cookie
    ss        		vmap.Map                                            // 集，map[id]*Session
}

//Len 当前Session数量
//	int	数量
func (T *Sessions) Len() int {
	return T.ss.Len()
}

//ProcessDeadAll 定时来处理过期的Session
//	[]string	过期的ID名称
func (T *Sessions) ProcessDeadAll() []interface{} {
    var expId   []interface{}
	if T.Expired != 0 {
	    currTime := time.Now()
		T.ss.Range(func(id, mse interface{}) bool{
			ms := mse.(*manageSession)
	        recentTime := ms.recent.Add(T.Expired)
	        if currTime.After(recentTime) {
	        	//追加了expId一次性删除
	        	expId = append(expId, id)
	        	//执行Defer
	        	go ms.s.Free()
	        }
			return true
		})
	    T.ss.Dels(expId)
	}
    return expId
}

//triggerDeadSession 由用户来触发，并删除已挂载入的Defer
func (T *Sessions) triggerDeadSession(ms *manageSession) (ok bool) {
	if T.Expired != 0 {
	    currTime := time.Now()
	    recentTime := ms.recent.Add(T.Expired)
	     if currTime.After(recentTime) {
	        go ms.s.Free()
	        return true
	    }
	}
    return
}

//generateSessionIdSalt 生成Session标识符,并加盐
//	string  标识符
func (T *Sessions) generateSessionIdSalt() string {
	rnd := make([]byte, T.Size)
    err := GenerateRandomId(rnd)
    if err != nil {
    	panic(err)
    }
    if T.Salt == "" {
	    id := fmt.Sprintf("%x", rnd)
	    return id[:T.Size]
    }
    return AddSalt(rnd, T.Salt)
}

//generateSessionId 生成Session标识符
//	string  标识符
func (T *Sessions) generateSessionId() string {
	rnd := make([]byte, T.Size)
    err := GenerateRandomId(rnd)
    if err != nil {
    	panic(err)
    }
    id := fmt.Sprintf("%x", rnd)
    return id[:T.Size]
}


//SessionId 从请求中读取会话标识
//	req *http.Request   请求
//	id string           id标识符
//	err error           错误
func (T *Sessions) SessionId(req *http.Request) (id string, err error) {
    c, err := req.Cookie(T.Name)
    if err != nil || c.Value == "" {
    	return "", verror.TrackErrorf("vweb: 该用会话属性（%s）名称，从客户端请求中没有找可用ID值。", T.Name)
    }
    return c.Value, nil
}

//NewSession 新建会话
//	id string	id标识符
//	Sessioner   会话
func (T *Sessions) NewSession(id string) Sessioner {
	if id == "" {
		id = T.generateRandSessionId()
	}
	if s, ok := T.GetSession(id); ok {
		return s
	}
	return T.SetSession(id, &Session{})
}

//GetSession 使用id读取会话
//	id string   id标识符
//	Sessioner   会话
//	bool        是否存在
func (T *Sessions) GetSession(id string) (Sessioner, bool) {
    mse, ok := T.ss.GetHas(id)
    if !ok {
    	return nil, false
    }
    ms := mse.(*manageSession)

    if T.triggerDeadSession(ms) {
    	T.ss.Del(id)
        return nil, false
    }
    ms.recent = time.Now()
    return ms.s, true
}

//SetSession 使用id写入新的会话
//	id string   id标识符
//	s Sessioner 新的会话
//	Sessioner   会话
func (T *Sessions) SetSession(id string, s Sessioner) Sessioner {
	return T.setSession(id, s, true)
}

func (T *Sessions) setSession(id string, s Sessioner, free bool) Sessioner {
	if inf, ok := T.ss.GetHas(id); ok {
		ms := inf.(*manageSession)
		if ms.s.Token() == s.Token() {
	    	//已经存在，无法再设置
	    	return s
		}
		if free {
	    	//替换原有Session，需要清理原有的defer
			go ms.s.Free()
		}
	}
	if t, can := s.(*Session); can {
		//对应这个id，并保存
		t.id = id
	}
	ms := &manageSession{
		s:s,
		recent:time.Now(),
	}
    T.ss.Set(id, ms)
    return s
}

//DelSession 使用id删除的会话
//	id string   id标识符
func (T *Sessions) DelSession(id string) {
    if mse, ok := T.ss.GetHas(id); ok {
	    ms := mse.(*manageSession)
		go ms.s.Free()
		T.ss.Del(id)
    }
}

//writeToClient 写入到客户端
//	rw http.ResponseWriter  响应
//	id string               id标识符
//	Sessioner    			会话
func (T *Sessions) writeToClient(rw http.ResponseWriter, id string) Sessioner {
    wh := rw.Header()
    
    //防止重复写入
    for _, c := range readSetCookies(wh) {
    	if c.Name == T.Name {
    		if ss, ok := T.GetSession(c.Value); ok {
    			return ss
    		}
    	}
    }
    
    cookie := &http.Cookie{
        Name: T.Name,
        Value: id,
        Path: "/",
        HttpOnly: true,
    }
    wh.Add("Set-Cookie", cookie.String())
    return T.SetSession(id, &Session{})
}

func (T *Sessions) generateRandSessionId() string {
	var (
		id 			string
		maxWait	 	= time.Second
		wait		time.Duration
		printErr	= "vweb: 警告>>会话ID即将耗尽，请尽快加大调整ID长度。本次已为用户分配临时ID(%s)\n"
	)
    if T.Salt != "" {
    	for id = T.generateSessionIdSalt(); T.ss.Has(id);{
    		wait=delay(wait, maxWait)
    		id = T.generateSessionIdSalt()
    		if wait >= maxWait {
    			id+="-temp"
    			//ID即将耗尽
    			fmt.Printf(printErr, id)
    		}
    	}
   		return id
    }
	for id = T.generateSessionId(); T.ss.Has(id);{
		wait=delay(wait, maxWait)
		id = T.generateSessionId()
		if wait >= maxWait {
			id+="-temp"
			//ID即将耗尽
    		fmt.Printf(printErr, id)
		}
	}
	return id
}

//Session 会话
//	rw http.ResponseWriter  响应
//	req *http.Request       请求
//	Sessioner               会话接口
func (T *Sessions) Session(rw http.ResponseWriter, req *http.Request) Sessioner {
    //判断标识名是否存在
    id, err := T.SessionId(req)
    if err != nil {
    	//客户是第一次请求，没有会话ID
    	//现在生成一个ID给客户端
	    id = T.generateRandSessionId()
        return T.writeToClient(rw, id)
    }

    //判断Id是否有效
    s, ok := T.GetSession(id)
    if !ok {
    	//会话ID过期或不存在
    	//判断是否重新使用旧ID
        if T.ActivationID && len(id) == T.Size {
            return T.writeToClient(rw, id)
        }
 
	    id = T.generateRandSessionId()
        return T.writeToClient(rw, id)
    }
    return s
}