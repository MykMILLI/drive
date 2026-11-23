package fbhttp

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "io"
 "net/http"
 "os"
 "strings"
 "time"

 "github.com/filebrowser/filebrowser/v2/storage"
)

const driveSyncUploadLimit int64 = 32 << 20

func driveUploadDevice(r *http.Request, d *data) (storage.SyncDevice, int, error) {
 var empty storage.SyncDevice
 if d.store.SyncDevices == nil { return empty, http.StatusServiceUnavailable, nil }
 header := r.Header.Get("Authorization")
 if len(header) > 256 || !strings.HasPrefix(header,"Bearer ") { return empty, http.StatusUnauthorized,nil }
 token := strings.TrimPrefix(header,"Bearer ")
 parts := strings.Split(token,".")
 if len(parts)!=3 || parts[0]!="ds1" || len(parts[1])!=32 || len(parts[2])!=64 { return empty,http.StatusUnauthorized,nil }
 if _,err:=hex.DecodeString(parts[1]+parts[2]);err!=nil{return empty,http.StatusUnauthorized,nil}
 device,err:=d.store.SyncDevices.Get(parts[1])
 if err!=nil { if errors.Is(err,storage.ErrSyncDeviceMissing){return empty,http.StatusUnauthorized,nil};return empty,http.StatusInternalServerError,err }
 if !time.Now().Before(device.ExpiresAt)||!driveSyncSame(device.Digest,driveSyncDigest(token)){return empty,http.StatusUnauthorized,nil}
 d.user,err=d.store.Users.Get(d.server.Root,d.server.FollowExternalSymlinks,device.UserID)
 if err!=nil{return empty,http.StatusUnauthorized,nil}
 if !driveSyncSame(device.Binding,driveSyncBinding(d))||!d.user.Perm.Download{return empty,http.StatusForbidden,nil}
 return device,0,nil
}

var driveSyncCapabilitiesHandler = func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 device,status,err:=driveUploadDevice(r,d);if status!=0{return status,err}
 return renderJSON(w,r,map[string]any{"newFileUpload":device.UploadGranted&&d.user.Perm.Create,"maxFileBytes":driveSyncUploadLimit,"recursive":device.TreeGranted,"overwrite":false,"delete":false,"download":device.ReceiveGranted})
}

func driveUploadName(name string) bool {
 if name=="" || len(name)>4096 || strings.HasPrefix(name,"/") || strings.ContainsAny(name,"\\\x00") { return false }
 parts:=strings.Split(name,"/")
 if len(parts)>64 { return false }
 for _,part:=range parts {
  if part=="" || part=="." || part==".." || len(part)>240 || strings.HasPrefix(part,".drive-sync-") || strings.IndexFunc(part,func(c rune)bool{return c<32||c==127})>=0 { return false }
 }
 return true
}

var driveSyncUploadHandler = func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 device,status,err:=driveUploadDevice(r,d);if status!=0{return status,err}
 if !device.UploadGranted||!d.user.Perm.Create{return http.StatusForbidden,nil}
 if r.Header.Get("If-None-Match")!="*"{return http.StatusPreconditionRequired,nil}
 name:=r.URL.Query().Get("name")
 if !driveUploadName(name){return http.StatusBadRequest,errors.New("select a top-level file name without path separators")}
 if strings.Contains(name,"/") && !device.TreeGranted { return http.StatusForbidden,nil }
 root,err:=driveSyncRoot(device.Root);if err!=nil{return http.StatusForbidden,nil}
 current:=""
 for _,part:=range strings.Split(strings.TrimPrefix(root,"/"),"/"){
  current+="/"+part
  if !d.Check(current){return http.StatusForbidden,nil}
  info,err:=driveSyncLstat(d.user.Fs,current)
  if err!=nil{return errToStatus(err),err}
  if !info.IsDir(){return http.StatusForbidden,nil}
 }
 virtual:=root+"/"+name
 if !driveTreePathAllowed(d,root,name){return http.StatusForbidden,nil}
 if r.ContentLength<0{return http.StatusLengthRequired,nil}
 if r.ContentLength>driveSyncUploadLimit{return http.StatusRequestEntityTooLarge,nil}
 expected:=strings.ToLower(r.Header.Get("X-Content-SHA256"))
 hash,err:=hex.DecodeString(expected)
 if err!=nil||len(hash)!=sha256.Size{return http.StatusBadRequest,errors.New("a SHA256 content digest is required")}
 if r.Body==nil{return http.StatusBadRequest,nil}
 r.Body=http.MaxBytesReader(w,r.Body,driveSyncUploadLimit)
 // Revalidate device/account immediately before publishing the completed file.
 authorize:=func()error{
  fresh:=*d
  latest,status,err:=driveUploadDevice(r,&fresh)
  if err!=nil{return err}
  if status!=0||!latest.UploadGranted||!fresh.user.Perm.Create||latest.Root!=root||!driveTreePathAllowed(&fresh,root,name)||(strings.Contains(name,"/")&&!latest.TreeGranted){return os.ErrPermission}
  return nil
 }
 err=d.RunHook(func()error{
  return driveCreateNewFile(d.user.FullPath("/"),root,name,r.Body,r.ContentLength,expected,uint32(d.settings.FileMode.Perm()&0666),authorize)
 },"upload",virtual,"",d.user)
 if err!=nil{
  var tooLarge *http.MaxBytesError
  switch{
  case errors.Is(err,os.ErrExist):return http.StatusPreconditionFailed,nil
  case errors.Is(err,os.ErrPermission):return http.StatusForbidden,nil
  case errors.Is(err,errDriveUploadContent):return http.StatusBadRequest,err
  case errors.As(err,&tooLarge):return http.StatusRequestEntityTooLarge,nil
  case errors.Is(err,errDriveUploadUnsupported):return http.StatusNotImplemented,nil
  default:return errToStatus(err),err
  }
 }
 w.Header().Set("Content-Type","application/json")
 w.WriteHeader(http.StatusCreated)
 _,err=io.WriteString(w,`{"created":true}`)
 return 0,err
}

var errDriveUploadContent=errors.New("uploaded content length or SHA256 digest did not match")
var errDriveUploadUnsupported=errors.New("safe create-only uploads are not available on this server platform")
