package fbhttp

import (
 "encoding/json"
 "errors"
 "io"
 "net"
 "net/http"
 "os"
 "path/filepath"
 "sync"

 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
)
var backupServices=struct{sync.Mutex;items map[string]*drivebackup.Service}{items:map[string]*drivebackup.Service{}}
func backupService()(*drivebackup.Service,error){
 dir:=os.Getenv("DRIVE_BACKUP_STATE_DIR");if dir==""{dir="/config/drive-backups"}
 dir=filepath.Clean(dir)
 backupServices.Lock();defer backupServices.Unlock()
 if service:=backupServices.items[dir];service!=nil{return service,nil}
 service,err:=drivebackup.NewService(dir);if err!=nil{return nil,err};backupServices.items[dir]=service;return service,nil
}
func backupActor(r *http.Request,d *data)drivebackup.Actor{address,_,err:=net.SplitHostPort(r.RemoteAddr);if err!=nil{address=r.RemoteAddr};return drivebackup.Actor{UserID:d.user.ID,Username:d.user.Username,Address:address}}
func backupBody(w http.ResponseWriter,r *http.Request,value any)error{
 if r.Body==nil{return errors.New("missing request body")};r.Body=http.MaxBytesReader(w,r.Body,64<<10);decoder:=json.NewDecoder(r.Body);decoder.DisallowUnknownFields();if err:=decoder.Decode(value);err!=nil{return errors.New("invalid backup request")};var extra any;if decoder.Decode(&extra)!=io.EOF{return errors.New("invalid trailing request data")};return nil
}
func backupError(err error)(int,error){switch{case errors.Is(err,os.ErrPermission):return http.StatusForbidden,nil;case errors.Is(err,drivebackup.ErrBusy):return http.StatusConflict,err;case errors.Is(err,drivebackup.ErrNotFound):return http.StatusNotFound,nil;case errors.Is(err,drivebackup.ErrEngineUnavailable):return http.StatusServiceUnavailable,err;default:return http.StatusBadRequest,err}}
var backupDestinationsHandler=withAdmin(func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 service,err:=backupService();if err!=nil{return http.StatusServiceUnavailable,err}
 if r.Method==http.MethodGet{destinations,err:=service.Destinations();if err!=nil{return http.StatusInternalServerError,err};return renderJSON(w,r,map[string]any{"destinations":destinations,"engineAvailable":service.EngineAvailable(),"dataBackupAvailable":d.store.ExportDatabase!=nil})}
 var input struct{Config drivebackup.Config `json:"config"`;Secrets drivebackup.SecretUpdate `json:"secrets"`}
 if err:=backupBody(w,r,&input);err!=nil{return http.StatusBadRequest,err}
 if input.Config.Backend!="s3"||input.Config.LocalRepository!=""{return http.StatusBadRequest,errors.New("administrator destination API accepts S3 destinations only")}
 sources,base,err:=validateBackupSourcesForSave(d,input.Config.SourcePaths);if err!=nil{return http.StatusBadRequest,err}
 input.Config.SourcePaths=sources;input.Config.SourceOwner=d.user.ID;input.Config.SourceBase=base
 destination,err:=service.Save(input.Config,input.Secrets,backupActor(r,d));if err!=nil{return backupError(err)}
 return renderJSON(w,r,destination)
})
var backupJobsHandler=withAdmin(func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 service,err:=backupService();if err!=nil{return http.StatusServiceUnavailable,err}
 if r.Method==http.MethodGet{return renderJSON(w,r,service.Jobs())}
 if r.Method==http.MethodDelete{id:=r.URL.Query().Get("id");if len(id)!=32{return http.StatusBadRequest,nil};if err:=service.Cancel(id,backupActor(r,d));err!=nil{return backupError(err)};return renderJSON(w,r,map[string]bool{"cancelRequested":true})}
 var input drivebackup.StartRequest;if err:=backupBody(w,r,&input);err!=nil{return http.StatusBadRequest,err}
 var job drivebackup.Job
 if input.Action=="backup"{job,err=startDriveDataBackup(service,w,r,d,input)}else if input.Action=="restore-staging"{job,err=startDriveStagingRestore(service,r,d,input)}else{job,err=service.Start(input,backupActor(r,d))};if err!=nil{return backupError(err)}
 return renderJSON(w,r,job)
})
