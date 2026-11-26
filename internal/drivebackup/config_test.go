package drivebackup
import("encoding/json";"strings";"testing")
func sampleConfig()Config{return Config{ID:"offsite",Name:"Offsite",Enabled:true,Backend:"s3",Endpoint:"https://storage.example.test",Bucket:"drive-backups",Prefix:"millitop/drive",Addressing:"path",Connections:2}}
func sampleSecrets()Secrets{return Secrets{AccessKeyID:"example-access-key",SecretAccessKey:"example-secret-key",SessionToken:"example-session-token",RepositoryPassword:"example-recovery-password"}}
func TestProviderNeutralConfig(t *testing.T){
 c,s:=sampleConfig(),sampleSecrets();if err:=c.Validate(s);err!=nil{t.Fatal(err)};repo,err:=c.Repository();if err!=nil||repo!="s3:https://storage.example.test/drive-backups/millitop/drive"{t.Fatalf("repo=%s err=%v",repo,err)}
 for _,lookup:=range []string{"auto","path","dns",""}{copy:=c;copy.Addressing=lookup;if err:=copy.Validate(s);err!=nil{t.Fatal(err)}}
 for _,endpoint:=range []string{"https://user:secret@storage.example.test","http://storage.example.test","https://storage.example.test/path","https://storage.example.test?x=1","file:///etc/passwd"}{copy:=c;copy.Endpoint=endpoint;if copy.Validate(s)==nil{t.Errorf("accepted invalid endpoint %q",endpoint)}}
 for _,prefix:=range []string{"../data","/data","a//b","a/../b","a/"}{copy:=c;copy.Prefix=prefix;if copy.Validate(s)==nil{t.Errorf("accepted prefix %q",prefix)}}
 c.Enabled=false;c.Endpoint="";if err:=c.Validate(Secrets{});err!=nil{t.Fatal("unconfigured destination should be saveable",err)}
 raw,err:=json.Marshal(sampleSecrets());if err!=nil{t.Fatal(err)};if string(raw)!="{}"{t.Fatalf("secrets exposed in JSON: %s",raw)}
 if strings.Contains(sampleSecrets().String(),sampleSecrets().SecretAccessKey){t.Fatal("secrets exposed in string")}
}
