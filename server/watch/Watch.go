package watch
import(
    "github.com/fsnotify/fsnotify"
    "sync"
    "time"
    "strings"
    "path/filepath"
)



//WatchEventFun 看守事件函数
type WatchEventFun func(fsnotify.Event)

//Watch 看守
type Watch struct{
    watcher     *fsnotify.Watcher                                               // fsnotify的看守望对象
    eventfunc   map[string]WatchEventFun                                        // 记录所有事件函数
    rwmu 		*sync.RWMutex
    closed      bool
}

//NewWatch 看守对象
func NewWatch() (*Watch, error) {
    fsnotifyWatcher, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }
    w := &Watch{
        watcher: fsnotifyWatcher,
        eventfunc: make(map[string]WatchEventFun),
        rwmu: new(sync.RWMutex),
    }
    go w.event()
    return w, nil
}

//event 事件
func (w *Watch) event(){
    var oldEvent    fsnotify.Event
    var oldSecond   int64
    L:for {
        if w.closed {
            break L
        }
        select {
        case event := <- w.watcher.Events:
        	if event.Op == 0 {
        		//过虑bug
        		continue
        	}
            newSecond := time.Now().Unix()
            if oldSecond == newSecond && (event.Op == oldEvent.Op && event.Name == oldEvent.Name) {
                continue
            }
            oldSecond   = newSecond
            oldEvent    = event
            
            w.rwmu.RLock()
            for k, f := range w.eventfunc {
                if strings.Contains(event.Name, filepath.Clean(k)) {
                   go f(event)
                }
            }
            w.rwmu.RUnlock()
        case <- w.watcher.Errors:
        }
    }
}

//Remove 移除事件函数
func (w *Watch) Remove(path string){
    w.rwmu.Lock()
    defer w.rwmu.Unlock()
    w.watcher.Remove(path)
    delete(w.eventfunc, path)
}

//Monitor 监视
func (w *Watch) Monitor(path string, fun WatchEventFun) error {
    w.rwmu.Lock()
    defer w.rwmu.Unlock()
    err := w.watcher.Add(path)
    if err != nil {
        return err
    }
    w.eventfunc[path] = fun
    return nil
}

//Close 关闭看守
func (w *Watch) Close() error {
    w.closed = true
    w.watcher.Close()
    return nil
}
