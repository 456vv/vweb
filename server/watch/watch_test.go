package watch
	
import (
	"testing"
    "io/ioutil"
    "github.com/fsnotify/fsnotify"
    "time"
)


var _ *fsnotify.Op

func Test_NewWatch(t *testing.T){
	tfile, err := ioutil.TempFile("", "*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	tpath := tfile.Name()
	
	watch, err := NewWatch()
	if err != nil {
		t.Fatal(err)
	}
	err = watch.Monitor(tpath, func(event fsnotify.Event){
		t.Log(event)
	})
	if err != nil {
		t.Fatal(err)
	}
	
	tfile.Write([]byte{65})
	tfile.Close()
	time.Sleep(time.Second)
	watch.Close()
}