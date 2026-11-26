package drivebackup
import "testing"
func TestBackupSourceSelectionValidation(t *testing.T){
 if err:=validateSourceSelection([]string{"/general/docs","/media/photos"},1,"/srv");err!=nil{t.Fatal(err)}
 for _,paths:=range [][]string{{"/"},{"/a/../b"},{"/a","/a/b"},{"/a","/a"},{"/a\\b"},{"relative"},{"/a\x00b"}}{if validateSourceSelection(paths,1,"/srv")==nil{t.Errorf("unsafe selection accepted: %q",paths)}}
 if validateSourceSelection([]string{"/general"},0,"/srv")==nil{t.Fatal("unbound owner accepted")}
}
