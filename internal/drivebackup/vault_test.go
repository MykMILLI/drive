package drivebackup
import("os";"path/filepath";"strings";"testing")
func TestVaultRoundTripAndTamper(t *testing.T){
 dir := filepath.Join(t.TempDir(), "vault");v,err:=NewVault(dir);if err!=nil{t.Fatal(err)};secrets:=sampleSecrets();if err:=v.Save("offsite",secrets);err!=nil{t.Fatal(err)}
 raw,err:=os.ReadFile(filepath.Join(dir,"offsite.secrets"));if err!=nil{t.Fatal(err)};if strings.Contains(string(raw),secrets.SecretAccessKey)||strings.Contains(string(raw),secrets.RepositoryPassword){t.Fatal("plaintext secret persisted")}
 reopened,err:=NewVault(dir);if err!=nil{t.Fatal(err)};loaded,err:=reopened.Load("offsite");if err!=nil||loaded!=secrets{t.Fatalf("roundtrip failed: %v",err)}
 raw[len(raw)-1]^=1;if err:=os.WriteFile(filepath.Join(dir,"offsite.secrets"),raw,0600);err!=nil{t.Fatal(err)};if _,err:=v.Load("offsite");err==nil{t.Fatal("tampered secrets accepted")}
 if err:=os.Remove(filepath.Join(dir,"master.key"));err!=nil{t.Fatal(err)};if _,err:=NewVault(dir);err==nil{t.Fatal("missing key silently replaced")}
}


func TestVaultRejectsInsecureDirectory(t *testing.T) {
    dir := filepath.Join(t.TempDir(), "insecure-vault")

    if err := os.Mkdir(dir, 0755); err != nil {
        t.Fatal(err)
    }
    if err := os.Chmod(dir, 0755); err != nil {
        t.Fatal(err)
    }

    if _, err := NewVault(dir); err == nil {
        t.Fatal("vault accepted a directory with group/other access")
    }

    if _, err := os.Stat(filepath.Join(dir, "master.key")); !os.IsNotExist(err) {
        t.Fatal("rejected vault must not create a master key")
    }
}

func TestVaultRejectsSymlinkDirectory(t *testing.T) {
    parent := t.TempDir()
    target := filepath.Join(parent, "real-vault")
    link := filepath.Join(parent, "linked-vault")

    if err := os.Mkdir(target, 0700); err != nil {
        t.Fatal(err)
    }
    if err := os.Symlink(target, link); err != nil {
        t.Skipf("symlinks unavailable: %v", err)
    }

    if _, err := NewVault(link); err == nil {
        t.Fatal("vault accepted a symlink directory")
    }

    if _, err := os.Stat(filepath.Join(target, "master.key")); !os.IsNotExist(err) {
        t.Fatal("rejected symlink must not create a master key")
    }
}
