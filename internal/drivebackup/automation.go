package drivebackup

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "sort"
 "strings"
 "sync"
 "time"
 _ "time/tzdata"
)
var ErrScheduleConflict=errors.New("schedule changed; refresh before saving")
type ScheduleInput struct{
 ID string `json:"id"`;Version uint64 `json:"version"`;Name string `json:"name"`;DestinationID string `json:"destinationId"`;Enabled bool `json:"enabled"`;TimeZone string `json:"timeZone"`;Hour int `json:"hour"`;Minute int `json:"minute"`;Weekdays []int `json:"weekdays"`
}
type Schedule struct{
 ScheduleInput
 OwnerID uint `json:"ownerId"`;SourceBase string `json:"sourceBase"`;ConfigRevision string `json:"configRevision"`
 NextRunAt time.Time `json:"nextRunAt"`;LastAttemptAt time.Time `json:"lastAttemptAt"`;LastWindow string `json:"lastWindow"`;LastJobID string `json:"lastJobId"`;State string `json:"state"`;Message string `json:"message"`
}
type Automation struct{mu sync.Mutex;service *Service;schedules map[string]Schedule}
type ScheduledPlan func(context.Context,Config,Schedule)(DataBackupPlan,Actor,error)
func validateSchedule(input ScheduleInput)error{
 if strings.TrimSpace(input.Name)==""||!cleanValue(input.Name,100)||input.Hour<0||input.Hour>23||input.Minute<0||input.Minute>59||len(input.TimeZone)>128{return errors.New("invalid schedule name or time")}
 if _,err:=time.LoadLocation(input.TimeZone);err!=nil{return errors.New("invalid IANA time zone")}
 seen:=map[int]bool{};for _,day:=range input.Weekdays{if day<0||day>6||seen[day]{return errors.New("weekdays must be unique values from 0 to 6")};seen[day]=true};return nil
}
func NextScheduleRun(input ScheduleInput,after time.Time)(time.Time,error){
 if err:=validateSchedule(input);err!=nil{return time.Time{},err}
 location,_:=time.LoadLocation(input.TimeZone);local:=after.In(location)
 for offset:=0;offset<15;offset++{
  noon:=time.Date(local.Year(),local.Month(),local.Day()+offset,12,0,0,0,location)
  permitted:=len(input.Weekdays)==0;for _,day:=range input.Weekdays{if int(noon.Weekday())==day{permitted=true}}
  if !permitted{continue}
  candidate:=time.Date(noon.Year(),noon.Month(),noon.Day(),input.Hour,input.Minute,0,0,location)
  // Skip nonexistent wall-clock times. The persisted local-date window key
  // prevents an ambiguous repeated hour from creating two daily executions.
  if candidate.Hour()!=input.Hour||candidate.Minute()!=input.Minute||candidate.Day()!=noon.Day(){continue}
  if candidate.After(after){return candidate.UTC(),nil}
 }
 return time.Time{},errors.New("could not determine next scheduled run")
}
func NewAutomation(service *Service)(*Automation,error){
 a:=&Automation{service:service,schedules:map[string]Schedule{}}
 path:=filepath.Join(service.dir,"schedules.json")
 if info,err:=os.Lstat(path);err==nil{if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>1<<20{return nil,errors.New("invalid private schedule metadata")};raw,err:=os.ReadFile(path);if err!=nil{return nil,err};if json.Unmarshal(raw,&a.schedules)!=nil{return nil,errors.New("invalid schedule metadata")}}else if !os.IsNotExist(err){return nil,err}
 if a.schedules==nil{a.schedules=map[string]Schedule{}};if len(a.schedules)>64{return nil,errors.New("schedule limit exceeded")}
 changed:=false
 for id,item:=range a.schedules{
  if !restoreIDPattern.MatchString(id)||id!=item.ID||item.OwnerID==0||!filepath.IsAbs(item.SourceBase)||item.NextRunAt.IsZero()||validateSchedule(item.ScheduleInput)!=nil{return nil,errors.New("invalid saved schedule")}
  item.Weekdays=append([]int(nil),item.Weekdays...)
  switch item.State{case "pending-save":item.Enabled=false;item.State="blocked";item.Message="Schedule save was interrupted; review and save again.";changed=true
  case "claimed","running":item.State="interrupted";item.Message="Server restarted; the prior window is not automatically replayed. Inspect its backup job.";changed=true}
  a.schedules[id]=item
 }
 if changed{if err:=service.writeJSON("schedules.json",a.schedules);err!=nil{return nil,err}};return a,nil
}
func(a *Automation)List(owner uint,scope string)[]Schedule{
 a.mu.Lock();defer a.mu.Unlock();out:=make([]Schedule,0)
 for _,item:=range a.schedules{if item.OwnerID==owner&&item.SourceBase==scope{item.Weekdays=append([]int(nil),item.Weekdays...);out=append(out,item)}}
 sort.Slice(out,func(i,j int)bool{return out[i].Name<out[j].Name});return out
}
func(a *Automation)Save(input ScheduleInput,actor Actor,scope string,now time.Time)(Schedule,error){
 var empty Schedule
 if err:=validateSchedule(input);err!=nil{return empty,err}
 c,err:=a.service.Configuration(input.DestinationID);if err!=nil{return empty,err}
 if actor.UserID==0||actor.UserID!=c.SourceOwner||scope!=c.SourceBase{return empty,os.ErrPermission}
 if input.Enabled&&(!c.Enabled||len(c.SourcePaths)==0){return empty,errors.New("enable a configured destination and select folders before enabling its schedule")}
 a.mu.Lock();defer a.mu.Unlock()
 old,exists:=a.schedules[input.ID]
 if input.ID==""{if len(a.schedules)>=64{return empty,errors.New("schedule limit reached")};input.ID,err=randomID();if err!=nil{return empty,err}}else{
  if !exists{return empty,ErrNotFound};if old.OwnerID!=actor.UserID||old.SourceBase!=scope{return empty,os.ErrPermission};if input.Version!=old.Version{return empty,ErrScheduleConflict}
 }
 input.Version=old.Version+1;input.Weekdays=append([]int(nil),input.Weekdays...);sort.Ints(input.Weekdays)
 next,err:=NextScheduleRun(input,now);if err!=nil{return empty,err}
 item:=Schedule{ScheduleInput:input,OwnerID:actor.UserID,SourceBase:scope,ConfigRevision:c.Revision(),NextRunAt:next,LastAttemptAt:old.LastAttemptAt,LastWindow:old.LastWindow,LastJobID:old.LastJobID,State:"pending-save",Message:"Schedule settings are awaiting durable completion."}
 if err:=a.service.AuditAutomation("schedule.save.requested",actor,c.ID,input.ID,"requested");err!=nil{return empty,errors.New("schedule audit failed; settings not changed")}
 a.schedules[input.ID]=item
 if err:=a.service.writeJSON("schedules.json",a.schedules);err!=nil{if exists{a.schedules[input.ID]=old}else{delete(a.schedules,input.ID)};return empty,errors.New("schedule metadata could not be saved")}
 if err:=a.service.AuditAutomation("schedule.saved",actor,c.ID,input.ID,"success");err!=nil{return empty,errors.New("schedule saved in pending state but audit failed; refresh and save again")}
 pending:=item;item.State="idle";item.Message="Schedule saved. Existing running jobs are not canceled by disabling this schedule.";a.schedules[item.ID]=item
 if err:=a.service.writeJSON("schedules.json",a.schedules);err!=nil{a.schedules[item.ID]=pending;return empty,errors.New("schedule activation could not be persisted; refresh and save again")}
 return item,nil
}
func scheduleTerminal(state string)bool{return state!="queued"&&state!="running"&&state!="canceling"}
// Tick claims a due window durably before any repository write. An uncertain
// crash window is not replayed automatically. At most one pending window runs;
// missed days are collapsed rather than launched as a catch-up burst.
func(a *Automation)Tick(ctx context.Context,now time.Time,build ScheduledPlan)error{
 if build==nil{return errors.New("scheduled source-plan builder is unavailable")};if err:=ctx.Err();err!=nil{return err}
 jobs:=a.service.Jobs();a.mu.Lock();changed:=false
 for id,item:=range a.schedules{if item.State=="running"&&item.LastJobID!=""{for _,job:=range jobs{if job.ID==item.LastJobID&&scheduleTerminal(job.State){item.State=job.State;item.Message="Scheduled backup finished: "+job.State+". Review the backup job and recovery catalog.";a.schedules[id]=item;changed=true;break}}}}
 if changed{if err:=a.service.writeJSON("schedules.json",a.schedules);err!=nil{a.mu.Unlock();return err}}
 if a.service.Active(){a.mu.Unlock();return nil}
 ids:=make([]string,0);for id,item:=range a.schedules{if item.Enabled&&item.State!="pending-save"&&item.State!="claimed"&&item.State!="blocked"&&!item.NextRunAt.After(now){ids=append(ids,id)}};sort.Strings(ids)
 if len(ids)==0{a.mu.Unlock();return nil};id:=ids[0];item:=a.schedules[id]
 cfg,err:=a.service.Configuration(item.DestinationID)
 if err!=nil||!cfg.Enabled||cfg.SourceOwner!=item.OwnerID||cfg.SourceBase!=item.SourceBase||cfg.Revision()!=item.ConfigRevision{
  item.Enabled=false;item.State="blocked";item.Message="Destination settings or ownership changed; review and save this schedule again.";a.schedules[id]=item;err=a.service.writeJSON("schedules.json",a.schedules);a.mu.Unlock();return err
 }
 location,_:=time.LoadLocation(item.TimeZone);window:=item.NextRunAt.In(location).Format("2006-01-02")
 next,err:=NextScheduleRun(item.ScheduleInput,now);if err!=nil{a.mu.Unlock();return err}
 if window==item.LastWindow{item.NextRunAt=next;a.schedules[id]=item;err=a.service.writeJSON("schedules.json",a.schedules);a.mu.Unlock();return err}
 item.NextRunAt=next;item.LastWindow=window;item.LastAttemptAt=now.UTC();item.LastJobID="";item.State="claimed";item.Message="Scheduled window claimed; validating current account access.";a.schedules[id]=item
 if err:=a.service.writeJSON("schedules.json",a.schedules);err!=nil{item.State="blocked";item.Message="Window claim persistence failed; review schedule before resuming.";a.schedules[id]=item;a.mu.Unlock();return err};a.mu.Unlock()
 plan,actor,planErr:=build(ctx,cfg,item)
 a.mu.Lock();defer a.mu.Unlock();current:=a.schedules[id]
 if current.Version!=item.Version||!current.Enabled||current.State!="claimed"{return nil}
 if planErr==nil&&ctx.Err()!=nil{planErr=ctx.Err()}
 if planErr==nil&&actor.UserID!=item.OwnerID{planErr=os.ErrPermission}
 if planErr==nil{plan.scheduleID=id;plan.scheduleWindow=window;plan.expectedConfigRevision=item.ConfigRevision
  job,startErr:=a.service.StartBackup(StartRequest{DestinationID:cfg.ID,Action:"backup",ConfirmBackup:true},actor,plan)
  if startErr==nil{current.LastJobID=job.ID;current.State="running";current.Message="Scheduled backup queued. No automatic retry after uncertain writes."}else{planErr=startErr}
 }
 if planErr!=nil{current.State="failed";current.Message="Scheduled window did not start cleanly; inspect permissions, source access, engine availability and current jobs. No automatic replay.";if errors.Is(planErr,os.ErrPermission){current.Enabled=false;current.State="blocked"}}
 a.schedules[id]=current
 if err:=a.service.writeJSON("schedules.json",a.schedules);err!=nil{return errors.New("scheduled window outcome persistence failed; inspect jobs before retrying")}
 return nil
}
func(a *Automation)Run(ctx context.Context,build ScheduledPlan,onError func(error)){
 ticker:=time.NewTicker(15*time.Second);defer ticker.Stop()
 run:=func(){if err:=a.Tick(ctx,time.Now().UTC(),build);err!=nil&&ctx.Err()==nil&&onError!=nil{onError(err)}}
 run();for{select{case <-ctx.Done():return;case <-ticker.C:run()}}
}
