package drivebackup

import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strconv"
 "strings"
 "time"
)

var ErrBusy=errors.New("another backup operation is already running")
var ErrOutputLimit=errors.New("restic output exceeded the capture limit; do not treat it as complete")
var snapshotPattern=regexp.MustCompile(`^[a-f0-9]{8,64}$`)

type Request struct {
 Action string
 RestoreID string
 VerifyRestore bool
 Sources []string
 Snapshot string
 IntegrityPercent int
 Timeout time.Duration
}
type Result struct {
 DatabaseExportPath string `json:"-"`
 Action string `json:"action"`
 ExitCode int `json:"exitCode"`
 StartedAt time.Time `json:"startedAt"`
 FinishedAt time.Time `json:"finishedAt"`
 RestoreDirectory string `json:"restoreDirectory,omitempty"`
 Stdout string `json:"stdout,omitempty"`
 Stderr string `json:"stderr,omitempty"`
}
type Driver struct { binary,workDir string; roots []string; gate chan struct{} }

func NewDriver(workDir string,allowedRoots []string)(*Driver,error){
 binary,err:=exec.LookPath("restic");if err!=nil{return nil,errors.New("restic executable is not installed")}
 if !filepath.IsAbs(workDir){return nil,errors.New("backup work directory must be absolute")}
 if err:=os.MkdirAll(workDir,0700);err!=nil{return nil,err}
 realWork,err:=filepath.EvalSymlinks(workDir);if err!=nil{return nil,err}
 roots:=make([]string,0,len(allowedRoots));for _,root:=range allowedRoots{
  if !filepath.IsAbs(root){return nil,errors.New("allowed source roots must be absolute")}
  resolved,err:=filepath.EvalSymlinks(root);if err!=nil{return nil,err};roots=append(roots,resolved)
 }
 return &Driver{binary:binary,workDir:realWork,roots:roots,gate:make(chan struct{},1)},nil
}
func within(child,parent string)bool{rel,err:=filepath.Rel(parent,child);return err==nil&&rel!=".."&&!strings.HasPrefix(rel,".."+string(os.PathSeparator))&&!filepath.IsAbs(rel)}

func (d *Driver) sources(c Config,input []string)([]string,error){
 if len(input)==0||len(input)>32{return nil,errors.New("select between 1 and 32 sources")}
 result:=make([]string,0,len(input))
 for _,source:=range input{
  if !filepath.IsAbs(source){return nil,errors.New("source must be absolute")}
  real,err:=filepath.EvalSymlinks(source);if err!=nil{return nil,err}
  allowed:=false;for _,root:=range d.roots{if within(real,root){allowed=true;break}}
  if !allowed{return nil,errors.New("source is outside administrator-approved roots")}
  if within(d.workDir,real)||within(real,d.workDir){return nil,errors.New("backup source overlaps the private work directory")}
  if c.Backend=="local"{repo:=filepath.Clean(c.LocalRepository);if within(repo,real)||within(real,repo){return nil,errors.New("source and local repository overlap")}}
  result=append(result,real)
 }
 return result,nil
}

// Construct a fixed action set; never accept arbitrary flags, shell commands,
// user-supplied restore paths, password commands or inherited RESTIC/AWS env.
func (d *Driver) command(c Config,s Secrets,r Request)([]string,[]string,string,error){
 if err:=c.Validate(s);err!=nil{return nil,nil,"",err};if !c.Enabled{return nil,nil,"",ErrDisabled}
 repo,err:=c.Repository();if err!=nil{return nil,nil,"",err}
 args:=[]string{"--repo",repo,"--json","--cache-dir",filepath.Join(d.workDir,"cache")}
 if c.UploadKiB>0{args=append(args,"--limit-upload",strconv.Itoa(c.UploadKiB))}
 if c.DownloadKiB>0{args=append(args,"--limit-download",strconv.Itoa(c.DownloadKiB))}
 if c.CABundle!=""{info,err:=os.Stat(c.CABundle);if err!=nil||!info.Mode().IsRegular(){return nil,nil,"",errors.New("CA bundle not available")};args=append(args,"--cacert",c.CABundle)}
 if c.Backend=="s3"{
  lookup:=c.Addressing;if lookup==""{lookup="auto"};connections:=c.Connections;if connections==0{connections=2}
  args=append(args,"-o","s3.bucket-lookup="+lookup,"-o","s3.connections="+strconv.Itoa(connections))
  if c.Region!=""{args=append(args,"-o","s3.region="+c.Region)}
 }
 env:=make([]string,0)
 for _,entry:=range os.Environ(){name:=strings.ToUpper(strings.SplitN(entry,"=",2)[0]);if strings.HasPrefix(name,"AWS_")||strings.HasPrefix(name,"RESTIC_")||strings.HasPrefix(name,"RCLONE_"){continue};env=append(env,entry)}
 env=append(env,"RESTIC_PASSWORD="+s.RepositoryPassword,"AWS_EC2_METADATA_DISABLED=true")
 if c.Backend=="s3"{env=append(env,"AWS_ACCESS_KEY_ID="+s.AccessKeyID,"AWS_SECRET_ACCESS_KEY="+s.SecretAccessKey);if s.SessionToken!=""{env=append(env,"AWS_SESSION_TOKEN="+s.SessionToken)}}
 restore:=""
 switch r.Action{
 case "initialize":args=append(args,"init")
 case "snapshots":args=append(args,"snapshots")
 case "check":
  if r.IntegrityPercent<0||r.IntegrityPercent>100{return nil,nil,"",errors.New("invalid integrity sample percentage")}
  args=append(args,"check");if r.IntegrityPercent==100{args=append(args,"--read-data")}else if r.IntegrityPercent>0{args=append(args,"--read-data-subset="+strconv.Itoa(r.IntegrityPercent)+"%")}
 case "backup":
  paths,err:=d.sources(c,r.Sources);if err!=nil{return nil,nil,"",err}
  args=append(args,"backup","--host","millitop-drive","--tag",c.ID)
  if stream:=c.BackupSetID();stream!=""{args=append(args,"--tag","drive-set-"+stream)}
  args=append(args,"--");args=append(args,paths...)
 case "restore-stats":
  if len(r.Snapshot)!=64||!snapshotPattern.MatchString(r.Snapshot){return nil,nil,"",errors.New("full snapshot ID is required")}
  args=append(args,"stats",r.Snapshot,"--mode","restore-size")
 case "restore":
  if r.Snapshot!="latest"&&!snapshotPattern.MatchString(r.Snapshot){return nil,nil,"",errors.New("invalid snapshot ID")}
  base,baseErr:=privateRestoreBase(d.workDir);if baseErr!=nil{return nil,nil,"",baseErr}
  if r.RestoreID!=""{if !restoreIDPattern.MatchString(r.RestoreID){return nil,nil,"",errors.New("invalid restore staging ID")};restore=filepath.Join(base,r.RestoreID);err=os.Mkdir(restore,0700)}else{restore,err=os.MkdirTemp(base,"restore-")}
  if err!=nil{return nil,nil,"",err}
  args=append(args,"restore",r.Snapshot,"--target",restore)
  if r.VerifyRestore{args=append(args,"--verify")}
 default:return nil,nil,"",errors.New("unsupported backup operation")
 }
 return args,env,restore,nil
}

type boundedOutput struct { bytes.Buffer; truncated bool }
func (b *boundedOutput) Write(p []byte)(int,error){n:=len(p);remaining:=(4<<20)-b.Len();if remaining>0{take:=n;if take>remaining{take=remaining};_,_=b.Buffer.Write(p[:take])};if n>remaining{b.truncated=true};return n,nil}
func redact(value string,s Secrets)string{for _,secret:=range []string{s.SecretAccessKey,s.SessionToken,s.RepositoryPassword,s.AccessKeyID}{if secret!=""{value=strings.ReplaceAll(value,secret,"[REDACTED]")}};return value}

// Run is synchronous, cancellable, and serialized per Driver. It does not
// schedule work, persist job history, or automatically initialize repositories.
func (d *Driver) Run(ctx context.Context,c Config,s Secrets,r Request)(Result,error){
 result:=Result{Action:r.Action,ExitCode:-1}
 select{case d.gate<-struct{}{}:defer func(){<-d.gate}();default:return result,ErrBusy}
 if r.Timeout<0||r.Timeout>24*time.Hour{return result,errors.New("invalid timeout")};timeout:=r.Timeout;if timeout==0{timeout=time.Hour}
 ctx,cancel:=context.WithTimeout(ctx,timeout);defer cancel()
 args,env,restore,err:=d.command(c,s,r);if err!=nil{return result,err};result.RestoreDirectory=restore
 cmd:=exec.CommandContext(ctx,d.binary,args...);cmd.Env=env;cmd.Dir=d.workDir
 var stdout,stderr boundedOutput;cmd.Stdout=&stdout;cmd.Stderr=&stderr
 result.StartedAt=time.Now().UTC();err=cmd.Run();result.FinishedAt=time.Now().UTC()
 result.Stdout=redact(stdout.String(),s);result.Stderr=redact(stderr.String(),s)
 if ctx.Err()!=nil{return result,ctx.Err()}
 if err!=nil{var exit *exec.ExitError;if errors.As(err,&exit){result.ExitCode=exit.ExitCode();return result,fmt.Errorf("restic operation failed with exit code %d",result.ExitCode)};return result,errors.New("restic could not be executed")}
 result.ExitCode=0
 if stdout.truncated||stderr.truncated{return result,ErrOutputLimit}
 return result,nil
}
