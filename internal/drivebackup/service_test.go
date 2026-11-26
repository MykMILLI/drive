package drivebackup

import("context";"encoding/json";"os";"os/exec";"path/filepath";"strings";"testing";"time")
func ptr(value string)*string{return &value}
func TestServicePersistsRedactedDestination(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"state");service,err:=NewService(dir);if err!=nil{t.Fatal(err)}
 c:=sampleConfig();c.Enabled=false;s:=sampleSecrets();actor:=Actor{UserID:1,Username:"admin"}
 saved,err:=service.Save(c,SecretUpdate{AccessKeyID:ptr(s.AccessKeyID),SecretAccessKey:ptr(s.SecretAccessKey),SessionToken:ptr(s.SessionToken),RepositoryPassword:ptr(s.RepositoryPassword)},actor);if err!=nil{t.Fatal(err)}
 if !saved.Secrets.PasswordSet{t.Fatal("missing secret-state flag")}
 reopened,err:=NewService(dir);if err!=nil{t.Fatal(err)};list,err:=reopened.Destinations();if err!=nil||len(list)!=1{t.Fatalf("reload: %v",err)}
 output,_:=json.Marshal(list);for _,secret:=range []string{s.AccessKeyID,s.SecretAccessKey,s.SessionToken,s.RepositoryPassword}{if strings.Contains(string(output),secret){t.Fatal("secret leaked in public destination")}}
 for _,file:=range []string{"destinations.json","audit.jsonl"}{raw,err:=os.ReadFile(filepath.Join(dir,file));if err!=nil{t.Fatal(err)};if strings.Contains(string(raw),s.SecretAccessKey)||strings.Contains(string(raw),s.RepositoryPassword){t.Fatal("secret leaked to metadata/audit")}}
 c.Endpoint="https://other.example.test";if _,err:=service.Save(c,SecretUpdate{},actor);err==nil{t.Fatal("endpoint changed while retaining old credentials")}
}
func TestServiceLocalRepositoryJob(t *testing.T){
 if _,err:=exec.LookPath("restic");err!=nil{if os.Getenv("DRIVE_REQUIRE_RESTIC_TEST")=="1"{t.Fatal(err)};t.Skip("restic not installed")}
 base:=t.TempDir();dir:=filepath.Join(base,"state");service,err:=NewService(dir);if err!=nil{t.Fatal(err)}
 c:=Config{ID:"local-service-test",Name:"local integration",Backend:"local",Enabled:true,LocalRepository:filepath.Join(base,"repo")};password:="service-integration-password"
 if _,err:=service.Save(c,SecretUpdate{RepositoryPassword:&password},Actor{UserID:1});err!=nil{t.Fatal(err)}
 if _,err:=service.Start(StartRequest{DestinationID:c.ID,Action:"initialize"},Actor{});err!=ErrConfirmation{t.Fatalf("initialization without acknowledgement: %v",err)}
 job,err:=service.Start(StartRequest{DestinationID:c.ID,Action:"initialize",ConfirmInitialize:true,RecoveryAcknowledged:true},Actor{UserID:1});if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
 for{done:=false;for _,current:=range service.Jobs(){if current.ID==job.ID&&current.State!="queued"&&current.State!="running"{if current.State!="succeeded"{t.Fatalf("job failed: %+v",current)};done=true}}
  if done{break};select{case<-ctx.Done():t.Fatal("job timed out");case<-time.After(50*time.Millisecond):}}
 reopened,err:=NewService(dir);if err!=nil{t.Fatal(err)};if len(reopened.Jobs())!=1||reopened.Jobs()[0].State!="succeeded"{t.Fatal("job history not persisted")}
 if _,err:=service.Start(StartRequest{DestinationID:c.ID,Action:"backup"},Actor{});err==nil{t.Fatal("data backup exposed before integration")}
}

func TestServiceBlocksInterruptedCredentialTransaction(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"state");service,err:=NewService(dir);if err!=nil{t.Fatal(err)}
 c:=sampleConfig();c.Enabled=false;secrets:=sampleSecrets();update:=SecretUpdate{AccessKeyID:ptr(secrets.AccessKeyID),SecretAccessKey:ptr(secrets.SecretAccessKey),RepositoryPassword:ptr(secrets.RepositoryPassword)}
 if _,err:=service.Save(c,update,Actor{});err!=nil{t.Fatal(err)}
 marker:=filepath.Join(dir,".pending-destination-"+c.ID+".json");if err:=os.WriteFile(marker,[]byte(`{"destinationId":"offsite"}`),0600);err!=nil{t.Fatal(err)}
 reopened,err:=NewService(dir);if err!=nil{t.Fatal(err)}
 if !reopened.blocked[c.ID]{t.Fatal("incomplete credential update not blocked after restart")}
 if _,err:=reopened.Save(c,SecretUpdate{},Actor{});err==nil{t.Fatal("repair accepted without fresh credentials")}
 if _,err:=reopened.Save(c,update,Actor{});err!=nil{t.Fatal(err)}
 if reopened.blocked[c.ID]{t.Fatal("successful credential repair remains blocked")}
 if _,err:=os.Stat(marker);!os.IsNotExist(err){t.Fatal("completed transaction marker retained")}
}
