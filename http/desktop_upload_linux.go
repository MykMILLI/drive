//go:build linux

package fbhttp

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "strings"

 "golang.org/x/sys/unix"
)

// Takes ownership of fd and returns the final directory descriptor. Every
// intermediate component is bound with O_NOFOLLOW; no symlink is traversed.
func driveWalkFD(fd int,parts []string,create bool)(int,error){
 for _,part:=range parts{
  if part==""{continue}
  if part=="."||part==".."{unix.Close(fd);return -1,os.ErrPermission}
  next,err:=unix.Openat(fd,part,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,0)
  if errors.Is(err,unix.ENOENT)&&create{
   if makeErr:=unix.Mkdirat(fd,part,0700);makeErr!=nil&&!errors.Is(makeErr,unix.EEXIST){unix.Close(fd);return -1,makeErr}
   next,err=unix.Openat(fd,part,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,0)
  }
  if err!=nil{unix.Close(fd);if errors.Is(err,unix.ELOOP){return -1,os.ErrPermission};if errors.Is(err,unix.ENOTDIR){return -1,os.ErrExist};return -1,err}
  unix.Close(fd);fd=next
 }
 return fd,nil
}
func driveDeviceRootFD(scope,root string)(int,error){
 fd,err:=unix.Open(scope,unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,0)
 if err!=nil{return -1,err}
 return driveWalkFD(fd,strings.Split(strings.TrimPrefix(root,"/"),"/"),false)
}
func driveCreateNewFile(scope,root,name string,body io.Reader,length int64,digest string,mode uint32,authorize func()error)error{
 fd,err:=driveDeviceRootFD(scope,root);if err!=nil{return err}
 parts:=strings.Split(name,"/")
 fd,err=driveWalkFD(fd,parts[:len(parts)-1],true);if err!=nil{return err}
 defer unix.Close(fd)
 var random [16]byte;if _,err:=rand.Read(random[:]);err!=nil{return err}
 temp:=".drive-sync-upload-"+hex.EncodeToString(random[:])
 fileFD,err:=unix.Openat(fd,temp,unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,0600);if err!=nil{return err}
 defer unix.Unlinkat(fd,temp,0)
 file:=os.NewFile(uintptr(fileFD),temp);defer file.Close()
 sum:=sha256.New();copied,err:=io.Copy(io.MultiWriter(file,sum),io.LimitReader(body,driveSyncUploadLimit+1))
 if err!=nil{return err}
 if copied!=length||copied>driveSyncUploadLimit||!driveSyncSame(hex.EncodeToString(sum.Sum(nil)),digest){return errDriveUploadContent}
 if mode==0{mode=0600};if err:=file.Chmod(os.FileMode(mode));err!=nil{return err}
 if err:=file.Sync();err!=nil{return err};if err:=authorize();err!=nil{return err}
 if err:=unix.Linkat(fd,temp,fd,parts[len(parts)-1],0);err!=nil{return err}
 _=unix.Fsync(fd);return nil
}
func driveCreateDirectory(scope,root,name string)error{
 fd,err:=driveDeviceRootFD(scope,root);if err!=nil{return err}
 fd,err=driveWalkFD(fd,strings.Split(name,"/"),true);if err!=nil{return err}
 defer unix.Close(fd);_=unix.Fsync(fd);return nil
}
func driveReadDeviceFile(scope,root,name string)([]byte,error){
 fd,err:=driveDeviceRootFD(scope,root);if err!=nil{return nil,err}
 parts:=strings.Split(name,"/");fd,err=driveWalkFD(fd,parts[:len(parts)-1],false);if err!=nil{return nil,err}
 defer unix.Close(fd)
 fileFD,err:=unix.Openat(fd,parts[len(parts)-1],unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,0)
 if err!=nil{if errors.Is(err,unix.ELOOP){return nil,os.ErrPermission};return nil,err}
 file:=os.NewFile(uintptr(fileFD),name);defer file.Close()
 info,err:=file.Stat();if err!=nil{return nil,err};if !info.Mode().IsRegular(){return nil,os.ErrPermission}
 if info.Size()>driveSyncUploadLimit{return nil,errDriveUploadContent}
 content,err:=io.ReadAll(io.LimitReader(file,driveSyncUploadLimit+1));if err!=nil{return nil,err}
 if int64(len(content))>driveSyncUploadLimit{return nil,errDriveUploadContent}
 return content,nil
}
