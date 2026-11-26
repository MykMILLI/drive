package fbhttp

import (
 "context"
 "errors"
 "net/http"
 "os"
 "path/filepath"

 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
)
func restoreScope(d *data)(string,error){return filepath.EvalSymlinks(d.user.FullPath("/"))}
func makeRestoreApproval(d *data,source drivebackup.Job)(drivebackup.RestoreApproval,error){
 if source.Actor.UserID!=d.user.ID||source.SourceBase==""{return drivebackup.RestoreApproval{},os.ErrPermission}
 actorID:=d.user.ID;store:=d.store;server:=d.server;expected:=source.SourceBase
 approval:=drivebackup.RestoreApproval{Revalidate:func(ctx context.Context)error{
  if err:=ctx.Err();err!=nil{return err}
  user,err:=store.Users.Get(server.Root,server.FollowExternalSymlinks,actorID);if err!=nil||!user.Perm.Admin{return os.ErrPermission}
  settings,err:=store.Settings.Get();if err!=nil{return err}
  // This first recovery release fails closed for any file rules rather than
  // silently restoring content those rules might now prohibit.
  if len(settings.Rules)>0||len(user.Rules)>0{return errors.New("staging restore currently requires an administrator without global or account file rules")}
  base,err:=filepath.EvalSymlinks(user.FullPath("/"));if err!=nil{return err};if base!=expected{return os.ErrPermission};return nil
 }}
 if err:=approval.Revalidate(context.Background());err!=nil{return drivebackup.RestoreApproval{},err};return approval,nil
}
func startDriveStagingRestore(service *drivebackup.Service,r *http.Request,d *data,input drivebackup.StartRequest)(drivebackup.Job,error){
 if !input.ConfirmRestore{return drivebackup.Job{},drivebackup.ErrRestoreApproval}
 source,err:=service.BackupForRestore(input.RestoreFromJobID,backupActor(r,d));if err!=nil{return drivebackup.Job{},err}
 approval,err:=makeRestoreApproval(d,source);if err!=nil{return drivebackup.Job{},err}
 return service.StartRestore(input,backupActor(r,d),approval)
}
var backupRestoresHandler=withAdmin(func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 service,err:=backupService();if err!=nil{return http.StatusServiceUnavailable,err}
 base,err:=restoreScope(d);if err!=nil{return http.StatusForbidden,nil}
 if r.Method==http.MethodGet{
  visible:=make([]drivebackup.RestoreArtifact,0)
  for _,artifact:=range service.RestoreArtifacts(){if artifact.OwnerID==d.user.ID&&artifact.SourceBase==base{visible=append(visible,artifact)}}
  return renderJSON(w,r,visible)
 }
 var input struct{ID string `json:"id"`;ConfirmCleanup bool `json:"confirmCleanup"`}
 if err:=backupBody(w,r,&input);err!=nil{return http.StatusBadRequest,err}
 if !input.ConfirmCleanup{return http.StatusBadRequest,errors.New("staged recovery cleanup requires explicit confirmation")}
 permitted:=false
 for _,artifact:=range service.RestoreArtifacts(){if artifact.ID==input.ID&&artifact.OwnerID==d.user.ID&&artifact.SourceBase==base{permitted=true;break}}
 if !permitted{return http.StatusNotFound,nil}
 if err:=service.CleanupRestore(input.ID,backupActor(r,d));err!=nil{return backupError(err)}
 return renderJSON(w,r,map[string]bool{"cleaned":true})
})
