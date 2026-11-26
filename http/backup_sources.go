package fbhttp

import (
 "errors"
 "io"
 "net/http"
 "os"
 "path"
 "path/filepath"
 "sort"
 "strings"
 "unicode/utf8"
)

type backupFolder struct{Name string `json:"name"`;Path string `json:"path"`}
func backupPathInside(child,parent string)bool{relative,err:=filepath.Rel(parent,child);return err==nil&&relative!=".."&&!strings.HasPrefix(relative,".."+string(os.PathSeparator))&&!filepath.IsAbs(relative)}
func backupRealOrClean(value string)string{if resolved,err:=filepath.EvalSymlinks(value);err==nil{return resolved};return filepath.Clean(value)}
func backupPrivateRoots()[]string{
 state:=os.Getenv("DRIVE_BACKUP_STATE_DIR");if state==""{state="/config/drive-backups"}
 return []string{backupRealOrClean("/config"),backupRealOrClean("/database"),backupRealOrClean("/srv/app-data"),backupRealOrClean(state)}
}
func backupVirtualPath(raw string)(string,error){
 if raw==""{raw="/"}
 if !utf8.ValidString(raw)||len(raw)>1024||!strings.HasPrefix(raw,"/")||strings.ContainsAny(raw,"\\\x00")||strings.IndexFunc(raw,func(c rune)bool{return c<32||c==127})>=0{return "",errors.New("invalid Drive folder path")}
 for _,part:=range strings.Split(raw,"/"){if part=="."||part==".."{return "",errors.New("folder traversal is not allowed")}}
 return path.Clean(raw),nil
}
func resolveBackupFolder(d *data,raw string,selection bool)(string,string,error){
 virtual,err:=backupVirtualPath(raw);if err!=nil{return "","",err}
 if selection&&virtual=="/"{return "","",errors.New("select specific folders, not the whole Drive root")}
 if !d.CheckRules("/"){return "","",os.ErrPermission}
 if virtual!="/"{current:="";for _,part:=range strings.Split(strings.TrimPrefix(virtual,"/"),"/"){
  current+="/"+part;if !d.CheckRules(current){return "","",os.ErrPermission};info,err:=driveSyncLstat(d.user.Fs,current);if err!=nil{return "","",err};if !info.IsDir(){return "","",errors.New("backup source must be a directory")}
 }}
 base,err:=filepath.EvalSymlinks(d.user.FullPath("/"));if err!=nil{return "","",err}
 real,err:=filepath.EvalSymlinks(d.user.FullPath(virtual));if err!=nil{return "","",err}
 if !backupPathInside(real,base){return "","",os.ErrPermission}
 for _,private:=range backupPrivateRoots(){
  if backupPathInside(real,private)||(selection&&backupPathInside(private,real)){return "","",errors.New("folder contains private configuration, a live database, or unrelated application data")}
 }
 return virtual,real,nil
}
func validateBackupSourcesForSave(d *data,paths []string)([]string,string,error){
 if len(paths)>16{return nil,"",errors.New("select at most 16 folders")}
 base,err:=filepath.EvalSymlinks(d.user.FullPath("/"));if err!=nil{return nil,"",err}
 out:=make([]string,0,len(paths))
 for _,raw:=range paths{virtual,_,err:=resolveBackupFolder(d,raw,true);if err!=nil{return nil,"",err};out=append(out,virtual)}
 return out,base,nil
}

var backupSourceFoldersHandler=withAdmin(func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 virtual,_,err:=resolveBackupFolder(d,r.URL.Query().Get("path"),false)
 if err!=nil{return http.StatusForbidden,err}
 directory,err:=d.user.Fs.Open(virtual);if err!=nil{return errToStatus(err),err};defer directory.Close()
 folders:=make([]backupFolder,0);visited:=0;limited:=false
 for{
  if err:=r.Context().Err();err!=nil{return http.StatusServiceUnavailable,nil}
  batch,readErr:=directory.Readdir(128)
  for _,info:=range batch{
   visited++;if visited>4096||len(folders)>=200{limited=true;break}
   if !info.IsDir()||info.Mode()&os.ModeSymlink!=0{continue}
   child:=path.Join(virtual,info.Name());clean,_,err:=resolveBackupFolder(d,child,true);if err!=nil{continue}
   folders=append(folders,backupFolder{info.Name(),clean})
  }
  if limited{break};if readErr!=nil{if readErr!=io.EOF{return errToStatus(readErr),readErr};break}
 }
 sort.Slice(folders,func(i,j int)bool{return strings.ToLower(folders[i].Name)<strings.ToLower(folders[j].Name)})
 _,_,selectErr:=resolveBackupFolder(d,virtual,true)
 return renderJSON(w,r,map[string]any{"path":virtual,"folders":folders,"canSelect":selectErr==nil,"limited":limited})
})
