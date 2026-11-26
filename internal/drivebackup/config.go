package drivebackup

import (
 "errors"
 "fmt"
 "net/url"
 "path/filepath"
 "regexp"
 "strings"
)

var ErrDisabled=errors.New("backup destination is disabled")
var validBucket=regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
var validPrefix=regexp.MustCompile(`^[A-Za-z0-9._/-]*$`)
var validID=regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Config contains public destination settings, never credentials.
// Local repositories are supported for isolated backend tests and local jobs;
// the eventual admin UI must explicitly distinguish them from offsite S3.
type Config struct {
 SourcePaths []string `json:"sourcePaths,omitempty"`
 SourceOwner uint `json:"sourceOwner,omitempty"`
 SourceBase string `json:"sourceBase,omitempty"`
 ID string `json:"id"`
 Name string `json:"name"`
 Enabled bool `json:"enabled"`
 Backend string `json:"backend"`
 Endpoint string `json:"endpoint"`
 Region string `json:"region"`
 Bucket string `json:"bucket"`
 Prefix string `json:"prefix"`
 Addressing string `json:"addressing"`
 Connections int `json:"connections"`
 UploadKiB int `json:"uploadKiB"`
 DownloadKiB int `json:"downloadKiB"`
 AllowHTTP bool `json:"allowHTTP"`
 CABundle string `json:"caBundle"`
 LocalRepository string `json:"localRepository,omitempty"`
}

type Secrets struct {
 AccessKeyID string `json:"-"`
 SecretAccessKey string `json:"-"`
 SessionToken string `json:"-"`
 RepositoryPassword string `json:"-"`
}
func (Secrets) String()string{return "[REDACTED BACKUP SECRETS]"}
func (Secrets) GoString()string{return "[REDACTED BACKUP SECRETS]"}

type SecretState struct {
 AccessKeySet bool `json:"accessKeySet"`
 SecretKeySet bool `json:"secretKeySet"`
 SessionTokenSet bool `json:"sessionTokenSet"`
 PasswordSet bool `json:"passwordSet"`
}
func (s Secrets) State()SecretState{return SecretState{s.AccessKeyID!="",s.SecretAccessKey!="",s.SessionToken!="",s.RepositoryPassword!=""}}

func cleanValue(value string,limit int)bool{return len(value)<=limit&&strings.IndexFunc(value,func(c rune)bool{return c<32||c==127})<0}
func (c Config) Validate(s Secrets)error{
 if !validID.MatchString(c.ID){return errors.New("invalid destination ID")}
 if !cleanValue(c.Name,100){return errors.New("invalid destination name")}
 if err:=validateSourceSelection(c.SourcePaths,c.SourceOwner,c.SourceBase);err!=nil{return err}
 if !c.Enabled{return nil}
 if c.UploadKiB<0||c.DownloadKiB<0||c.UploadKiB>100000000||c.DownloadKiB>100000000{return errors.New("invalid bandwidth limit")}
 if c.Connections<0||c.Connections>64{return errors.New("connections must be between 0 and 64")}
 if !cleanValue(c.Region,128){return errors.New("invalid region")}
 if len(s.RepositoryPassword)<16||strings.ContainsRune(s.RepositoryPassword,'\x00'){return errors.New("repository password must have at least 16 characters and no NUL")}
 if c.CABundle!=""&&(!filepath.IsAbs(c.CABundle)||!cleanValue(c.CABundle,4096)){return errors.New("CA bundle must be an absolute local path")}
 switch c.Backend{
 case "local":
  if !filepath.IsAbs(c.LocalRepository)||!cleanValue(c.LocalRepository,4096){return errors.New("local repository must be an absolute path")}
 case "s3":
  u,err:=url.Parse(c.Endpoint)
  if err!=nil||u==nil||u.Hostname()==""||len(c.Endpoint)>2048||!cleanValue(c.Endpoint,2048)||u.User!=nil||u.RawQuery!=""||u.Fragment!=""||(u.Path!=""&&u.Path!="/"){return errors.New("S3 endpoint must be a root URL without embedded credentials, query or fragment")}
  if u.Scheme!="https"&&!(u.Scheme=="http"&&c.AllowHTTP){return errors.New("HTTPS is required unless HTTP is explicitly allowed for testing")}
  if !validBucket.MatchString(c.Bucket)||strings.Contains(c.Bucket,".."){return errors.New("invalid S3 bucket name")}
  if !validPrefix.MatchString(c.Prefix)||len(c.Prefix)>512||strings.HasPrefix(c.Prefix,"/")||strings.HasSuffix(c.Prefix,"/"){return errors.New("invalid repository prefix")}
  if c.Prefix!=""{for _,part:=range strings.Split(c.Prefix,"/"){if part==""||part=="."||part==".."{return errors.New("invalid prefix component")}}}
  if c.Addressing!=""&&c.Addressing!="auto"&&c.Addressing!="path"&&c.Addressing!="dns"{return errors.New("addressing must be auto, path or dns")}
  if s.AccessKeyID==""||s.SecretAccessKey==""{return errors.New("S3 access key and secret key are required")}
  for _,value:=range []string{s.AccessKeyID,s.SecretAccessKey,s.SessionToken}{if strings.ContainsRune(value,'\x00'){return errors.New("invalid credential")}}
 default:return errors.New("backend must be s3 or local")
 }
 return nil
}
func (c Config) Repository()(string,error){
 switch c.Backend{
 case "local":return filepath.Clean(c.LocalRepository),nil
 case "s3":
  u,err:=url.Parse(c.Endpoint);if err!=nil||u==nil{return "",errors.New("invalid S3 endpoint")}
  u.Path="/"+c.Bucket
  if c.Prefix!=""{u.Path+="/"+c.Prefix}
  return "s3:"+u.String(),nil
 default:return "",fmt.Errorf("unsupported backend %q",c.Backend)
 }
}
