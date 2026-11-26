package drivebackup
import("context";"fmt";"path/filepath";"testing";"time"
 bbolt "go.etcd.io/bbolt")
func TestCatalogSurvivesHistoryRolloverAndRestart(t *testing.T){
 service,err:=NewService(filepath.Join(t.TempDir(),"state"));if err!=nil{t.Fatal(err)}
 actor:=Actor{UserID:1,Username:"admin"};scope:=t.TempDir();all:=make([]Job,0,115);start:=time.Now().Add(-3*time.Hour).UTC()
 for i:=0;i<115;i++{job:=Job{ID:fmt.Sprintf("%032x",i+1),DestinationID:"test",Action:"backup",State:"succeeded",Actor:actor,CreatedAt:start.Add(time.Duration(i)*time.Minute),FinishedAt:start.Add(time.Duration(i)*time.Minute+time.Second),SourceBase:scope,DatabaseExportPath:"/config/drive-backups/exports/test/drive-state.db",Sources:[]string{"/docs"},SnapshotID:fmt.Sprintf("%064x",i+1),DatabaseIncluded:true,StreamID:backupSetID("test",1,scope)};all=append(all,job)}
 if err:=service.withCatalog(false,func(db *bbolt.DB)error{return db.Update(func(tx *bbolt.Tx)error{for _,job:=range all{if err:=putCatalog(tx,job);err!=nil{return err}};return nil})});err!=nil{t.Fatal(err)}
 service.jobs=append([]Job(nil),all[15:]...);if err:=service.writeJSON("jobs.json",service.jobs);err!=nil{t.Fatal(err)}
 if _,err:=service.BackupForRestore(all[0].ID,actor);err!=nil{t.Fatalf("old cataloged backup inaccessible: %v",err)}
 if _,err:=service.BackupForRestore(all[0].ID,Actor{UserID:2});err==nil{t.Fatal("wrong actor accessed catalog backup")}
 first,err:=service.Catalog(context.Background(),1,scope,"",100);if err!=nil||len(first.Entries)!=100||first.NextCursor==""{t.Fatalf("first page: %d %q %v",len(first.Entries),first.NextCursor,err)}
 second,err:=service.Catalog(context.Background(),1,scope,first.NextCursor,100);if err!=nil||len(second.Entries)!=15{t.Fatalf("second page: %d %v",len(second.Entries),err)}
 hidden,err:=service.Catalog(context.Background(),2,scope,"",100);if err!=nil||len(hidden.Entries)!=0{t.Fatal("catalog owner filter failed")}
 reopened,err:=NewService(service.dir);if err!=nil{t.Fatal(err)};if _,err:=reopened.BackupForRestore(all[0].ID,actor);err!=nil{t.Fatal(err)}
 changed:=all[0];changed.SnapshotID=fmt.Sprintf("%064x",9999);if err:=service.storeCatalog(changed);err==nil{t.Fatal("immutable catalog identity overwritten")}
 if _,err:=service.Catalog(context.Background(),1,scope,"not-a-cursor",10);err==nil{t.Fatal("bad cursor accepted")}
}
func TestBackupSetStableAcrossJobPaths(t *testing.T){
 a:=Config{ID:"test",SourceOwner:1,SourceBase:"/srv",SourcePaths:[]string{"/docs"}}
 b:=a;b.SourcePaths=[]string{"/docs","/media"}
 if a.BackupSetID()!=b.BackupSetID(){t.Fatal("logical job sources split a destination/account stream")}
 b.SourceOwner=2;if a.BackupSetID()==b.BackupSetID(){t.Fatal("different owners share backup stream")}
 b=a;b.SourceBase="/different";if a.BackupSetID()==b.BackupSetID(){t.Fatal("different scopes share backup stream")}
}
