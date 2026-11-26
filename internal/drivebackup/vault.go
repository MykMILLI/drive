package drivebackup

import (
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "encoding/json"
 "errors"
 "io"
 "os"
 "path/filepath"
 "sync"
)

// Vault is a single-process secret store. Its master key must be protected
// separately; encryption is not protection from an attacker who can read both.
type Vault struct { dir string; aead cipher.AEAD; mu sync.Mutex }
type secretWire struct { AccessKeyID string; SecretAccessKey string; SessionToken string; RepositoryPassword string }
var vaultAAD=[]byte("MILLITOP Drive backup secrets v1")

func NewVault(dir string)(*Vault,error){
 if !filepath.IsAbs(dir){return nil,errors.New("vault path must be absolute")}
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 info,err:=os.Lstat(dir);if err!=nil{return nil,err}
 if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{return nil,errors.New("vault directory must be private (0700) and not a symlink")}
 keyPath:=filepath.Join(dir,"master.key")
 key,err:=os.ReadFile(keyPath)
 if os.IsNotExist(err){
  entries,scanErr:=filepath.Glob(filepath.Join(dir,"*.secrets"));if scanErr!=nil{return nil,scanErr};if len(entries)>0{return nil,errors.New("master key is missing but encrypted secrets exist; restore the key, do not overwrite")}
  key=make([]byte,32);if _,err:=rand.Read(key);err!=nil{return nil,err}
  file,err:=os.OpenFile(keyPath,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{return nil,err}
  _,writeErr:=file.Write(key);if writeErr==nil{writeErr=file.Sync()};closeErr:=file.Close();if writeErr!=nil{return nil,writeErr};if closeErr!=nil{return nil,closeErr}
 }else if err!=nil{return nil,err}
 info,err=os.Lstat(keyPath);if err!=nil{return nil,err}
 if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||len(key)!=32{return nil,errors.New("invalid or insufficiently protected vault key")}
 block,err:=aes.NewCipher(key);for i:=range key{key[i]=0};if err!=nil{return nil,err}
 aead,err:=cipher.NewGCM(block);if err!=nil{return nil,err};return &Vault{dir:dir,aead:aead},nil
}
func (v *Vault) Save(id string,s Secrets)error{
 if !validID.MatchString(id){return errors.New("invalid destination ID")}
 v.mu.Lock();defer v.mu.Unlock()
 raw,err:=json.Marshal(secretWire{s.AccessKeyID,s.SecretAccessKey,s.SessionToken,s.RepositoryPassword});if err!=nil{return err};defer func(){for i:=range raw{raw[i]=0}}()
 nonce:=make([]byte,v.aead.NonceSize());if _,err:=rand.Read(nonce);err!=nil{return err}
 aad:=append(append([]byte{},vaultAAD...),[]byte("/"+id)...)
 encrypted:=v.aead.Seal(nonce,nonce,raw,aad)
 file,err:=os.CreateTemp(v.dir,".secret-");if err!=nil{return err};name:=file.Name();defer os.Remove(name)
 if err:=file.Chmod(0600);err!=nil{file.Close();return err}
 _,err=file.Write(encrypted);if err==nil{err=file.Sync()};closeErr:=file.Close();if err!=nil{return err};if closeErr!=nil{return closeErr}
 return os.Rename(name,filepath.Join(v.dir,id+".secrets"))
}
func (v *Vault) Load(id string)(Secrets,error){
 var empty Secrets;if !validID.MatchString(id){return empty,errors.New("invalid destination ID")}
 v.mu.Lock();defer v.mu.Unlock()
 path:=filepath.Join(v.dir,id+".secrets");info,err:=os.Lstat(path);if err!=nil{return empty,err}
 if !info.Mode().IsRegular()||info.Size()>65536{return empty,errors.New("invalid secret file")}
 raw,err:=os.ReadFile(path);if err!=nil{return empty,err};n:=v.aead.NonceSize();if len(raw)<n{return empty,errors.New("truncated secret file")}
 aad:=append(append([]byte{},vaultAAD...),[]byte("/"+id)...)
 clear,err:=v.aead.Open(nil,raw[:n],raw[n:],aad);if err!=nil{return empty,errors.New("secret authentication failed")};defer func(){for i:=range clear{clear[i]=0}}()
 var wire secretWire;if err:=json.Unmarshal(clear,&wire);err!=nil{return empty,err}
 return Secrets{wire.AccessKeyID,wire.SecretAccessKey,wire.SessionToken,wire.RepositoryPassword},nil
}
func (v *Vault) MasterKeyCopy(dst io.Writer)error{
 // Intended only for an explicitly authorized recovery-export operation.
 v.mu.Lock();defer v.mu.Unlock();file,err:=os.Open(filepath.Join(v.dir,"master.key"));if err!=nil{return err};defer file.Close();_,err=io.Copy(dst,file);return err
}
