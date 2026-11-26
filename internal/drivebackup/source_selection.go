package drivebackup

import("errors";"path";"strings";"unicode/utf8")

// Sources are Drive-account virtual directories, not arbitrary host paths.
// Owner/base bind the selection to the administrator's current file scope.
func validateSourceSelection(paths []string,owner uint,base string)error{
 if len(paths)>16{return errors.New("select at most 16 backup folders")}
 if len(paths)==0{return nil}
 if owner==0||base==""||len(base)>4096{return errors.New("backup folder selection has no valid account-scope binding")}
 total:=0
 for i,current:=range paths{
  total+=len(current)
  if !utf8.ValidString(current)||current=="/"||current==""||len(current)>1024||!strings.HasPrefix(current,"/")||strings.ContainsAny(current,"\\\x00")||path.Clean(current)!=current||strings.IndexFunc(current,func(c rune)bool{return c<32||c==127})>=0{return errors.New("invalid backup folder path")}
  for _,part:=range strings.Split(current,"/"){if part=="."||part==".."{return errors.New("invalid backup folder component")}}
  for _,previous:=range paths[:i]{if current==previous||strings.HasPrefix(current,previous+"/")||strings.HasPrefix(previous,current+"/"){return errors.New("backup folders must not overlap")}}
 }
 if total>4096{return errors.New("backup folder selection is too long")}
 return nil
}
