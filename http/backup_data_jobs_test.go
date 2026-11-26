package fbhttp

import("context";"os";"path/filepath";"testing"
 "github.com/filebrowser/filebrowser/v2/internal/drivebackup"
 "github.com/filebrowser/filebrowser/v2/settings"
 "github.com/filebrowser/filebrowser/v2/users")
func TestDriveBackupPlanScopeAndExport(t *testing.T){
 root:=t.TempDir();scope:=filepath.Join(root,"user");if err:=os.MkdirAll(filepath.Join(scope,"docs"),0755);err!=nil{t.Fatal(err)}
 t.Setenv("DRIVE_BACKUP_STATE_DIR",filepath.Join(t.TempDir(),"private-state"))
 key:=[]byte("test-signing-key");st:=scopedUserStorage(t,scope,users.Permissions{Admin:true},key)
 all,err:=st.Users.Gets(root,false);if err!=nil||len(all)!=1{t.Fatal("user fixture failed")};set,err:=st.Settings.Get();if err!=nil{t.Fatal(err)}
 current,err:=st.Users.Get(root,false,all[0].ID)
 if err!=nil{t.Fatal(err)}
 d:=&data{store:st,server:&settings.Server{Root:root},settings:set,user:current}
 if err:=d.user.Fs.MkdirAll("/docs",0755);err!=nil{t.Fatal(err)}
 base,err:=filepath.EvalSymlinks(d.user.FullPath("/"));if err!=nil{t.Fatal(err)}
 config:=drivebackup.Config{SourcePaths:[]string{"/docs"},SourceOwner:d.user.ID,SourceBase:base}
 plan,err:=makeDriveBackupPlan(context.Background(),d,config);if err!=nil{t.Fatal(err)}
 if len(plan.Sources)!=1||plan.ExportDatabase==nil{t.Fatal("incomplete backup plan")};if err:=plan.Revalidate(context.Background());err!=nil{t.Fatal(err)}
 config.SourceOwner++;if _,err:=makeDriveBackupPlan(context.Background(),d,config);err==nil{t.Fatal("different source owner accepted")}
 config.SourceOwner=d.user.ID;config.SourceBase=filepath.Join(base,"changed");if _,err:=makeDriveBackupPlan(context.Background(),d,config);err==nil{t.Fatal("changed scope accepted")}
 config.SourceBase=base;config.SourcePaths=[]string{"/"};if _,err:=makeDriveBackupPlan(context.Background(),d,config);err==nil{t.Fatal("whole Drive root accepted")}
}
