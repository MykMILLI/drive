//go:build linux

package fbhttp

import (
 "crypto/sha256"
 "encoding/hex"
 "net/http/httptest"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/storage"
 "github.com/filebrowser/filebrowser/v2/users"
)

func TestDriveUploadCreateOnly(t *testing.T){
 root:=t.TempDir();scope:=filepath.Join(root,"user")
 if err:=os.MkdirAll(filepath.Join(scope,"docs"),0755);err!=nil{t.Fatal(err)}
 key:=[]byte("test-signing-key");perm:=users.Permissions{Download:true,Create:true}
 st:=scopedUserStorage(t,scope,perm,key);srv:=&settings.Server{Root:root}
 all,err:=st.Users.Gets(root,false);if err!=nil||len(all)!=1{t.Fatalf("users: %v",err)}
 set,err:=st.Settings.Get();if err!=nil{t.Fatal(err)}
 token:="ds1."+strings.Repeat("a",32)+"."+strings.Repeat("b",64)
 record:=storage.SyncDevice{ID:strings.Repeat("a",32),UserID:all[0].ID,Root:"/docs",Name:"upload test",Digest:driveSyncDigest(token),Binding:driveSyncBinding(&data{user:all[0],settings:set}),ExpiresAt:time.Now().Add(time.Hour),UploadGranted:true}
 if err:=st.SyncDevices.Create(record);err!=nil{t.Fatal(err)}
 do:=func(name,body,credential,digest string,length int64)*httptest.ResponseRecorder{
  req:=httptest.NewRequest("POST","/api/sync/device/upload?name="+url.QueryEscape(name),strings.NewReader(body))
  req.ContentLength=length;req.Header.Set("If-None-Match","*");req.Header.Set("X-Content-SHA256",digest)
  if credential!=""{req.Header.Set("Authorization","Bearer "+credential)}
  rec:=httptest.NewRecorder();handle(driveSyncUploadHandler,"",st,srv).ServeHTTP(rec,req);return rec
 }
 hash:=func(s string)string{sum:=sha256.Sum256([]byte(s));return hex.EncodeToString(sum[:])}
 if r:=do("hello.txt","hello","",hash("hello"),5);r.Code!=401{t.Fatalf("anonymous: %d",r.Code)}
 if r:=do("hello.txt","hello",token,hash("hello"),5);r.Code!=201{t.Fatalf("upload: %d %s",r.Code,r.Body.String())}
 if r:=do("hello.txt","other",token,hash("other"),5);r.Code!=412{t.Fatalf("overwrite: %d %s",r.Code,r.Body.String())}
 raw,err:=os.ReadFile(filepath.Join(scope,"docs","hello.txt"));if err!=nil||string(raw)!="hello"{t.Fatalf("original changed: %q %v",raw,err)}
 if r:=do("../escape.txt","hello",token,hash("hello"),5);r.Code!=400{t.Fatalf("traversal: %d",r.Code)}
 if r:=do("bad.txt","hello",token,hash("other"),5);r.Code!=400{t.Fatalf("digest: %d",r.Code)}
 if _,err:=os.Stat(filepath.Join(scope,"docs","bad.txt"));!os.IsNotExist(err){t.Fatal("bad digest published a file")}
 if r:=do("short.txt","hello",token,hash("hello"),6);r.Code!=400{t.Fatalf("short body: %d",r.Code)}
 if r:=do("huge.txt","",token,hash(""),driveSyncUploadLimit+1);r.Code!=413{t.Fatalf("size limit: %d",r.Code)}
 target:=filepath.Join(t.TempDir(),"outside.txt");if err:=os.WriteFile(target,[]byte("outside"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(target,filepath.Join(scope,"docs","link.txt"));err!=nil{t.Fatal(err)}
 if r:=do("link.txt","hello",token,hash("hello"),5);r.Code!=412{t.Fatalf("symlink target: %d",r.Code)}
 raw,_=os.ReadFile(target);if string(raw)!="outside"{t.Fatal("symlink target modified")}
 entries,_:=os.ReadDir(filepath.Join(scope,"docs"));for _,entry:=range entries{if strings.HasPrefix(entry.Name(),".drive-sync-upload-"){t.Fatal("temporary upload leaked")}}
 oldToken:="ds1."+strings.Repeat("c",32)+"."+strings.Repeat("d",64)
 old:=record;old.ID=strings.Repeat("c",32);old.Digest=driveSyncDigest(oldToken);old.UploadGranted=false
 if err:=st.SyncDevices.Create(old);err!=nil{t.Fatal(err)}
 if r:=do("not-approved.txt","hello",oldToken,hash("hello"),5);r.Code!=403{t.Fatalf("inventory-only device gained upload rights: %d",r.Code)}
 if err:=st.SyncDevices.Revoke(record.UserID,record.ID);err!=nil{t.Fatal(err)}
 if r:=do("revoked.txt","hello",token,hash("hello"),5);r.Code!=401{t.Fatalf("revocation: %d",r.Code)}
}
