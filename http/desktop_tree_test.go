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
func TestDriveTreeTransfers(t *testing.T){
 root:=t.TempDir();scope:=filepath.Join(root,"user");if err:=os.MkdirAll(filepath.Join(scope,"docs"),0755);err!=nil{t.Fatal(err)}
 key:=[]byte("test-signing-key");perm:=users.Permissions{Download:true,Create:true};st:=scopedUserStorage(t,scope,perm,key);srv:=&settings.Server{Root:root}
 all,err:=st.Users.Gets(root,false);if err!=nil||len(all)!=1{t.Fatal("user fixture failed")};set,err:=st.Settings.Get();if err!=nil{t.Fatal(err)}
 token:="ds1."+strings.Repeat("e",32)+"."+strings.Repeat("f",64)
 record:=storage.SyncDevice{ID:strings.Repeat("e",32),UserID:all[0].ID,Root:"/docs",Name:"tree",Digest:driveSyncDigest(token),Binding:driveSyncBinding(&data{user:all[0],settings:set}),ExpiresAt:time.Now().Add(time.Hour),UploadGranted:true,TreeGranted:true,ReceiveGranted:true}
 if err:=st.SyncDevices.Create(record);err!=nil{t.Fatal(err)}
 do:=func(h handleFunc,method,name,content,credential string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(method,"/api/sync/device/test?name="+url.QueryEscape(name),strings.NewReader(content));req.Header.Set("Authorization","Bearer "+credential);req.Header.Set("If-None-Match","*");sum:=sha256.Sum256([]byte(content));req.Header.Set("X-Content-SHA256",hex.EncodeToString(sum[:]));req.ContentLength=int64(len(content));rec:=httptest.NewRecorder();handle(h,"",st,srv).ServeHTTP(rec,req);return rec
 }
 if r:=do(driveSyncMkdirHandler,"POST","empty/nested","",token);r.Code!=200{t.Fatalf("mkdir: %d %s",r.Code,r.Body.String())}
 if info,err:=os.Stat(filepath.Join(scope,"docs","empty","nested"));err!=nil||!info.IsDir(){t.Fatal("empty directory missing")}
 if r:=do(driveSyncUploadHandler,"POST","nested/files/a.txt","hello",token);r.Code!=201{t.Fatalf("nested upload: %d %s",r.Code,r.Body.String())}
 r:=do(driveSyncDownloadHandler,"GET","nested/files/a.txt","",token);if r.Code!=200||r.Body.String()!="hello"{t.Fatalf("download: %d %s",r.Code,r.Body.String())}
 sum:=sha256.Sum256([]byte("hello"));if r.Header().Get("X-Content-SHA256")!=hex.EncodeToString(sum[:]){t.Fatal("download digest missing")}
 if r:=do(driveSyncDownloadHandler,"GET","../escape.txt","",token);r.Code!=400{t.Fatalf("traversal: %d",r.Code)}
 if err:=os.WriteFile(filepath.Join(scope,"docs","browser-created.txt"),[]byte("browser"),0600);err!=nil{t.Fatal(err)}
 if r:=do(driveSyncDownloadHandler,"GET","browser-created.txt","",token);r.Code!=200||r.Body.String()!="browser"{t.Fatal("external file not readable")}
 outside:=t.TempDir();if err:=os.Symlink(outside,filepath.Join(scope,"docs","link"));err!=nil{t.Fatal(err)}
 if r:=do(driveSyncMkdirHandler,"POST","link/escape","",token);r.Code==200{t.Fatal("mkdir followed symlink")}
 if _,err:=os.Stat(filepath.Join(outside,"escape"));!os.IsNotExist(err){t.Fatal("escaped selected tree")}
 oldToken:="ds1."+strings.Repeat("1",32)+"."+strings.Repeat("2",64);old:=record;old.ID=strings.Repeat("1",32);old.Digest=driveSyncDigest(oldToken);old.TreeGranted=false;old.ReceiveGranted=false
 if err:=st.SyncDevices.Create(old);err!=nil{t.Fatal(err)}
 if r:=do(driveSyncDownloadHandler,"GET","browser-created.txt","",oldToken);r.Code!=403{t.Fatalf("old device gained read capability: %d",r.Code)}
 if r:=do(driveSyncMkdirHandler,"POST","denied","",oldToken);r.Code!=403{t.Fatalf("old device gained mkdir capability: %d",r.Code)}
 if r:=do(driveSyncUploadHandler,"POST","new/denied.txt","hello",oldToken);r.Code!=403{t.Fatalf("old device gained recursive upload: %d",r.Code)}
}
