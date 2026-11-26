package fbhttp

import("encoding/json";"net/http/httptest";"os";"path/filepath";"strings";"testing"
 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/users")
func TestBackupAdminAuthorization(t *testing.T){
 for _,admin:=range []bool{false,true}{t.Run(map[bool]string{false:"ordinary-user",true:"administrator"}[admin],func(t *testing.T){
  root:=t.TempDir();scope:=filepath.Join(root,"user");if err:=os.MkdirAll(scope,0755);err!=nil{t.Fatal(err)}
  t.Setenv("DRIVE_BACKUP_STATE_DIR",filepath.Join(t.TempDir(),"state"))
  key:=[]byte("test-signing-key");perm:=users.Permissions{Admin:admin};st:=scopedUserStorage(t,scope,perm,key);srv:=&settings.Server{Root:root};token:=signToken(t,perm,key)
  request:=httptest.NewRequest("GET","/api/admin/backups/destinations",nil);request.Header.Set("X-Auth",token);rec:=httptest.NewRecorder();handle(backupDestinationsHandler,"",st,srv).ServeHTTP(rec,request)
  if !admin{if rec.Code!=403{t.Fatalf("nonadmin access: %d",rec.Code)};return};if rec.Code!=200{t.Fatalf("admin access: %d %s",rec.Code,rec.Body.String())}
  body:=`{"config":{"id":"","name":"Not configured","enabled":false,"backend":"s3","endpoint":"","region":"","bucket":"","prefix":"","addressing":"auto","connections":2,"uploadKiB":0,"downloadKiB":0,"allowHTTP":false,"caBundle":""},"secrets":{"repositoryPassword":"do-not-echo-this-password"}}`
  request=httptest.NewRequest("POST","/api/admin/backups/destinations",strings.NewReader(body));request.Header.Set("X-Auth",token);rec=httptest.NewRecorder();handle(backupDestinationsHandler,"",st,srv).ServeHTTP(rec,request)
  if rec.Code!=200{t.Fatalf("save disabled destination: %d %s",rec.Code,rec.Body.String())};if strings.Contains(rec.Body.String(),"do-not-echo"){t.Fatal("credential exposed")}
  var saved map[string]any;if err:=json.Unmarshal(rec.Body.Bytes(),&saved);err!=nil{t.Fatal(err)}
 })}
}
