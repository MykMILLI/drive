package drivebackup

import (
 "context"
 "crypto/sha256"
 "encoding/base64"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "path/filepath"
 "time"
 bbolt "go.etcd.io/bbolt"
)
const catalogLimit=50000
var catalogJobs=[]byte("jobs")
var catalogTimeline=[]byte("timeline")
type CatalogEntry struct{Schema int `json:"schema"`;Tagged bool `json:"tagged"`;Source Job `json:"source"`}
type CatalogPage struct{Entries []CatalogEntry `json:"entries"`;NextCursor string `json:"nextCursor,omitempty"`}
func backupSetID(destination string,owner uint,scope string)string{
 raw,_:=json.Marshal([]any{"drive-backup-set-v1",destination,owner,filepath.Clean(scope)})
 sum:=sha256.Sum256(raw);return hex.EncodeToString(sum[:])
}
func(c Config)BackupSetID()string{if c.SourceOwner==0||!filepath.IsAbs(c.SourceBase){return ""};return backupSetID(c.ID,c.SourceOwner,c.SourceBase)}
func(c Config)Revision()string{raw,_:=json.Marshal(c);sum:=sha256.Sum256(raw);return hex.EncodeToString(sum[:])}
func(s *Service)withCatalog(readOnly bool,fn func(*bbolt.DB)error)error{
 path:=filepath.Join(s.dir,"recovery-catalog.db")
 if info,err:=os.Lstat(path);err==nil{if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0{return errors.New("recovery catalog must be a private regular file")}}else if !os.IsNotExist(err){return err}
 db,err:=bbolt.Open(path,0600,&bbolt.Options{ReadOnly:readOnly,Timeout:time.Second});if err!=nil{return errors.New("recovery catalog unavailable")}
 defer db.Close();return fn(db)
}
func catalogSource(job Job)Job{
 return Job{ID:job.ID,DestinationID:job.DestinationID,Action:"backup",State:"succeeded",ExitCode:0,CreatedAt:job.CreatedAt,FinishedAt:job.FinishedAt,Actor:Actor{UserID:job.Actor.UserID,Username:limitText(job.Actor.Username,256)},SourceBase:job.SourceBase,DatabaseExportPath:job.DatabaseExportPath,Sources:append([]string(nil),job.Sources...),SnapshotID:job.SnapshotID,DatabaseIncluded:true,FilesProcessed:job.FilesProcessed,BytesProcessed:job.BytesProcessed,StreamID:job.StreamID,ScheduleID:job.ScheduleID,ScheduleWindow:job.ScheduleWindow,Message:"Successful backup retained in durable recovery catalog."}
}
func putCatalog(tx *bbolt.Tx,job Job)error{
 if !eligibleRestoreSource(job,job.Actor)||!restoreIDPattern.MatchString(job.ID)||job.CreatedAt.IsZero()||job.CreatedAt.UnixNano()<0{return errors.New("backup cannot be safely cataloged")}
 jobs:=tx.Bucket(catalogJobs);timeline:=tx.Bucket(catalogTimeline)
 if existing:=jobs.Get([]byte(job.ID));existing!=nil{
  var entry CatalogEntry
  if json.Unmarshal(existing,&entry)!=nil||entry.Source.SnapshotID!=job.SnapshotID||entry.Source.SourceBase!=job.SourceBase||entry.Source.Actor.UserID!=job.Actor.UserID{return errors.New("recovery catalog immutable identity mismatch")};return nil
 }
 if jobs.Stats().KeyN>=catalogLimit{return errors.New("recovery catalog capacity reached")}
 expected:=backupSetID(job.DestinationID,job.Actor.UserID,job.SourceBase)
 entry:=CatalogEntry{Schema:1,Tagged:job.StreamID==expected,Source:catalogSource(job)}
 raw,err:=json.Marshal(entry);if err!=nil{return err};if len(raw)>128<<10{return errors.New("backup catalog record too large")}
 if err:=jobs.Put([]byte(job.ID),raw);err!=nil{return err}
 key:=make([]byte,8,40);binary.BigEndian.PutUint64(key,uint64(job.CreatedAt.UnixNano()));key=append(key,[]byte(job.ID)...)
 return timeline.Put(key,[]byte(job.ID))
}
func(s *Service)initCatalog()error{
 err:=s.withCatalog(false,func(db *bbolt.DB)error{return db.Update(func(tx *bbolt.Tx)error{
  if _,err:=tx.CreateBucketIfNotExists(catalogJobs);err!=nil{return err};if _,err:=tx.CreateBucketIfNotExists(catalogTimeline);err!=nil{return err}
  for _,job:=range s.jobs{if eligibleRestoreSource(job,job.Actor){if err:=putCatalog(tx,job);err!=nil{return err}}};return nil
 })})
 if err!=nil{return err};return s.syncDirectory()
}
func(s *Service)storeCatalog(job Job)error{return s.withCatalog(false,func(db *bbolt.DB)error{return db.Update(func(tx *bbolt.Tx)error{return putCatalog(tx,job)})})}
func(s *Service)catalogCapacity()error{return s.withCatalog(true,func(db *bbolt.DB)error{return db.View(func(tx *bbolt.Tx)error{if tx.Bucket(catalogJobs).Stats().KeyN>=catalogLimit{return errors.New("recovery catalog is full; backup was not started")};return nil})})}
// Caller holds service.mu; durable catalog lookup remains available after the
// bounded recent-job list no longer contains the source backup.
func(s *Service)recoverySourceLocked(id string)(Job,error){
 if !restoreIDPattern.MatchString(id){return Job{},ErrNotFound}
 var source Job;found:=false
 err:=s.withCatalog(true,func(db *bbolt.DB)error{return db.View(func(tx *bbolt.Tx)error{raw:=tx.Bucket(catalogJobs).Get([]byte(id));if raw==nil{return nil};var entry CatalogEntry;if json.Unmarshal(raw,&entry)!=nil||entry.Schema!=1||entry.Source.ID!=id{return errors.New("invalid recovery catalog entry")};source=entry.Source;found=true;return nil})})
 if err!=nil{return Job{},err};if found{return source,nil}
 i:=s.jobIndex(id);if i<0{return Job{},ErrNotFound};return s.jobs[i],nil
}
func(s *Service)Catalog(ctx context.Context,owner uint,scope,cursor string,limit int)(CatalogPage,error){
 s.mu.Lock();defer s.mu.Unlock();out:=CatalogPage{Entries:make([]CatalogEntry,0)}
 if err:=ctx.Err();err!=nil{return out,err}
 if owner==0||!filepath.IsAbs(scope)||limit<1||limit>100{return out,errors.New("invalid catalog query")}
 var before []byte
 if cursor!=""{var err error;before,err=base64.RawURLEncoding.DecodeString(cursor);if err!=nil||len(before)!=40||!restoreIDPattern.Match(before[8:]){return out,errors.New("invalid catalog cursor")}}
 err:=s.withCatalog(true,func(db *bbolt.DB)error{return db.View(func(tx *bbolt.Tx)error{
  items:=tx.Bucket(catalogJobs);scan:=tx.Bucket(catalogTimeline).Cursor();key,value:=scan.Last()
  if len(before)>0{key,value=scan.Seek(before);if key==nil{key,value=scan.Last()}else{key,value=scan.Prev()}}
  var last []byte;examined:=0
  for key!=nil&&examined<512&&len(out.Entries)<limit{
   if err:=ctx.Err();err!=nil{return err};examined++;last=append(last[:0],key...)
   raw:=items.Get(value);var entry CatalogEntry
   if raw==nil||json.Unmarshal(raw,&entry)!=nil||entry.Schema!=1{return errors.New("invalid recovery catalog entry")}
   if entry.Source.Actor.UserID==owner&&entry.Source.SourceBase==scope{entry.Source.Sources=append([]string(nil),entry.Source.Sources...);out.Entries=append(out.Entries,entry)}
   key,value=scan.Prev()
  }
  if key!=nil&&len(last)>0{out.NextCursor=base64.RawURLEncoding.EncodeToString(last)};return nil
 })});return out,err
}
func(s *Service)ExportCatalog(ctx context.Context,w io.Writer)error{
 return s.withCatalog(true,func(db *bbolt.DB)error{return db.View(func(tx *bbolt.Tx)error{_,err:=tx.WriteTo(&exportLimitWriter{ctx:ctx,writer:w});return err})})
}
func(s *Service)Active()bool{s.mu.Lock();defer s.mu.Unlock();return s.active!=""}
func(s *Service)AuditAutomation(action string,actor Actor,destination,id,outcome string)error{s.mu.Lock();defer s.mu.Unlock();return s.audit(action,actor,destination,id,outcome)}
func(s *Service)Shutdown(ctx context.Context)error{
 s.mu.Lock();s.stopping=true;if s.cancel!=nil{s.cancel()};s.mu.Unlock()
 ticker:=time.NewTicker(25*time.Millisecond);defer ticker.Stop()
 for{if !s.Active(){return nil};select{case <-ctx.Done():return ctx.Err();case <-ticker.C:}}
}
