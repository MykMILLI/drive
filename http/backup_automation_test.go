package fbhttp
import("encoding/json";"net/http/httptest";"os";"path/filepath";"strings";"testing"
 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/users")
func TestBackupAutomationAdminAndDisabledSave(t *testing.T){
 for _,admin:=range []bool{false,true}{t.Run(map[bool]string{false:"ordinary",true:"administrator"}[admin],func(t *testing.T){
 root:=t.TempDir();scope:=filepath.Join(root,"user");if err:=os.MkdirAll(scope,0755);err!=nil{t.Fatal(err)};t.Setenv("DRIVE_BACKUP_STATE_DIR",filepath.Join(t.TempDir(),"state"))
 key:=[]byte("test-signing-key");perm:=users.Permissions{Admin:admin};st:=scopedUserStorage(t,scope,perm,key);server:=&settings.Server{Root:root};token:=signToken(t,perm,key)
 do:=func(fn handleFunc,method,target,body string)*httptest.ResponseRecorder{request:=httptest.NewRequest(method,target,strings.NewReader(body));request.Header.Set("X-Auth",token);rec:=httptest.NewRecorder();handle(fn,"",st,server).ServeHTTP(rec,request);return rec}
 for _,handler:=range []handleFunc{backupCatalogHandler,backupSchedulesHandler}{rec:=do(handler,"GET","/api/admin/backups/catalog","");if !admin{if rec.Code!=403{t.Fatalf("ordinary access: %d",rec.Code)}}else if rec.Code!=200{t.Fatalf("administrator read: %d %s",rec.Code,rec.Body.String())}}
 if !admin{return}
 destinationBody:=`{"config":{"name":"Disabled destination","enabled":false,"backend":"s3","addressing":"auto"},"secrets":{}}`
 rec:=do(backupDestinationsHandler,"POST","/api/admin/backups/destinations",destinationBody);if rec.Code!=200{t.Fatalf("destination: %d %s",rec.Code,rec.Body.String())};var destination drivebackup.Destination;if err:=json.Unmarshal(rec.Body.Bytes(),&destination);err!=nil{t.Fatal(err)}
 input:=drivebackup.ScheduleInput{Name:"Disabled schedule",DestinationID:destination.Config.ID,TimeZone:"UTC",Hour:2,Minute:0}
 raw,_:=json.Marshal(input);rec=do(backupSchedulesHandler,"POST","/api/admin/backups/schedules",string(raw));if rec.Code!=200{t.Fatalf("disabled schedule save: %d %s",rec.Code,rec.Body.String())}
 var saved drivebackup.Schedule;if err:=json.Unmarshal(rec.Body.Bytes(),&saved);err!=nil{t.Fatal(err)};if saved.Enabled||saved.State!="idle"{t.Fatalf("unexpected schedule: %+v",saved)}
 input.Enabled=true;input.ID=saved.ID;input.Version=saved.Version;raw,_=json.Marshal(input);rec=do(backupSchedulesHandler,"POST","/api/admin/backups/schedules",string(raw));if rec.Code==200{t.Fatal("schedule enabled for disabled/unconfigured destination")}
 rec=do(backupSchedulesHandler,"POST","/api/admin/backups/schedules",`{"name":"bad","ownerId":999}`);if rec.Code!=400{t.Fatal("client-selected owner accepted")}
 })}
}
