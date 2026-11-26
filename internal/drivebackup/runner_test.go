package drivebackup

import("context";"encoding/json";"os";"os/exec";"path/filepath";"strings";"testing";"time")
func TestCommandConstruction(t *testing.T){
 d:=&Driver{workDir:t.TempDir()};c,s:=sampleConfig(),sampleSecrets();c.UploadKiB=1024;c.DownloadKiB=2048
 args,env,_,err:=d.command(c,s,Request{Action:"snapshots"});if err!=nil{t.Fatal(err)}
 joined:=strings.Join(args," ");for _,want:=range []string{"s3.bucket-lookup=path","s3.connections=2","--limit-upload 1024","--limit-download 2048"}{if !strings.Contains(joined,want){t.Error("missing",want)}}
 for _,secret:=range []string{s.AccessKeyID,s.SecretAccessKey,s.SessionToken,s.RepositoryPassword}{if strings.Contains(joined,secret){t.Fatal("secret in process arguments")}}
 if !strings.Contains(strings.Join(env,"\n"),"AWS_SESSION_TOKEN="+s.SessionToken){t.Fatal("session token missing")}
 if _,_,_,err:=d.command(c,s,Request{Action:"prune"});err==nil{t.Fatal("destructive unsupported action accepted")}
 c.Enabled=false;if _,_,_,err:=d.command(c,s,Request{Action:"initialize"});err!=ErrDisabled{t.Fatalf("disabled destination: %v",err)}
}
func TestLocalBackupRestoreIntegration(t *testing.T){
 if _,err:=exec.LookPath("restic");err!=nil{if os.Getenv("DRIVE_REQUIRE_RESTIC_TEST")=="1"{t.Fatal("restic required for this test image")};t.Skip("restic not installed")}
 base:=t.TempDir();source:=filepath.Join(base,"data");if err:=os.Mkdir(source,0700);err!=nil{t.Fatal(err)}
 content:=[]byte("MILLITOP Drive backup restore test\n");if err:=os.WriteFile(filepath.Join(source,"test.txt"),content,0600);err!=nil{t.Fatal(err)}
 d,err:=NewDriver(filepath.Join(base,"private-work"),[]string{source});if err!=nil{t.Fatal(err)}
 c:=Config{ID:"local-test",Enabled:true,Backend:"local",LocalRepository:filepath.Join(base,"repository")};s:=Secrets{RepositoryPassword:"isolated-test-password"}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute);defer cancel()
 run:=func(request Request)Result{t.Helper();result,err:=d.Run(ctx,c,s,request);if err!=nil{t.Fatalf("%s: %v\n%s\n%s",request.Action,err,result.Stdout,result.Stderr)};return result}
 // Listing must not silently initialize a repository.
 if _,err:=d.Run(ctx,c,s,Request{Action:"snapshots"});err==nil{t.Fatal("uninitialized repository unexpectedly accessible")}
 run(Request{Action:"initialize"});run(Request{Action:"backup",Sources:[]string{source}})
 listed:=run(Request{Action:"snapshots"});var snapshots []struct{ID string `json:"id"`};if err:=json.Unmarshal([]byte(listed.Stdout),&snapshots);err!=nil||len(snapshots)!=1{t.Fatalf("snapshots: %v %s",err,listed.Stdout)}
 run(Request{Action:"check",IntegrityPercent:100})
 restored:=run(Request{Action:"restore",Snapshot:snapshots[0].ID})
 found:=false;err=filepath.Walk(restored.RestoreDirectory,func(path string,info os.FileInfo,err error)error{if err!=nil{return err};if !info.IsDir()&&info.Name()=="test.txt"{raw,err:=os.ReadFile(path);if err!=nil{return err};if string(raw)==string(content){found=true}};return nil});if err!=nil||!found{t.Fatalf("restored content mismatch: %v",err)}
 original,err:=os.ReadFile(filepath.Join(source,"test.txt"));if err!=nil||string(original)!=string(content){t.Fatal("restore changed original data")}
 if _,err:=d.Run(ctx,c,s,Request{Action:"backup",Sources:[]string{base}});err==nil{t.Fatal("unapproved source accepted")}
 if _,err:=d.Run(ctx,c,s,Request{Action:"restore",Snapshot:"--target=/etc"});err==nil{t.Fatal("invalid snapshot argument accepted")}
}
