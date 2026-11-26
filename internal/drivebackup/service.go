package drivebackup

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"
 "time"
)
var ErrNotFound=errors.New("backup record not found")
var ErrEngineUnavailable=errors.New("restic is not installed or the private work directory is unavailable")
var ErrConfirmation=errors.New("initialization requires explicit write and recovery-password acknowledgement")

type Actor struct { UserID uint `json:"userId"`; Username string `json:"username"`; Address string `json:"address"` }
type Destination struct { Config Config `json:"config"`; Secrets SecretState `json:"secrets"` }
type SecretUpdate struct {
 AccessKeyID *string `json:"accessKeyId"`
 SecretAccessKey *string `json:"secretAccessKey"`
 SessionToken *string `json:"sessionToken"`
 RepositoryPassword *string `json:"repositoryPassword"`
}
type SnapshotView struct { ID string `json:"id"`; Time string `json:"time"`; Hostname string `json:"hostname"` }
type Job struct {
 StreamID string `json:"streamId,omitempty"`; ScheduleID string `json:"scheduleId,omitempty"`; ScheduleWindow string `json:"scheduleWindow,omitempty"`
 SourceBase string `json:"sourceBase,omitempty"`; DatabaseExportPath string `json:"databaseExportPath,omitempty"`; RestoreArtifactID string `json:"restoreArtifactId,omitempty"`
 Sources []string `json:"sources,omitempty"`; SnapshotID string `json:"snapshotId,omitempty"`; DatabaseIncluded bool `json:"databaseIncluded,omitempty"`; FilesProcessed uint64 `json:"filesProcessed,omitempty"`; BytesProcessed uint64 `json:"bytesProcessed,omitempty"`
 ID string `json:"id"`; DestinationID string `json:"destinationId"`; Action string `json:"action"`
 State string `json:"state"`; Message string `json:"message"`; ExitCode int `json:"exitCode"`
 CreatedAt time.Time `json:"createdAt"`; StartedAt time.Time `json:"startedAt,omitempty"`; FinishedAt time.Time `json:"finishedAt,omitempty"`
 Actor Actor `json:"actor"`; Snapshots []SnapshotView `json:"snapshots,omitempty"`; SnapshotListTruncated bool `json:"snapshotListTruncated,omitempty"`
}
type StartRequest struct {
 RestoreFromJobID string `json:"restoreFromJobId,omitempty"`; ConfirmRestore bool `json:"confirmRestore"`
 ConfirmBackup bool `json:"confirmBackup"`
 DestinationID string `json:"destinationId"`; Action string `json:"action"`; IntegrityPercent int `json:"integrityPercent"`
 ConfirmInitialize bool `json:"confirmInitialize"`; RecoveryAcknowledged bool `json:"recoveryAcknowledged"`
}
type Service struct {
 mu sync.Mutex; dir string; vault *Vault; driver *Driver; stopping bool
 artifacts map[string]RestoreArtifact
 destinations map[string]Config; blocked map[string]bool; jobs []Job; active string; cancel context.CancelFunc
}
func randomID()(string,error){var value [16]byte;if _,err:=rand.Read(value[:]);err!=nil{return "",err};return hex.EncodeToString(value[:]),nil}
func NewService(dir string)(*Service,error){
 if !filepath.IsAbs(dir){return nil,errors.New("backup state path must be absolute")}
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 info,err:=os.Lstat(dir);if err!=nil{return nil,err};if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{return nil,errors.New("backup state directory must be private (0700) and not a symlink")}
 vault,err:=NewVault(filepath.Join(dir,"vault"));if err!=nil{return nil,err}
 s:=&Service{dir:dir,vault:vault,destinations:map[string]Config{},blocked:map[string]bool{},jobs:make([]Job,0),artifacts:map[string]RestoreArtifact{}}
 for _,item:=range []struct{name string; target any}{{"destinations.json",&s.destinations},{"jobs.json",&s.jobs},{"restore-artifacts.json",&s.artifacts}}{
  raw,err:=os.ReadFile(filepath.Join(dir,item.name));if os.IsNotExist(err){continue};if err!=nil{return nil,err};if len(raw)>4<<20{return nil,errors.New("backup metadata exceeds safety limit")};if err:=json.Unmarshal(raw,item.target);err!=nil{return nil,errors.New("invalid backup metadata; restore or repair it before proceeding")}
 }
 if s.destinations==nil{s.destinations=map[string]Config{}}
 if s.jobs==nil{s.jobs=make([]Job,0)}
 if s.artifacts==nil{s.artifacts=map[string]RestoreArtifact{}}
 if len(s.artifacts)>4{return nil,errors.New("restore artifact limit exceeded; inspect private state")}
 artifactChanged:=false
 for id,artifact:=range s.artifacts{if !restoreIDPattern.MatchString(id)||id!=artifact.ID{return nil,errors.New("invalid restore artifact metadata")};if artifact.State=="preparing"{artifact.State="interrupted";artifact.Message="Server restarted; partial staged data may remain. Cleanup is available.";s.artifacts[id]=artifact;artifactChanged=true}}
 if artifactChanged{if err:=s.writeJSON("restore-artifacts.json",s.artifacts);err!=nil{return nil,err}}
 pending,err:=filepath.Glob(filepath.Join(dir,".pending-destination-*.json"));if err!=nil{return nil,err}
 for _,path:=range pending{id:=strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path),".pending-destination-"),".json");if validID.MatchString(id){s.blocked[id]=true}}
 changed:=false
 for i:=range s.jobs{if s.jobs[i].State=="queued"||s.jobs[i].State=="running"||s.jobs[i].State=="canceling"{s.jobs[i].State="interrupted";s.jobs[i].Message="Server restarted; operation outcome must be checked before retrying.";s.jobs[i].FinishedAt=time.Now().UTC();changed=true}}
 if changed{if err:=s.writeJSON("jobs.json",s.jobs);err!=nil{return nil,err}}
 // Configuration remains accessible when restic is absent; job starts fail closed.
 s.driver,_=NewDriver(filepath.Join(dir,"work"),nil)
 if err:=s.initCatalog();err!=nil{return nil,err}
 return s,nil
}
func(s *Service)writeJSON(name string,value any)error{
 raw,err:=json.MarshalIndent(value,"","  ");if err!=nil{return err}
 file,err:=os.CreateTemp(s.dir,".metadata-");if err!=nil{return err};temp:=file.Name();defer os.Remove(temp)
 if err:=file.Chmod(0600);err!=nil{file.Close();return err};_,err=file.Write(raw);if err==nil{err=file.Sync()};closeErr:=file.Close();if err!=nil{return err};if closeErr!=nil{return closeErr}
 if err:=os.Rename(temp,filepath.Join(s.dir,name));err!=nil{return err}
 return s.syncDirectory()
}
func(s *Service)syncDirectory()error{dir,err:=os.Open(s.dir);if err!=nil{return err};defer dir.Close();return dir.Sync()}
func(s *Service)audit(action string,actor Actor,destination,job,outcome string)error{
 actor.Username=limitText(actor.Username,256);actor.Address=limitText(actor.Address,256)
 path:=filepath.Join(s.dir,"audit.jsonl")
 if info,err:=os.Lstat(path);err==nil{
  if !info.Mode().IsRegular(){return errors.New("invalid audit file")}
  if info.Size()>4<<20{
   _=os.Remove(filepath.Join(s.dir,"audit.4.jsonl"))
   for n:=3;n>=1;n--{old:=filepath.Join(s.dir,"audit."+string(rune('0'+n))+".jsonl");next:=filepath.Join(s.dir,"audit."+string(rune('0'+n+1))+".jsonl");if err:=os.Rename(old,next);err!=nil&&!os.IsNotExist(err){return err}}
   if err:=os.Rename(path,filepath.Join(s.dir,"audit.1.jsonl"));err!=nil{return err}
  }
 }else if !os.IsNotExist(err){return err}
 raw,err:=json.Marshal(map[string]any{"time":time.Now().UTC(),"action":action,"actor":actor,"destinationId":destination,"jobId":job,"outcome":outcome});if err!=nil{return err}
 file,err:=os.OpenFile(path,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{return err};defer file.Close();if _,err:=file.Write(append(raw,'\n'));err!=nil{return err};return file.Sync()
}
func limitText(value string,max int)string{if len(value)>max{return value[:max]};return value}
func(s *Service)EngineAvailable()bool{return s.driver!=nil}
func(s *Service)Destinations()([]Destination,error){
 s.mu.Lock();defer s.mu.Unlock();out:=make([]Destination,0,len(s.destinations))
 for id,c:=range s.destinations{secrets,err:=s.vault.Load(id);if err!=nil{return nil,errors.New("stored destination credentials could not be decrypted")};if s.blocked[id]{c.Enabled=false};out=append(out,Destination{c,secrets.State()})}
 sort.Slice(out,func(i,j int)bool{return out[i].Config.Name<out[j].Config.Name});return out,nil
}
func(s *Service)Save(c Config,update SecretUpdate,actor Actor)(Destination,error){
 s.mu.Lock();defer s.mu.Unlock()
 if c.ID==""{id,err:=randomID();if err!=nil{return Destination{},err};c.ID=id}
 if !validID.MatchString(c.ID){return Destination{},errors.New("invalid destination ID")}
 if strings.TrimSpace(c.Name)==""{return Destination{},errors.New("destination name is required")}
 old,exists:=s.destinations[c.ID]
 if !exists&&len(s.destinations)>=32{return Destination{},errors.New("destination limit reached")}
 if s.active!=""{for _,job:=range s.jobs{if job.ID==s.active&&job.DestinationID==c.ID{return Destination{},ErrBusy}}}
 var secrets Secrets
 if exists{var err error;secrets,err=s.vault.Load(c.ID);if err!=nil{return Destination{},errors.New("stored credentials could not be decrypted")}}
 changedLocation:=exists&&(old.Backend!=c.Backend||old.Endpoint!=c.Endpoint||old.Bucket!=c.Bucket||old.Prefix!=c.Prefix||old.LocalRepository!=c.LocalRepository)||s.blocked[c.ID]
 if changedLocation{
  if update.RepositoryPassword==nil||(c.Backend=="s3"&&(update.AccessKeyID==nil||update.SecretAccessKey==nil)){return Destination{},errors.New("destination location changed; re-enter its keys and repository password")}
  secrets=Secrets{}
 }
 if update.AccessKeyID!=nil{secrets.AccessKeyID=*update.AccessKeyID};if update.SecretAccessKey!=nil{secrets.SecretAccessKey=*update.SecretAccessKey};if update.SessionToken!=nil{secrets.SessionToken=*update.SessionToken};if update.RepositoryPassword!=nil{secrets.RepositoryPassword=*update.RepositoryPassword}
 if err:=c.Validate(secrets);err!=nil{return Destination{},err}
 // Intent is durably audited before credentials/config are changed. A later
 // storage error may leave encrypted orphan secrets, never an automatic job.
 if err:=s.audit("destination.save.requested",actor,c.ID,"","requested");err!=nil{return Destination{},errors.New("audit recording failed; settings not changed")}
 marker:=".pending-destination-"+c.ID+".json"
 if err:=s.writeJSON(marker,map[string]any{"destinationId":c.ID,"time":time.Now().UTC()});err!=nil{return Destination{},errors.New("could not record credential transaction; settings not changed")}
 s.blocked[c.ID]=true
 if err:=s.vault.Save(c.ID,secrets);err!=nil{return Destination{},errors.New("could not persist encrypted credentials")}
 s.destinations[c.ID]=c
 if err:=s.writeJSON("destinations.json",s.destinations);err!=nil{if exists{s.destinations[c.ID]=old}else{delete(s.destinations,c.ID)};return Destination{},errors.New("could not persist destination metadata; inspect state before retrying")}
 if err:=s.audit("destination.saved",actor,c.ID,"","success");err!=nil{return Destination{},errors.New("settings were saved but completion audit failed; refresh before retrying")}
 if err:=os.Remove(filepath.Join(s.dir,marker));err!=nil{return Destination{},errors.New("settings saved but transaction cleanup failed; refresh and re-enter credentials before running jobs")}
 if err:=s.syncDirectory();err!=nil{return Destination{},errors.New("settings saved but transaction durability check failed; refresh before running jobs")}
 delete(s.blocked,c.ID)
 return Destination{c,secrets.State()},nil
}
func(s *Service)Jobs()[]Job{s.mu.Lock();defer s.mu.Unlock();out:=make([]Job,len(s.jobs));copy(out,s.jobs);return out}
func(s *Service)Start(request StartRequest,actor Actor)(Job,error){return s.start(request,actor,nil)}
func(s *Service)StartBackup(request StartRequest,actor Actor,plan DataBackupPlan)(Job,error){
 plan.Sources=append([]string(nil),plan.Sources...);plan.DisplaySources=append([]string(nil),plan.DisplaySources...);return s.start(request,actor,&plan)
}
func(s *Service)start(request StartRequest,actor Actor,plan *DataBackupPlan)(Job,error){
 s.mu.Lock();defer s.mu.Unlock()
 if s.stopping||s.driver==nil{return Job{},ErrEngineUnavailable};if s.active!=""{return Job{},ErrBusy}
 c,ok:=s.destinations[request.DestinationID];if !ok{return Job{},ErrNotFound};if s.blocked[c.ID]{return Job{},errors.New("destination configuration transaction is incomplete; re-enter credentials and save before running jobs")}
 secrets,err:=s.vault.Load(c.ID);if err!=nil{return Job{},errors.New("stored credentials could not be decrypted")}
 if err:=c.Validate(secrets);err!=nil{return Job{},err};if !c.Enabled{return Job{},ErrDisabled}
 if request.Action!="initialize"&&request.Action!="snapshots"&&request.Action!="check"&&request.Action!="backup"&&request.Action!="restore-staging"{return Job{},errors.New("this release permits initialize, snapshots, check and confirmed data backup and private staging restore only")}
 if request.Action=="backup"{if err:=s.catalogCapacity();err!=nil{return Job{},err};if plan!=nil&&plan.expectedConfigRevision!=""&&plan.expectedConfigRevision!=c.Revision(){return Job{},ErrScheduleConflict};if !request.ConfirmBackup{return Job{},ErrBackupPlan};if err:=s.validateDataPlan(c,actor,plan);err!=nil{return Job{},err}}
 if request.Action=="restore-staging"{if err:=s.validateRestoreStart(c,actor,request,plan);err!=nil{return Job{},err}}
 if request.IntegrityPercent<0||request.IntegrityPercent>100{return Job{},errors.New("invalid integrity percentage")}
 if request.Action=="initialize"&&(!request.ConfirmInitialize||!request.RecoveryAcknowledged){return Job{},ErrConfirmation}
 id,err:=randomID();if err!=nil{return Job{},err}
 sourceNames:=[]string(nil);if request.Action=="backup"{sourceNames=append([]string(nil),plan.DisplaySources...)}
 job:=Job{Sources:sourceNames,ID:id,DestinationID:c.ID,Action:request.Action,State:"queued",ExitCode:-1,CreatedAt:time.Now().UTC(),Actor:actor,Message:"Waiting for repository operation."}
 if request.Action=="backup"{job.SourceBase=c.SourceBase;job.StreamID=c.BackupSetID();job.ScheduleID=plan.scheduleID;job.ScheduleWindow=plan.scheduleWindow}
 if request.Action=="restore-staging"{job.RestoreArtifactID=id;job.SourceBase=c.SourceBase;job.SnapshotID=plan.restoreSource.SnapshotID}
 if err:=s.audit("job.requested",actor,c.ID,id,request.Action);err!=nil{return Job{},errors.New("audit recording failed; operation not started")}
 if request.Action=="restore-staging"{if err:=s.registerRestore(id,c,actor,plan.restoreSource);err!=nil{return Job{},err};plan.restoreID=id}
 s.jobs=append([]Job{job},s.jobs...);if len(s.jobs)>100{s.jobs=s.jobs[:100]}
 if err:=s.writeJSON("jobs.json",s.jobs);err!=nil{s.jobs=s.jobs[1:];return Job{},errors.New("could not persist job; operation not started")}
 ctx,cancel:=context.WithCancel(context.Background());s.active=id;s.cancel=cancel
 go s.run(ctx,c,secrets,request,id,plan)
 return job,nil
}
func(s *Service)run(ctx context.Context,c Config,secrets Secrets,request StartRequest,id string,plan *DataBackupPlan){
 s.mu.Lock();index:=s.jobIndex(id)
 if index<0{s.active="";s.cancel=nil;s.mu.Unlock();return}
 s.jobs[index].State="running";s.jobs[index].StartedAt=time.Now().UTC();actor:=s.jobs[index].Actor
 if err:=s.writeJSON("jobs.json",s.jobs);err!=nil{s.jobs[index].State="failed";s.jobs[index].Message="Job-state recording failed; operation was not executed.";s.active="";if s.cancel!=nil{s.cancel()};s.cancel=nil;s.mu.Unlock();return}
 if err:=s.audit("job.started",actor,c.ID,id,request.Action);err!=nil{s.jobs[index].State="failed";s.jobs[index].Message="Audit recording failed; operation was not executed.";_=s.writeJSON("jobs.json",s.jobs);s.active="";if s.cancel!=nil{s.cancel()};s.cancel=nil;s.mu.Unlock();return}
 s.mu.Unlock()
 result,runErr:=s.executeJob(ctx,c,secrets,request,plan)
 s.mu.Lock();defer s.mu.Unlock();index=s.jobIndex(id);if index<0{return}
 job:=&s.jobs[index];job.ExitCode=result.ExitCode;job.FinishedAt=time.Now().UTC()
 switch{
 case errors.Is(runErr,context.Canceled):job.State="canceled";job.Message="Operation canceled. Verify repository state before retrying a write operation."
 case errors.Is(runErr,context.DeadlineExceeded):job.State="failed";job.Message="Operation timed out."
 case runErr!=nil:job.State="failed";job.Message="Repository operation failed; exit code "+integer(result.ExitCode)+". Check configuration, initialization, credentials, network and provider permissions."
 default:job.State="succeeded";job.Message="Repository operation completed. This does not mean Drive data has been backed up."
 }
 if request.Action=="backup"{finalizeDataBackup(job,result,runErr)}
 if request.Action=="restore-staging"{if runErr==nil{job.Message="Restored to private staging with restic content verification, matching manifest and readable database header. Live data was not replaced."}else if !errors.Is(runErr,context.Canceled){job.Message="Staging restore failed or a safety limit was reached. Check staged-recovery status; partial data cleanup was attempted."}}
 if runErr==nil&&request.Action=="snapshots"{
  var views []SnapshotView
  if err:=json.Unmarshal([]byte(result.Stdout),&views);err!=nil{job.State="failed";job.Message="Snapshot listing could not be parsed."}else{
   if len(views)>50{views=views[:50];job.SnapshotListTruncated=true}
   for i:=range views{views[i].ID=limitText(views[i].ID,64);views[i].Hostname=limitText(views[i].Hostname,256);views[i].Time=limitText(views[i].Time,64)}
   job.Snapshots=views
  }
 }
 // Raw restic stdout/stderr are deliberately NOT persisted or exposed.
 if err:=s.audit("job.finished",actor,c.ID,id,job.State);err!=nil{job.State="recording-error";job.Message="Operation finished but audit recording failed. Inspect repository state before retrying."}
 if job.Action=="backup"&&job.State=="succeeded"{if err:=s.storeCatalog(*job);err!=nil{job.State="recording-error";job.Message="Snapshot created, but durable recovery catalog recording failed. Inspect repository and catalog before retrying.";_=s.audit("catalog.record.failed",actor,c.ID,id,"recording-error")}}
 if err:=s.writeJSON("jobs.json",s.jobs);err!=nil{job.State="recording-error";job.Message="Operation finished but job-history persistence failed. Inspect repository state before retrying."}
 s.active="";if s.cancel!=nil{s.cancel()};s.cancel=nil
}
func integer(n int)string{if n<0{return "unknown"};if n==0{return "0"};var out [20]byte;i:=len(out);for n>0{i--;out[i]=byte('0'+n%10);n/=10};return string(out[i:])}
func(s *Service)jobIndex(id string)int{for i:=range s.jobs{if s.jobs[i].ID==id{return i}};return -1}
func(s *Service)Cancel(id string,actor Actor)error{
 s.mu.Lock();defer s.mu.Unlock()
 if s.active!=id||s.cancel==nil{return ErrNotFound}
 if err:=s.audit("job.cancel.requested",actor,"",id,"requested");err!=nil{return errors.New("audit recording failed; cancellation not submitted")}
 index:=s.jobIndex(id);if index>=0{s.jobs[index].State="canceling";s.jobs[index].Message="Cancellation requested; waiting for restic to exit.";_=s.writeJSON("jobs.json",s.jobs)}
 s.cancel();return nil
}
