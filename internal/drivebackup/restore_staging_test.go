package drivebackup
import("context";"os";"path/filepath";"strings";"testing";"time")
func TestRestoreBudgetAndPrivateBase(t *testing.T){
 if restoreBudget(1,1,5)!=nil{t.Fatal("small restore rejected")}
 for _,limits:=range [][3]uint64{{0,1,5},{1,0,5},{6,1,5},{1,50001,5}}{if restoreBudget(limits[0],limits[1],limits[2])==nil{t.Fatalf("bad restore budget accepted: %v",limits)}}
 work:=t.TempDir();outside:=t.TempDir();if err:=os.Symlink(outside,filepath.Join(work,"restores"));err!=nil{t.Fatal(err)}
 if _,err:=privateRestoreBase(work);err==nil{t.Fatal("symlink staging base accepted")}
}
func TestRestoreVerifierRejectsSymlink(t *testing.T){
 stage:=t.TempDir();if err:=os.Symlink(t.TempDir(),filepath.Join(stage,"escape"));err!=nil{t.Fatal(err)}
 if _,_,err:=verifyStagedRestore(context.Background(),stage,Job{},1<<20);err==nil{t.Fatal("restored symlink accepted")}
}
func TestRestoreRegistrySurvivesHistoryRolloverAndRestart(t *testing.T){
 service,err:=NewService(filepath.Join(t.TempDir(),"state"));if err!=nil{t.Fatal(err)}
 id:=strings.Repeat("a",32);service.artifacts[id]=RestoreArtifact{ID:id,OwnerID:1,SourceBase:"/srv",State:"preparing",CreatedAt:time.Now()}
 if err:=service.writeJSON("restore-artifacts.json",service.artifacts);err!=nil{t.Fatal(err)}
 reopened,err:=NewService(service.dir);if err!=nil{t.Fatal(err)}
 artifacts:=reopened.RestoreArtifacts();if len(artifacts)!=1||artifacts[0].State!="interrupted"{t.Fatalf("interrupted stage not recorded: %+v",artifacts)}
}
