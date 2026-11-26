package drivebackup
import("context";"os";"path/filepath";"testing";"time")
func TestScheduleCalendarDSTAndWeekdays(t *testing.T){
 spec:=ScheduleInput{Name:"DST",TimeZone:"America/New_York",Hour:2,Minute:30}
 after:=time.Date(2026,3,8,5,0,0,0,time.UTC);next,err:=NextScheduleRun(spec,after);if err!=nil{t.Fatal(err)}
 zone,_:=time.LoadLocation(spec.TimeZone);local:=next.In(zone);if local.Day()!=9||local.Hour()!=2||local.Minute()!=30{t.Fatalf("nonexistent spring time not skipped: %v",local)}
 spec.Hour=1;next,err=NextScheduleRun(spec,time.Date(2026,11,1,4,0,0,0,time.UTC));if err!=nil{t.Fatal(err)}
 if local:=next.In(zone);local.Day()!=1||local.Hour()!=1||local.Minute()!=30{t.Fatalf("fall occurrence: %v",local)}
 spec.TimeZone="UTC";spec.Hour=2;spec.Minute=0;spec.Weekdays=[]int{1}
 next,err=NextScheduleRun(spec,time.Date(2026,11,23,3,0,0,0,time.UTC));if err!=nil||next.Weekday()!=time.Monday||next.Day()!=30{t.Fatalf("weekday: %v %v",next,err)}
 spec.Weekdays=[]int{1,1};if validateSchedule(spec)==nil{t.Fatal("duplicate weekdays allowed")}
}
func TestScheduleDisabledAndInterruptedNoReplay(t *testing.T){
 service,err:=NewService(filepath.Join(t.TempDir(),"state"));if err!=nil{t.Fatal(err)}
 actor:=Actor{UserID:1,Username:"admin"};scope:=t.TempDir();cfg:=Config{ID:"schedule-test",Name:"Schedule",Enabled:false,Backend:"local",LocalRepository:filepath.Join(t.TempDir(),"repo"),SourceOwner:1,SourceBase:scope}
 if _,err:=service.Save(cfg,SecretUpdate{},actor);err!=nil{t.Fatal(err)}
 automation,err:=NewAutomation(service);if err!=nil{t.Fatal(err)}
 now:=time.Date(2026,11,23,12,0,0,0,time.UTC)
 input:=ScheduleInput{Name:"Daily",DestinationID:cfg.ID,TimeZone:"UTC",Hour:2,Minute:0}
 saved,err:=automation.Save(input,actor,scope,now);if err!=nil{t.Fatal(err)}
 called:=0;build:=func(context.Context,Config,Schedule)(DataBackupPlan,Actor,error){called++;return DataBackupPlan{},actor,nil}
 if err:=automation.Tick(context.Background(),now.Add(48*time.Hour),build);err!=nil{t.Fatal(err)};if called!=0{t.Fatal("disabled schedule executed")}
 // Simulate a durably claimed window before a crash. No write is replayed.
 saved.Enabled=true;saved.State="claimed";saved.LastWindow="2026-11-23";saved.NextRunAt=now.Add(24*time.Hour);automation.schedules[saved.ID]=saved
 if err:=service.writeJSON("schedules.json",automation.schedules);err!=nil{t.Fatal(err)}
 reopened,err:=NewAutomation(service);if err!=nil{t.Fatal(err)};if reopened.List(1,scope)[0].State!="interrupted"{t.Fatal("uncertain claim not marked interrupted")}
 if err:=reopened.Tick(context.Background(),now,build);err!=nil{t.Fatal(err)};if called!=0{t.Fatal("uncertain window replayed")}
 input=saved.ScheduleInput;input.Enabled=false;input.Version++
 if _,err:=reopened.Save(input,actor,scope,now);err!=ErrScheduleConflict{t.Fatalf("stale version: %v",err)}
 input.Version=saved.Version;if _,err:=reopened.Save(input,Actor{UserID:2},scope,now);!os.IsPermission(err){t.Fatalf("foreign owner save: %v",err)}
}
