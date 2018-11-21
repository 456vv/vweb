package vweb

import (
    "fmt"
    "time"
    "net/http"
    "github.com/456vv/vmap/v2"
)

type manageSession struct{
	s		*Session
	recent	time.Time
}

// Sessions集
type Sessions struct{
    Expired         time.Duration                                       // 保存session时间长（默认：20分钟）
    Name            string                                              // 标识名称(默认:VID)
    Size            int                                                 // 会话ID长度(默认长度40位)
    Salt            string                                              // 加盐，由于计算机随机数是伪随机数。（可默认为空）
    ActivationID    bool                                                // 为true，保持会话ID
    sessions        *vmap.Map                                           // 集，map[id]*Session
}

func newSessions() *Sessions {
    T := &Sessions{
        Name:"VID",
        Expired: time.Minute * 20,
        Size: 40,
        sessions: vmap.NewMap(),
    }
    return T
}

func (T *Sessions) init(){
	if T.sessions == nil {
		T.sessions = vmap.NewMap()
	}
}

//update 更新配置
func (T *Sessions) update(confSession ConfigSitePropertySession){

    if confSession.Expired != 0 {
        T.Expired = time.Duration(confSession.Expired) * time.Millisecond
    }

    if confSession.Name != "" {
        T.Name = confSession.Name
    }

    if confSession.Size != 0 {
        T.Size = confSession.Size
    }

    T.Salt 			= confSession.Salt
    T.ActivationID 	= confSession.ActivationID
}

//ProcessDeadAll 定时来处理过期的Session
//	[]string	过期的ID名称
func (T *Sessions) ProcessDeadAll() []interface{} {
    var expId   []interface{}
	if T.Expired != 0 {
		T.init()
	    currTime := time.Now()
		T.sessions.Range(func(id, mse interface{}) bool{
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
	    T.sessions.Dels(expId)
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

//SessionIdSalt 加盐
//	rnd []byte	标识字节串
//	string  	标识符
func (T *Sessions) SessionIdSalt(rnd []byte) string {
    return AddSalt(rnd, T.Salt)
}

//GenerateSessionIdSalt 生成Session标识符,并加盐
//	string  标识符
func (T *Sessions) GenerateSessionIdSalt() string {
    var rnd = make([]byte, T.Size)
    err := GenerateRandomId(rnd)
    if err != nil {
    	panic(err)
    }
    return T.SessionIdSalt(rnd)
}

//GenerateSessionId 生成Session标识符
//	string  标识符
func (T *Sessions) GenerateSessionId() string {
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
    	return "", fmt.Errorf("vweb.Sessions: 该用会话属性（%s）名称，从客户端请求中没有找可用ID值。", T.Name)
    }
    return c.Value, nil
}

//NewSession 使用id读取会话，不存在，则新建
//	id string   id标识符
//	*session    会话
func (T *Sessions) NewSession(id string) *Session {
	s, err := T.GetSession(id)
	if err != nil {
		return T.SetSession(id, NewSession())
	}
	return s
}

//GetSession 使用id读取会话
//	id string   id标识符
//	*session    会话
//	error       错误
func (T *Sessions) GetSession(id string) (*Session, error) {
	T.init()
    mse, ok := T.sessions.GetHas(id)
    if !ok {
    	return nil, fmt.Errorf("vweb.Sessions: 该ID（%s）不是有效的。", id)
    }
    ms := mse.(*manageSession)

    if T.triggerDeadSession(ms) {
    	T.sessions.Del(id)
        return nil, fmt.Errorf("vweb.Sessions: 该ID（%s）是有效的，但会话已经过期了。", id)
    }
    ms.recent = time.Now()
    return ms.s, nil
}

//SetSession 使用id写入新的会话
//	id string   id标识符
//	s *Session  新的会话
//	*Session    会话
func (T *Sessions) SetSession(id string, s *Session) *Session {
	T.init()
	mse, ok := T.sessions.GetHas(id)
	if ok {
    	ms := mse.(*manageSession)
    	if ms.s.id == s.id {
    		//已经存在，无法再设置
    		return s
    	}else{
    		//替换原有Session，需要清理原有的defer
    		go ms.s.Free()
    	}
	}
	//对应这个id，并保存
	s.id = id
	ms := &manageSession{
		s:s,
		recent:time.Now(),
	}
    T.sessions.Set(id, ms)
    return s
}

//DelSession 使用id删除的会话
//	id string   id标识符
func (T *Sessions) DelSession(id string) {
	T.init()
    if mse, ok := T.sessions.GetHas(id); ok {
	    ms := mse.(*manageSession)
		go ms.s.Free()
		T.sessions.Del(id)
    }
}

//writeToClient 写入到客户端
//	rw http.ResponseWriter  响应
//	id string               id标识符
//	*session    			会话
func (T *Sessions) writeToClient(rw http.ResponseWriter, id string) *Session {
    cookie := &http.Cookie{
        Name: T.Name,
        Value: id,
        Path: "/",
        HttpOnly: true,
    }
    wh := rw.Header()
    wh.Add("Set-Cookie", cookie.String())

    return T.SetSession(id, NewSession())
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

	    if T.Salt != "" {
			id = T.GenerateSessionIdSalt()
	    }else{
			id = T.GenerateSessionId()
	    }
        return T.writeToClient(rw, id)
    }

    //判断Id是否有效
    s, err := T.GetSession(id)
    if err != nil {
    	//会话ID过期或不存在
    	//判断是否重新使用旧ID
        if T.ActivationID && len(id) == T.Size {
            return T.writeToClient(rw, id)
        }
 
	    if T.Salt != "" {
			id = T.GenerateSessionIdSalt()
	    }else{
			id = T.GenerateSessionId()
	    }
        return T.writeToClient(rw, id)
    }
    return s
}