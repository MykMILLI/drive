package drivebackup
import("errors";"syscall")
func restoreDiskSpace(path string)(uint64,uint64,error){var stat syscall.Statfs_t;if err:=syscall.Statfs(path,&stat);err!=nil{return 0,0,err};if stat.Bsize<=0{return 0,0,errors.New("invalid staging filesystem block size")};return stat.Bavail*uint64(stat.Bsize),stat.Ffree,nil}
