package drivebackup

import (
 "context"
 "errors"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
 bbolt "go.etcd.io/bbolt"
)
func TestServiceDataBackupAndRestore(t *testing.T){
 if _,err:=exec.LookPath("restic");err!=nil{if os.Getenv("DRIVE_REQUIRE_RESTIC_TEST")=="1"{t.Fatal(err)};t.Skip("restic unavailable")}
 base:=t.TempDir();dataRoot:=filepath.Join(base,"data");if err:=os.MkdirAll(filepath.Join(dataRoot,"nested"),0700);err!=nil{t.Fatal(err)}
 wanted:=[]byte("Drive data backup integration test\n");if err:=os.WriteFile(filepath.Join(dataRoot,"nested","test.txt"),wanted,0600);err!=nil{t.Fatal(err)}
 db,err:=bbolt.Open(filepath.Join(base,"live.db"),0600,nil);if err!=nil{t.Fatal(err)};defer db.Close()
 if err:=db.Update(func(tx *bbolt.Tx)error{b,err:=tx.CreateBucket([]byte("test"));if err!=nil{return err};return b.Put([]byte("value"),[]byte("before"))});err!=nil{t.Fatal(err)}
 service,err:=NewService(filepath.Join(base,"state"));if err!=nil{t.Fatal(err)}
 password:="data-backup-integration-password"
 c:=Config{ID:"data-test",Name:"Data integration",Enabled:true,Backend:"local",LocalRepository:filepath.Join(base,"repo"),SourcePaths:[]string{"/data"},SourceOwner:1,SourceBase:base}
 actor:=Actor{UserID:1,Username:"test-admin"}
 if _,err:=service.Save(c,SecretUpdate{RepositoryPassword:&password},actor);err!=nil{t.Fatal(err)}
 wait:=func(id string)Job{t.Helper();deadline:=time.Now().Add(time.Minute);for time.Now().Before(deadline){for _,job:=range service.Jobs(){if job.ID==id&&job.State!="queued"&&job.State!="running"&&job.State!="canceling"{return job}};time.Sleep(50*time.Millisecond)};t.Fatal("job timed out");return Job{}}
 initialized,err:=service.Start(StartRequest{DestinationID:c.ID,Action:"initialize",ConfirmInitialize:true,RecoveryAcknowledged:true},actor);if err!=nil{t.Fatal(err)}
 if job:=wait(initialized.ID);job.State!="succeeded"{t.Fatalf("init: %+v",job)}
 plan:=DataBackupPlan{Sources:[]string{dataRoot},DisplaySources:[]string{"/data"},Revalidate:func(context.Context)error{return nil},ExportDatabase:func(ctx context.Context,w io.Writer)error{
  if err:=db.View(func(tx *bbolt.Tx)error{_,err:=tx.WriteTo(w);return err});err!=nil{return err}
  return db.Update(func(tx *bbolt.Tx)error{return tx.Bucket([]byte("test")).Put([]byte("value"),[]byte("after"))})
 }}
 request:=StartRequest{DestinationID:c.ID,Action:"backup",ConfirmBackup:true}
 if _,err:=service.StartBackup(StartRequest{DestinationID:c.ID,Action:"backup"},actor,plan);err==nil{t.Fatal("backup started without confirmation")}
 started,err:=service.StartBackup(request,actor,plan);if err!=nil{t.Fatal(err)}
 finished:=wait(started.ID);if finished.State!="succeeded"||finished.SnapshotID==""||!finished.DatabaseIncluded{t.Fatalf("data backup: %+v",finished)}
 snapshots,err:=service.driver.Run(context.Background(),c,Secrets{RepositoryPassword:password},Request{Action:"restore",Snapshot:finished.SnapshotID});if err!=nil{t.Fatalf("restore: %v %s",err,snapshots.Stderr)}
 foundData,foundDB,foundManifest:=false,false,false
 err=filepath.Walk(snapshots.RestoreDirectory,func(path string,info os.FileInfo,walkErr error)error{
  if walkErr!=nil{return walkErr};if info.IsDir(){return nil}
  switch info.Name(){
  case "test.txt":raw,err:=os.ReadFile(path);if err!=nil{return err};if string(raw)==string(wanted){foundData=true}
  case "drive-state.db":restored,err:=bbolt.Open(path,0600,&bbolt.Options{ReadOnly:true});if err!=nil{return err};defer restored.Close();return restored.View(func(tx *bbolt.Tx)error{if string(tx.Bucket([]byte("test")).Get([]byte("value")))!="before"{return errors.New("database export was not the earlier consistent image")};foundDB=true;return nil})
  case "drive-manifest.json":foundManifest=true
  }
  return nil
 });if err!=nil||!foundData||!foundDB||!foundManifest{t.Fatalf("restore verification: data=%v db=%v manifest=%v err=%v",foundData,foundDB,foundManifest,err)}
 pending,_:=filepath.Glob(filepath.Join(service.dir,"exports","job-*"));if len(pending)!=0{t.Fatal("plaintext staging retained after successful job")}
 approval:=RestoreApproval{Revalidate:func(context.Context)error{return nil}}
 restoreRequest:=StartRequest{DestinationID:c.ID,Action:"restore-staging",RestoreFromJobID:finished.ID,ConfirmRestore:true}
 unconfirmed:=restoreRequest;unconfirmed.ConfirmRestore=false
 if _,err:=service.StartRestore(unconfirmed,actor,approval);err==nil{t.Fatal("restore started without confirmation")}
 if _,err:=service.StartRestore(restoreRequest,Actor{UserID:2},approval);err==nil{t.Fatal("different owner allowed to restore")}
 // Recovery must not require the original data folder to still exist.
 if err:=os.RemoveAll(dataRoot);err!=nil{t.Fatal(err)}
 if _,err:=service.Start(restoreRequest,actor);err==nil{t.Fatal("restore started without trusted approval")}
 staged,err:=service.StartRestore(restoreRequest,actor,approval);if err!=nil{t.Fatal(err)}
 stagedJob:=wait(staged.ID);if stagedJob.State!="succeeded"{t.Fatalf("staging restore: %+v",stagedJob)}
 artifacts:=service.RestoreArtifacts();if len(artifacts)!=1||!artifacts[0].ContentVerified||!artifacts[0].DatabaseHeaderReadable||!artifacts[0].Available{t.Fatalf("restore registry: %+v",artifacts)}
 stagePath:=filepath.Join(service.driver.workDir,"restores",staged.ID)
 stagedDataFound:=false
 if err:=filepath.Walk(stagePath,func(path string,info os.FileInfo,err error)error{if err!=nil{return err};if info.Name()=="test.txt"{stagedDataFound=true;raw,err:=os.ReadFile(path);if err!=nil{return err};if string(raw)!=string(wanted){return errors.New("staged content mismatch")}};return nil});err!=nil{t.Fatal(err)}
 if !stagedDataFound{t.Fatal("staged document missing")}
 reopened,err:=NewService(service.dir);if err!=nil{t.Fatal(err)};if len(reopened.RestoreArtifacts())!=1{t.Fatal("staged artifact lost across restart")}
 if err:=service.CleanupRestore(staged.ID,Actor{UserID:2});err==nil{t.Fatal("other owner cleanup allowed")}
 if err:=service.CleanupRestore(staged.ID,actor);err!=nil{t.Fatal(err)};if len(service.RestoreArtifacts())!=0{t.Fatal("artifact registry not cleaned")}
 if _,err:=os.Lstat(stagePath);!os.IsNotExist(err){t.Fatal("staging path retained after cleanup")}
 if err:=db.View(func(tx *bbolt.Tx)error{if string(tx.Bucket([]byte("test")).Get([]byte("value")))!="after"{return errors.New("live database was replaced")};return nil});err!=nil{t.Fatal(err)}
 // Recreate the selected source so the existing export-failure test can run.
 if err:=os.MkdirAll(dataRoot,0700);err!=nil{t.Fatal(err)}
 t.Setenv("DRIVE_RESTORE_MAX_BYTES","1")
 oversized,err:=service.StartRestore(restoreRequest,actor,approval);if err!=nil{t.Fatal(err)}
 if job:=wait(oversized.ID);job.State!="failed"{t.Fatalf("restore byte limit bypassed: %+v",job)}
 if err:=service.CleanupRestore(oversized.ID,actor);err!=nil{t.Fatal(err)}
 bad:=plan;bad.ExportDatabase=func(context.Context,io.Writer)error{return errors.New("injected export failure")}
 failed,err:=service.StartBackup(request,actor,bad);if err!=nil{t.Fatal(err)};if result:=wait(failed.ID);result.State!="failed"||result.SnapshotID!=""{t.Fatalf("export failure reported success: %+v",result)}
 pending,_=filepath.Glob(filepath.Join(service.dir,"exports","job-*"));if len(pending)!=0{t.Fatal("plaintext staging retained after failure")}
 automation,err:=NewAutomation(service);if err!=nil{t.Fatal(err)}
 clock:=time.Now().UTC();now:=time.Date(clock.Year(),clock.Month(),clock.Day(),12,0,0,0,time.UTC);spec:=ScheduleInput{Name:"Scheduled integration",DestinationID:c.ID,Enabled:true,TimeZone:"UTC",Hour:0,Minute:0}
 schedule,err:=automation.Save(spec,actor,c.SourceBase,now);if err!=nil{t.Fatal(err)}
 schedule.NextRunAt=now.Add(-time.Hour);automation.schedules[schedule.ID]=schedule
 build:=func(context.Context,Config,Schedule)(DataBackupPlan,Actor,error){return plan,actor,nil}
 if err:=automation.Tick(context.Background(),now,build);err!=nil{t.Fatal(err)}
 recorded:=automation.List(actor.UserID,c.SourceBase);if len(recorded)!=1||recorded[0].LastJobID==""{t.Fatalf("schedule did not queue backup: %+v",recorded)}
 scheduledResult:=wait(recorded[0].LastJobID);if scheduledResult.State!="succeeded"||scheduledResult.ScheduleID!=schedule.ID||scheduledResult.StreamID==""{t.Fatalf("scheduled backup: %+v",scheduledResult)}
 if err:=automation.Tick(context.Background(),now.Add(15*time.Second),build);err!=nil{t.Fatal(err)}
 if current:=automation.List(actor.UserID,c.SourceBase)[0];current.LastJobID!=scheduledResult.ID||current.State!="succeeded"{t.Fatalf("duplicate or stale scheduled outcome: %+v",current)}
 catalog,err:=service.Catalog(context.Background(),actor.UserID,c.SourceBase,"",100);if err!=nil||len(catalog.Entries)<2{t.Fatalf("catalog missing backups: %v",err)}
}
