package bolt

import("bytes";"context";"errors";"io";"os";"path/filepath";"testing"
 "github.com/asdine/storm/v3")
func TestDatabaseExporterConsistentImage(t *testing.T){
 base:=t.TempDir();db,err:=storm.Open(filepath.Join(base,"source.db"));if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=db.Set("backup-test","value","before");err!=nil{t.Fatal(err)}
 var image bytes.Buffer;if err:=databaseExporter(db)(context.Background(),&image);err!=nil{t.Fatal(err)}
 if err:=db.Set("backup-test","value","after");err!=nil{t.Fatal(err)}
 clonePath:=filepath.Join(base,"export.db");if err:=os.WriteFile(clonePath,image.Bytes(),0600);err!=nil{t.Fatal(err)}
 clone,err:=storm.Open(clonePath);if err!=nil{t.Fatal(err)};defer clone.Close();var value string
 if err:=clone.Get("backup-test","value",&value);err!=nil{t.Fatal(err)};if value!="before"{t.Fatalf("snapshot contains %q",value)}
 ctx,cancel:=context.WithCancel(context.Background());cancel();if err:=databaseExporter(db)(ctx,io.Discard);!errors.Is(err,context.Canceled){t.Fatalf("canceled export: %v",err)}
}
