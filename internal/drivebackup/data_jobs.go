package drivebackup

import (
 "bufio"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "time"
)

var ErrBackupPlan=errors.New("data backup requires an approved source plan, database exporter and explicit confirmation")
var ErrBackupPreparation=errors.New("backup preparation failed")

// DataBackupPlan is created by trusted server code, never decoded from a client.
type DataBackupPlan struct {
 scheduleID string
 scheduleWindow string
 expectedConfigRevision string
 restoreAccess func(context.Context)error
 restoreSource Job
 restoreID string
 Sources []string
 DisplaySources []string
 ExportDatabase func(context.Context,io.Writer)error
 Revalidate func(context.Context)error
}
func(s *Service)Configuration(id string)(Config,error){
 s.mu.Lock();defer s.mu.Unlock();c,ok:=s.destinations[id];if !ok{return Config{},ErrNotFound};c.SourcePaths=append([]string(nil),c.SourcePaths...);return c,nil
}
func(s *Service)validateDataPlan(c Config,actor Actor,plan *DataBackupPlan)error{
 if plan==nil||plan.ExportDatabase==nil||plan.Revalidate==nil||len(plan.Sources)==0||len(plan.Sources)>16||len(plan.Sources)!=len(plan.DisplaySources)||len(plan.Sources)!=len(c.SourcePaths)||actor.UserID!=c.SourceOwner{return ErrBackupPlan}
 base,err:=filepath.EvalSymlinks(c.SourceBase);if err!=nil||!filepath.IsAbs(base){return ErrBackupPlan}
 state,err:=filepath.EvalSymlinks(s.dir);if err!=nil{return err}
 for i,source:=range plan.Sources{
  if !filepath.IsAbs(source)||plan.DisplaySources[i]!=c.SourcePaths[i]{return ErrBackupPlan}
  real,err:=filepath.EvalSymlinks(source);if err!=nil{return err}
  if !within(real,base)||within(real,state)||within(state,real){return ErrBackupPlan}
  info,err:=os.Stat(real);if err!=nil||!info.IsDir(){return ErrBackupPlan}
 }
 return nil
}

type exportLimitWriter struct{ctx context.Context;writer io.Writer;written int64}
func(w *exportLimitWriter)Write(p []byte)(int,error){
 if err:=w.ctx.Err();err!=nil{return 0,err}
 if w.written+int64(len(p))>1<<30{return 0,errors.New("database export exceeds the 1 GiB preparation limit")}
 n,err:=w.writer.Write(p);w.written+=int64(n);return n,err
}

func(s *Service)executeJob(ctx context.Context,c Config,secrets Secrets,request StartRequest,plan *DataBackupPlan)(Result,error){
 if request.Action=="restore-staging"{return s.executeRestore(ctx,c,secrets,plan)}
 if request.Action!="backup"{return s.driver.Run(ctx,c,secrets,Request{Action:request.Action,IntegrityPercent:request.IntegrityPercent,Timeout:30*time.Minute})}
 failed:=Result{Action:"backup",ExitCode:-1}
 if plan==nil{return failed,ErrBackupPlan}
 ctx,cancel:=context.WithTimeout(ctx,6*time.Hour);defer cancel()
 prepareError:=func(err error)(Result,error){return failed,fmt.Errorf("%w: %w",ErrBackupPreparation,err)}
 if err:=plan.Revalidate(ctx);err!=nil{return prepareError(err)}
 exports:=filepath.Join(s.dir,"exports")
 if err:=os.MkdirAll(exports,0700);err!=nil{return prepareError(err)}
 info,err:=os.Lstat(exports);if err!=nil{return prepareError(err)}
 if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{return prepareError(errors.New("export staging directory is not private"))}
 stage,err:=os.MkdirTemp(exports,"job-");if err!=nil{return prepareError(err)}
 // Best-effort cleanup after success, failure or cancellation. A process crash
 // can leave a private orphan, which is reported for operator cleanup.
 defer os.RemoveAll(stage)
 file,err:=os.OpenFile(filepath.Join(stage,"drive-state.db"),os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return prepareError(err)}
 writer:=&exportLimitWriter{ctx:ctx,writer:file}
 exportErr:=plan.ExportDatabase(ctx,writer)
 if exportErr==nil{exportErr=file.Sync()};closeErr:=file.Close();if exportErr!=nil{return prepareError(exportErr)};if closeErr!=nil{return prepareError(closeErr)}
 catalogFile,err:=os.OpenFile(filepath.Join(stage,"drive-catalog.db"),os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return prepareError(err)}
 catalogErr:=s.ExportCatalog(ctx,catalogFile);if catalogErr==nil{catalogErr=catalogFile.Sync()};catalogCloseErr:=catalogFile.Close()
 if catalogErr!=nil{return prepareError(catalogErr)};if catalogCloseErr!=nil{return prepareError(catalogCloseErr)}
 manifest:=struct{
  Schema int `json:"schema"`;CreatedAt time.Time `json:"createdAt"`;Database string `json:"database"`;CatalogDatabase string `json:"catalogDatabase"`;Sources []string `json:"driveSources"`;Consistency string `json:"consistency"`
 }{1,time.Now().UTC(),"drive-state.db","drive-catalog.db",append([]string(nil),plan.DisplaySources...),"Consistent database export plus live selected files; not an atomic filesystem snapshot."}
 raw,err:=json.MarshalIndent(manifest,"","  ");if err!=nil{return prepareError(err)}
 if err:=os.WriteFile(filepath.Join(stage,"drive-manifest.json"),raw,0600);err!=nil{return prepareError(err)}
 if err:=plan.Revalidate(ctx);err!=nil{return prepareError(err)}
 // The export is a separate approved source, never the whole private workdir.
 driver:=*s.driver
 driver.roots=append(append([]string(nil),plan.Sources...),stage)
 sources:=append(append([]string(nil),plan.Sources...),stage)
 result,runErr:=driver.Run(ctx,c,secrets,Request{Action:"backup",Sources:sources,Timeout:6*time.Hour})
 result.DatabaseExportPath=filepath.Join(stage,"drive-state.db")
 return result,runErr
}
func finalizeDataBackup(job *Job,result Result,runErr error){
 if errors.Is(runErr,context.Canceled)||errors.Is(runErr,context.DeadlineExceeded){return}
 if errors.Is(runErr,ErrBackupPreparation){job.State="failed";job.Message="Database export/source preparation failed; restic backup was not started.";return}
 if runErr!=nil{job.Message="Backup did not report a clean completion. A snapshot or uploaded data may exist; inspect the repository before retrying.";return}
 var summary struct{
  Type string `json:"message_type"`;SnapshotID string `json:"snapshot_id"`;Files uint64 `json:"total_files_processed"`;Bytes uint64 `json:"total_bytes_processed"`
 }
 found:=false
 scanner:=bufio.NewScanner(strings.NewReader(result.Stdout));scanner.Buffer(make([]byte,65536),1<<20)
 for scanner.Scan(){var line struct{Type string `json:"message_type"`};if json.Unmarshal(scanner.Bytes(),&line)!=nil||line.Type!="summary"{continue};if err:=json.Unmarshal(scanner.Bytes(),&summary);err==nil&&snapshotPattern.MatchString(summary.SnapshotID){found=true}}
 if scanner.Err()!=nil||!found{job.State="needs-review";job.Message="Restic exited successfully but no valid snapshot summary was recorded. Inspect the repository; do not assume a verified backup.";return}
 job.DatabaseExportPath=result.DatabaseExportPath
 job.SnapshotID=summary.SnapshotID;job.FilesProcessed=summary.Files;job.BytesProcessed=summary.Bytes;job.DatabaseIncluded=true
 job.Message="Snapshot created with a consistent Drive database export and selected live folders. This is not an atomic filesystem snapshot; verification and restore tests remain necessary."
}
