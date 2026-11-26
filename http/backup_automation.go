package fbhttp

import (
 "context"
 "errors"
 "log"
 "net/http"
 "os"
 "sync"
 "time"

 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/storage"
)
var automationServices=struct{sync.Mutex;items map[*drivebackup.Service]*drivebackup.Automation}{items:map[*drivebackup.Service]*drivebackup.Automation{}}
func backupAutomation(service *drivebackup.Service)(*drivebackup.Automation,error){
 automationServices.Lock();defer automationServices.Unlock()
 if existing:=automationServices.items[service];existing!=nil{return existing,nil}
 automation,err:=drivebackup.NewAutomation(service);if err!=nil{return nil,err};automationServices.items[service]=automation;return automation,nil
}
var backupCatalogHandler=withAdmin(func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store");service,err:=backupService();if err!=nil{return http.StatusServiceUnavailable,err}
 scope,err:=restoreScope(d);if err!=nil{return http.StatusForbidden,nil}
 cursor:=r.URL.Query().Get("cursor");if len(cursor)>128{return http.StatusBadRequest,nil}
 page,err:=service.Catalog(r.Context(),d.user.ID,scope,cursor,50);if err!=nil{return backupError(err)};return renderJSON(w,r,page)
})
var backupSchedulesHandler=withAdmin(func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store");service,err:=backupService();if err!=nil{return http.StatusServiceUnavailable,err};automation,err:=backupAutomation(service);if err!=nil{return http.StatusServiceUnavailable,err}
 scope,err:=restoreScope(d);if err!=nil{return http.StatusForbidden,nil}
 if r.Method==http.MethodGet{return renderJSON(w,r,automation.List(d.user.ID,scope))}
 var input drivebackup.ScheduleInput;if err:=backupBody(w,r,&input);err!=nil{return http.StatusBadRequest,err}
 if input.Enabled{
  cfg,err:=service.Configuration(input.DestinationID);if err!=nil{return backupError(err)}
  if cfg.SourceOwner!=d.user.ID||cfg.SourceBase!=scope{return http.StatusForbidden,nil}
  if _,err:=makeDriveBackupPlan(r.Context(),d,cfg);err!=nil{return backupError(err)}
 }
 item,err:=automation.Save(input,backupActor(r,d),scope,time.Now().UTC());if err!=nil{if errors.Is(err,drivebackup.ErrScheduleConflict){return http.StatusConflict,err};return backupError(err)};return renderJSON(w,r,item)
})
// Called by the actual server lifecycle, never lazily by opening the settings
// page. Tests can construct handlers without starting scheduler goroutines.
func StartBackupAutomation(store *storage.Storage,server *settings.Server)(func(),error){
 service,err:=backupService();if err!=nil{return nil,err};automation,err:=backupAutomation(service);if err!=nil{return nil,err}
 build:=func(ctx context.Context,cfg drivebackup.Config,item drivebackup.Schedule)(drivebackup.DataBackupPlan,drivebackup.Actor,error){
  user,err:=store.Users.Get(server.Root,server.FollowExternalSymlinks,item.OwnerID);if err!=nil||!user.Perm.Admin{return drivebackup.DataBackupPlan{},drivebackup.Actor{},os.ErrPermission}
  settings,err:=store.Settings.Get();if err!=nil{return drivebackup.DataBackupPlan{},drivebackup.Actor{},err}
  d:=&data{store:store,server:server,settings:settings,user:user}
  scope,err:=restoreScope(d);if err!=nil||scope!=item.SourceBase{return drivebackup.DataBackupPlan{},drivebackup.Actor{},os.ErrPermission}
  plan,err:=makeDriveBackupPlan(ctx,d,cfg);if err!=nil{return drivebackup.DataBackupPlan{},drivebackup.Actor{},err}
  return *plan,drivebackup.Actor{UserID:user.ID,Username:user.Username},nil
 }
 ctx,cancel:=context.WithCancel(context.Background());done:=make(chan struct{})
 go func(){defer close(done);automation.Run(ctx,build,func(error){log.Print("Drive backup scheduler: operation failed; inspect schedule/job state.")})}()
 var once sync.Once
 stop:=func(){once.Do(func(){cancel();<-done;shutdown,release:=context.WithTimeout(context.Background(),20*time.Second);defer release();if err:=service.Shutdown(shutdown);err!=nil{log.Print("Drive backup worker shutdown did not finish before its deadline.")}})}
 return stop,nil
}
