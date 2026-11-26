package bolt

import (
 "context"
 "io"

 "github.com/asdine/storm/v3"
 bbolt "go.etcd.io/bbolt"
)

type databaseBackupWriter struct{ctx context.Context;writer io.Writer}
func(w databaseBackupWriter)Write(p []byte)(int,error){if err:=w.ctx.Err();err!=nil{return 0,err};return w.writer.Write(p)}

// The exported image comes from a consistent read transaction, not an ordinary
// live filesystem copy. There is deliberately no HTTP database-download route.
func databaseExporter(db *storm.DB)func(context.Context,io.Writer)error{
 return func(ctx context.Context,destination io.Writer)error{
  if err:=ctx.Err();err!=nil{return err}
  return db.Bolt.View(func(tx *bbolt.Tx)error{_,err:=tx.WriteTo(databaseBackupWriter{ctx,destination});return err})
 }
}
