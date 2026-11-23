package fbhttp

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "net/http"
 "os"
 "strings"
)

func driveTreePathAllowed(d *data,root,name string) bool {
 clean,err:=driveSyncRoot(root);if err!=nil||clean!=root||!driveUploadName(name){return false}
 current:=""
 for _,part:=range strings.Split(strings.TrimPrefix(root,"/")+"/"+name,"/"){
  current+="/"+part
  if !d.Check(current){return false}
 }
 return true
}

func driveTreeStatus(err error)(int,error){
 switch{
 case errors.Is(err,os.ErrPermission):return http.StatusForbidden,nil
 case errors.Is(err,os.ErrExist):return http.StatusConflict,nil
 case errors.Is(err,errDriveUploadUnsupported):return http.StatusNotImplemented,nil
 default:return errToStatus(err),err
 }
}

var driveSyncMkdirHandler=func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 device,status,err:=driveUploadDevice(r,d);if status!=0{return status,err}
 if !device.TreeGranted||!device.UploadGranted||!d.user.Perm.Create{return http.StatusForbidden,nil}
 name:=r.URL.Query().Get("name")
 if !driveUploadName(name){return http.StatusBadRequest,nil}
 if !driveTreePathAllowed(d,device.Root,name){return http.StatusForbidden,nil}
 err=d.RunHook(func()error{return driveCreateDirectory(d.user.FullPath("/"),device.Root,name)},"upload",device.Root+"/"+name+"/","",d.user)
 if err!=nil{return driveTreeStatus(err)}
 return renderJSON(w,r,map[string]any{"directory":true})
}

var driveSyncDownloadHandler=func(w http.ResponseWriter,r *http.Request,d *data)(int,error){
 w.Header().Set("Cache-Control","no-store")
 device,status,err:=driveUploadDevice(r,d);if status!=0{return status,err}
 if !device.ReceiveGranted{return http.StatusForbidden,nil}
 name:=r.URL.Query().Get("name")
 if !driveUploadName(name){return http.StatusBadRequest,nil}
 if !driveTreePathAllowed(d,device.Root,name){return http.StatusForbidden,nil}
 content,err:=driveReadDeviceFile(d.user.FullPath("/"),device.Root,name)
 if errors.Is(err,errDriveUploadContent){return http.StatusRequestEntityTooLarge,nil}
 if err!=nil{return driveTreeStatus(err)}
 sum:=sha256.Sum256(content)
 w.Header().Set("Content-Type","application/octet-stream")
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("X-Content-SHA256",hex.EncodeToString(sum[:]))
 _,err=w.Write(content)
 return 0,err
}
