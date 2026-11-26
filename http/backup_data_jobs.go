package fbhttp

import (
 "context"
 "errors"
 "net/http"
 "os"
 "path/filepath"
 "time"

 "github.com/spf13/afero"
 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
)

func backupVerifyDescendantRules(ctx context.Context,d *data,virtual string)error{
 if len(d.settings.Rules)==0&&len(d.user.Rules)==0{return nil}
 visited:=0;deadline:=time.Now().Add(5*time.Second)
 return afero.Walk(d.user.Fs,virtual,func(name string,info os.FileInfo,err error)error{
  if ctxErr:=ctx.Err();ctxErr!=nil{return ctxErr};if err!=nil{return err}
  visited++;if visited>100000||time.Now().After(deadline){return errors.New("source-rule validation budget exceeded; select smaller folders")}
  if !d.CheckRules(filepath.ToSlash(name)){return os.ErrPermission}
  return nil
 })
}
func makeDriveBackupPlan(ctx context.Context,d *data,c drivebackup.Config)(*drivebackup.DataBackupPlan,error){
 if len(c.SourcePaths)==0||c.SourceOwner!=d.user.ID||d.store.ExportDatabase==nil{return nil,os.ErrPermission}
 paths,base,err:=validateBackupSourcesForSave(d,c.SourcePaths);if err!=nil{return nil,err}
 if base!=c.SourceBase{return nil,os.ErrPermission}
 sources:=make([]string,0,len(paths))
 for _,virtual:=range paths{
  _,real,err:=resolveBackupFolder(d,virtual,true);if err!=nil{return nil,err}
  if err:=backupVerifyDescendantRules(ctx,d,virtual);err!=nil{return nil,err}
  sources=append(sources,real)
 }
 actorID:=d.user.ID;store:=d.store;server:=d.server;sourceBase:=base
 plan:=&drivebackup.DataBackupPlan{Sources:sources,DisplaySources:append([]string(nil),paths...),ExportDatabase:store.ExportDatabase}
 plan.Revalidate=func(ctx context.Context)error{
  if err:=ctx.Err();err!=nil{return err}
  user,err:=store.Users.Get(server.Root,server.FollowExternalSymlinks,actorID);if err!=nil||!user.Perm.Admin{return os.ErrPermission}
  settings,err:=store.Settings.Get();if err!=nil{return err}
  fresh:=&data{store:store,server:server,settings:settings,user:user}
  latest,latestBase,err:=validateBackupSourcesForSave(fresh,paths);if err!=nil{return err}
  if latestBase!=sourceBase||len(latest)!=len(sources){return os.ErrPermission}
  for i,virtual:=range latest{
   _,real,err:=resolveBackupFolder(fresh,virtual,true);if err!=nil{return err};if real!=sources[i]{return os.ErrPermission}
   if err:=backupVerifyDescendantRules(ctx,fresh,virtual);err!=nil{return err}
  }
  return nil
 }
 return plan,nil
}
func startDriveDataBackup(service *drivebackup.Service,w http.ResponseWriter,r *http.Request,d *data,input drivebackup.StartRequest)(drivebackup.Job,error){
 c,err:=service.Configuration(input.DestinationID);if err!=nil{return drivebackup.Job{},err}
 plan,err:=makeDriveBackupPlan(r.Context(),d,c);if err!=nil{return drivebackup.Job{},err}
 return service.StartBackup(input,backupActor(r,d),*plan)
}
