//go:build !linux

package drivebackup
import "errors"
func restoreDiskSpace(string)(uint64,uint64,error){return 0,0,errors.New("staging restore disk safeguards currently require Linux")}
