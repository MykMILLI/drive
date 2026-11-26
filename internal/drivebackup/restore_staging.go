package drivebackup

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"
 bbolt "go.etcd.io/bbolt"
)

var restoreIDPattern=regexp.MustCompile(`^[a-f0-9]{32}$`)
var ErrRestoreApproval=errors.New("staging restore requires confirmation, a recorded eligible backup and current account-scope approval")
type RestoreApproval struct{Revalidate func(context.Context)error}
type RestoreArtifact struct{
 ID string `json:"id"`;SourceJobID string `json:"sourceJobId"`;DestinationID string `json:"destinationId"`;SnapshotID string `json:"snapshotId"`
 OwnerID uint `json:"ownerId"`;SourceBase string `json:"sourceBase"`;DatabaseExportPath string `json:"databaseExportPath"`;Sources []string `json:"sources"`
 State string `json:"state"`;Message string `json:"message"`;CreatedAt time.Time `json:"createdAt"`;Files uint64 `json:"files"`;Bytes uint64 `json:"bytes"`
 ContentVerified bool `json:"contentVerified"`;DatabaseHeaderReadable bool `json:"databaseHeaderReadable"`;Available bool `json:"available"`
}
func privateRestoreBase(work string)(string,error){
 base:=filepath.Join(work,"restores");if err:=os.MkdirAll(base,0700);err!=nil{return "",err}
 info,err:=os.Lstat(base);if err!=nil{return "",err};if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{return "",errors.New("restore staging base must be private and not a symlink")};return base,nil
}
func eligibleRestoreSource(job Job,actor Actor)bool{
 return job.Action=="backup"&&job.State=="succeeded"&&job.DatabaseIncluded&&job.Actor.UserID==actor.UserID&&job.SourceBase!=""&&filepath.IsAbs(job.SourceBase)&&len(job.SnapshotID)==64&&snapshotPattern.MatchString(job.SnapshotID)&&filepath.IsAbs(job.DatabaseExportPath)&&filepath.Base(job.DatabaseExportPath)=="drive-state.db"&&len(job.Sources)>0
}
func(s *Service)BackupForRestore(id string,actor Actor)(Job,error){
 s.mu.Lock();defer s.mu.Unlock();job,err:=s.recoverySourceLocked(id);if err!=nil{return Job{},err};if !eligibleRestoreSource(job,actor){return Job{},ErrRestoreApproval};job.Sources=append([]string(nil),job.Sources...);return job,nil
}
func(s *Service)StartRestore(request StartRequest,actor Actor,approval RestoreApproval)(Job,error){return s.start(request,actor,&DataBackupPlan{restoreAccess:approval.Revalidate})}
func(s *Service)validateRestoreStart(c Config,actor Actor,request StartRequest,plan *DataBackupPlan)error{
 if !request.ConfirmRestore||plan==nil||plan.restoreAccess==nil||!restoreIDPattern.MatchString(request.RestoreFromJobID)||c.SourceOwner!=actor.UserID{return ErrRestoreApproval}
 source,err:=s.recoverySourceLocked(request.RestoreFromJobID);if err!=nil{return err}
 if !eligibleRestoreSource(source,actor)||source.DestinationID!=c.ID||source.SourceBase!=c.SourceBase{return ErrRestoreApproval}
 if len(s.artifacts)>=4{return errors.New("cleanup existing staged recoveries before creating more; limit is four")}
 source.Sources=append([]string(nil),source.Sources...);plan.restoreSource=source;return nil
}
// Called with the service lock held, before the asynchronous worker starts.
func(s *Service)registerRestore(id string,c Config,actor Actor,source Job)error{
 s.artifacts[id]=RestoreArtifact{ID:id,SourceJobID:source.ID,DestinationID:c.ID,SnapshotID:source.SnapshotID,OwnerID:actor.UserID,SourceBase:source.SourceBase,DatabaseExportPath:source.DatabaseExportPath,Sources:append([]string(nil),source.Sources...),State:"preparing",Message:"Restore requested; data may not yet be present.",CreatedAt:time.Now().UTC()}
 if err:=s.writeJSON("restore-artifacts.json",s.artifacts);err!=nil{delete(s.artifacts,id);return errors.New("could not persist restore intent; restore not started")};return nil
}
func(s *Service)RestoreArtifacts()[]RestoreArtifact{
 s.mu.Lock();defer s.mu.Unlock();out:=make([]RestoreArtifact,0,len(s.artifacts))
 for id,value:=range s.artifacts{
  value.Sources=append([]string(nil),value.Sources...)
  if s.driver!=nil{info,err:=os.Lstat(filepath.Join(s.driver.workDir,"restores",id));value.Available=err==nil&&info.IsDir()&&info.Mode()&os.ModeSymlink==0}
  out=append(out,value)
 }
 sort.Slice(out,func(i,j int)bool{return out[i].CreatedAt.After(out[j].CreatedAt)});return out
}
func(s *Service)CleanupRestore(id string,actor Actor)error{
 s.mu.Lock();defer s.mu.Unlock();if !restoreIDPattern.MatchString(id){return ErrNotFound};artifact,ok:=s.artifacts[id];if !ok{return ErrNotFound};if artifact.OwnerID!=actor.UserID{return os.ErrPermission};if s.active==id{return ErrBusy}
 if s.driver==nil{return ErrEngineUnavailable}
 base,err:=privateRestoreBase(s.driver.workDir);if err!=nil{return err}
 if err:=s.audit("restore.cleanup.requested",actor,artifact.DestinationID,id,"requested");err!=nil{return errors.New("cleanup audit failed; data not removed")}
 if err:=os.RemoveAll(filepath.Join(base,id));err!=nil{return errors.New("staging cleanup failed; private data may remain")}
 delete(s.artifacts,id)
 if err:=s.writeJSON("restore-artifacts.json",s.artifacts);err!=nil{artifact.State="cleanup-error";artifact.Message="Data removal completed but metadata persistence failed; retry cleanup.";s.artifacts[id]=artifact;return errors.New(artifact.Message)}
 if err:=s.audit("restore.cleaned",actor,artifact.DestinationID,id,"success");err!=nil{return errors.New("staged data removed but completion audit failed")};return nil
}
func restoreByteLimit()(uint64,error){
 const standard=uint64(5)<<30
 raw:=os.Getenv("DRIVE_RESTORE_MAX_BYTES");if raw==""{return standard,nil};value,err:=strconv.ParseUint(raw,10,64);if err!=nil||value==0||value>1<<40{return 0,errors.New("invalid server restore byte limit")};return value,nil
}
func restoreBudget(size,files,limit uint64)error{if size==0||files==0||size>limit||files>50000{return errors.New("restore exceeds byte/entry limits or returned invalid size information")};return nil}
func restoreFreeSpace(path string,size,files uint64)error{
 available,inodes,err:=restoreDiskSpace(path);if err!=nil{return err}
 required:=size+files*8192+(256<<20)
 if available<required||(inodes>0&&inodes<files+128){return errors.New("not enough free staging space or inodes with safety reserve")};return nil
}
func(s *Service)executeRestore(ctx context.Context,c Config,secrets Secrets,plan *DataBackupPlan)(result Result,runErr error){
 result=Result{Action:"restore-staging",ExitCode:-1}
 if plan==nil||plan.restoreAccess==nil||!restoreIDPattern.MatchString(plan.restoreID){return result,ErrRestoreApproval}
 ctx,timeoutCancel:=context.WithTimeout(ctx,6*time.Hour);defer timeoutCancel()
 base,err:=privateRestoreBase(s.driver.workDir);if err!=nil{return result,err};stage:=filepath.Join(base,plan.restoreID)
 // Registry survives job-history rollover and process restarts. Failed or
 // interrupted stages remain explicitly listed until the owner cleans them.
 defer func(){
  if runErr==nil{return}
  cleanupErr:=os.RemoveAll(stage)
  s.mu.Lock();defer s.mu.Unlock();artifact:=s.artifacts[plan.restoreID];artifact.State="failed";artifact.Message="Restore failed; partial staged data was removed. Cleanup removes this record."
  if cleanupErr!=nil{artifact.State="cleanup-failed";artifact.Message="Restore failed and partial private data may remain; request cleanup."}
  artifact.ContentVerified=false;s.artifacts[plan.restoreID]=artifact;_=s.writeJSON("restore-artifacts.json",s.artifacts)
 }()
 if err:=plan.restoreAccess(ctx);err!=nil{return result,err}
 limit,err:=restoreByteLimit();if err!=nil{return result,err}
 stats,err:=s.driver.Run(ctx,c,secrets,Request{Action:"restore-stats",Snapshot:plan.restoreSource.SnapshotID,Timeout:15*time.Minute});if err!=nil{return result,err}
 var counts struct{Size uint64 `json:"total_size"`;Files uint64 `json:"total_file_count"`}
 if err:=json.Unmarshal([]byte(stats.Stdout),&counts);err!=nil{return result,errors.New("restore size preflight could not be parsed")}
 if err:=restoreBudget(counts.Size,counts.Files,limit);err!=nil{return result,err};if err:=restoreFreeSpace(base,counts.Size,counts.Files);err!=nil{return result,err}
 if err:=plan.restoreAccess(ctx);err!=nil{return result,err}
 // The free-space watchdog is a best-effort safeguard, not a filesystem quota.
 child,cancel:=context.WithCancel(ctx);stop:=make(chan struct{});done:=make(chan struct{});var monitorErr error;var monitorMu sync.Mutex
 go func(){defer close(done);ticker:=time.NewTicker(2*time.Second);defer ticker.Stop();for{select{case <-stop:return;case <-child.Done():return;case <-ticker.C:
  err:=plan.restoreAccess(child)
  if err==nil{free,_,diskErr:=restoreDiskSpace(base);if diskErr!=nil{err=diskErr}else if free<128<<20{err=errors.New("staging free-space safety reserve reached")}}
  if err!=nil{monitorMu.Lock();monitorErr=err;monitorMu.Unlock();cancel();return}
 }}}()
 result,runErr=s.driver.Run(child,c,secrets,Request{Action:"restore",Snapshot:plan.restoreSource.SnapshotID,RestoreID:plan.restoreID,VerifyRestore:true,Timeout:6*time.Hour})
 close(stop);<-done;cancel();monitorMu.Lock();watchError:=monitorErr;monitorMu.Unlock()
 if ctx.Err()!=nil{return result,ctx.Err()};if watchError!=nil{return result,watchError};if runErr!=nil{return result,runErr}
 if err:=os.Chmod(stage,0700);err!=nil{return result,err}
 files,bytes,err:=verifyStagedRestore(ctx,stage,plan.restoreSource,limit);if err!=nil{return result,err}
 if err:=plan.restoreAccess(ctx);err!=nil{return result,err}
 s.mu.Lock();defer s.mu.Unlock();artifact:=s.artifacts[plan.restoreID];artifact.State="staged";artifact.Message="Content verified against snapshot; matching manifest and readable database header. Live data unchanged.";artifact.Files=files;artifact.Bytes=bytes;artifact.ContentVerified=true;artifact.DatabaseHeaderReadable=true;s.artifacts[plan.restoreID]=artifact
 if err:=s.writeJSON("restore-artifacts.json",s.artifacts);err!=nil{return result,errors.New("restore completion metadata could not be persisted")}
 return result,nil
}
func readableDatabaseHeader(path string)(err error){
 defer func(){if recover()!=nil{err=errors.New("restored database header could not be read")}}()
 database,err:=bbolt.Open(path,0600,&bbolt.Options{ReadOnly:true,Timeout:time.Second});if err!=nil{return errors.New("restored database header is not readable")};return database.Close()
}
func verifyStagedRestore(ctx context.Context,stage string,source Job,limit uint64)(uint64,uint64,error){
 var files,total,entries uint64
 err:=filepath.WalkDir(stage,func(path string,entry os.DirEntry,walkErr error)error{
  if err:=ctx.Err();err!=nil{return err};if walkErr!=nil{return walkErr};entries++;if entries>50000{return errors.New("restored entry count exceeds limit")}
  info,err:=entry.Info();if err!=nil{return err};if info.Mode()&os.ModeSymlink!=0{return errors.New("initial staging restore does not accept symlinks")};if info.IsDir(){return nil};if !info.Mode().IsRegular(){return errors.New("special restored files are not supported")}
  files++;size:=uint64(info.Size());if size>limit||total>limit-size{return errors.New("restored byte count exceeds limit")};total+=size;return nil
 });if err!=nil{return 0,0,err}
 if !filepath.IsAbs(source.DatabaseExportPath)||filepath.Base(source.DatabaseExportPath)!="drive-state.db"{return 0,0,ErrRestoreApproval}
 databasePath:=filepath.Join(stage,strings.TrimPrefix(filepath.Clean(source.DatabaseExportPath),string(os.PathSeparator)))
 if !within(databasePath,stage){return 0,0,ErrRestoreApproval}
 manifestPath:=filepath.Join(filepath.Dir(databasePath),"drive-manifest.json")
 info,err:=os.Stat(manifestPath);if err!=nil||info.Size()>64<<10{return 0,0,errors.New("recovery manifest missing or too large")}
 raw,err:=os.ReadFile(manifestPath);if err!=nil{return 0,0,err}
 var manifest struct{Schema int `json:"schema"`;Database string `json:"database"`;Sources []string `json:"driveSources"`}
 if json.Unmarshal(raw,&manifest)!=nil||manifest.Schema!=1||manifest.Database!="drive-state.db"||len(manifest.Sources)!=len(source.Sources){return 0,0,errors.New("recovery manifest does not match recorded backup")}
 for i,virtual:=range source.Sources{if manifest.Sources[i]!=virtual{return 0,0,errors.New("recovery source mapping differs from recorded backup")}}
 if err:=readableDatabaseHeader(databasePath);err!=nil{return 0,0,err}
 return files,total,nil
}
