//go:build !linux

package fbhttp
import "io"
func driveCreateNewFile(scope,root,name string,body io.Reader,length int64,digest string,mode uint32,authorize func()error)error{return errDriveUploadUnsupported}
func driveCreateDirectory(scope,root,name string)error{return errDriveUploadUnsupported}
func driveReadDeviceFile(scope,root,name string)([]byte,error){return nil,errDriveUploadUnsupported}
