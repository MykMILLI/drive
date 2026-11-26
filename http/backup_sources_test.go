package fbhttp

import("encoding/json";"net/http/httptest";"os";"path/filepath";"testing"
 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/users")
func TestBackupFolderBrowserScope(t *testing.T){
 root:=t.TempDir();scope:=filepath.Join(root,"user");if err:=os.MkdirAll(filepath.Join(scope,"docs","nested"),0755);err!=nil{t.Fatal(err)}
 outside:=t.TempDir();if err:=os.Symlink(outside,filepath.Join(scope,"escape"));err!=nil{t.Fatal(err)}
 key:=[]byte("test-signing-key");perm:=users.Permissions{Admin:true};st:=scopedUserStorage(t,scope,perm,key);srv:=&settings.Server{Root:root};token:=signToken(t,perm,key)
 do:=func(target string)*httptest.ResponseRecorder{req:=httptest.NewRequest("GET",target,nil);req.Header.Set("X-Auth",token);rec:=httptest.NewRecorder();handle(backupSourceFoldersHandler,"",st,srv).ServeHTTP(rec,req);return rec}
 rec:=do("/api/admin/backups/source-folders?path=/");if rec.Code!=200{t.Fatalf("root browse: %d %s",rec.Code,rec.Body.String())}
 var result struct{Path string `json:"path"`;Folders []backupFolder `json:"folders"`;CanSelect bool `json:"canSelect"`};if err:=json.Unmarshal(rec.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
 if result.CanSelect{t.Fatal("whole Drive root selectable")};if len(result.Folders)!=1||result.Folders[0].Path!="/docs"{t.Fatalf("unexpected folders: %s",rec.Body.String())}
 if rec:=do("/api/admin/backups/source-folders?path=/docs");rec.Code!=200{t.Fatalf("nested browse: %d",rec.Code)}
 if rec:=do("/api/admin/backups/source-folders?path=/../outside");rec.Code==200{t.Fatal("traversal allowed")}
 if rec:=do("/api/admin/backups/source-folders?path=/escape");rec.Code==200{t.Fatal("symlink root allowed")}
}
