package fbhttp
import("context";"encoding/json";"net/http/httptest";"os";"path/filepath";"strings";"testing"
 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
 "github.com/filebrowser/filebrowser/v2/rules"
 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/users")
func TestBackupRestoresAuthorizationAndConfirmation(t *testing.T){
 for _,admin:=range []bool{false,true}{t.Run(map[bool]string{false:"ordinary",true:"admin"}[admin],func(t *testing.T){
 root:=t.TempDir();scope:=filepath.Join(root,"user");if err:=os.MkdirAll(scope,0755);err!=nil{t.Fatal(err)}
 t.Setenv("DRIVE_BACKUP_STATE_DIR",filepath.Join(t.TempDir(),"state"))
 key:=[]byte("test-signing-key");perm:=users.Permissions{Admin:admin};st:=scopedUserStorage(t,scope,perm,key);server:=&settings.Server{Root:root};token:=signToken(t,perm,key)
 request:=httptest.NewRequest("GET","/api/admin/backups/restores",nil);request.Header.Set("X-Auth",token);rec:=httptest.NewRecorder();handle(backupRestoresHandler,"",st,server).ServeHTTP(rec,request)
 if !admin{if rec.Code!=403{t.Fatalf("ordinary access: %d",rec.Code)};return}
 if rec.Code!=200{t.Fatalf("admin access: %d %s",rec.Code,rec.Body.String())};var records []drivebackup.RestoreArtifact;if err:=json.Unmarshal(rec.Body.Bytes(),&records);err!=nil||len(records)!=0{t.Fatalf("unexpected artifacts: %s",rec.Body.String())}
 request=httptest.NewRequest("POST","/api/admin/backups/restores",strings.NewReader(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","confirmCleanup":false}`));request.Header.Set("X-Auth",token);rec=httptest.NewRecorder();handle(backupRestoresHandler,"",st,server).ServeHTTP(rec,request);if rec.Code!=400{t.Fatalf("unconfirmed cleanup: %d",rec.Code)}
 request=httptest.NewRequest("POST","/api/admin/backups/jobs",strings.NewReader(`{"action":"restore-staging","confirmRestore":true,"restoreTarget":"/database"}`));request.Header.Set("X-Auth",token);rec=httptest.NewRecorder();handle(backupJobsHandler,"",st,server).ServeHTTP(rec,request);if rec.Code!=400{t.Fatalf("arbitrary restore target accepted: %d",rec.Code)}
 all,err:=st.Users.Gets(root,false);if err!=nil||len(all)!=1{t.Fatal("fixture users missing")};user,err:=st.Users.Get(root,false,all[0].ID);if err!=nil{t.Fatal(err)};set,err:=st.Settings.Get();if err!=nil{t.Fatal(err)}
 d:=&data{store:st,server:server,settings:set,user:user};base,err:=filepath.EvalSymlinks(user.FullPath("/"));if err!=nil{t.Fatal(err)}
 source:=drivebackup.Job{Actor:drivebackup.Actor{UserID:user.ID},SourceBase:base,Sources:[]string{"/deleted-folder"}}
 approval,err:=makeRestoreApproval(d,source);if err!=nil{t.Fatal(err)};if err:=approval.Revalidate(context.Background());err!=nil{t.Fatal(err)}
 set.Rules=[]rules.Rule{{}};if err:=st.Settings.Save(set);err!=nil{t.Fatal(err)};if err:=approval.Revalidate(context.Background());err==nil{t.Fatal("changed global rules were ignored")};set.Rules=nil;if err:=st.Settings.Save(set);err!=nil{t.Fatal(err)}
 source.SourceBase=filepath.Join(base,"wrong-scope");if _,err:=makeRestoreApproval(d,source);err==nil{t.Fatal("changed source scope accepted")}
 })}
}
