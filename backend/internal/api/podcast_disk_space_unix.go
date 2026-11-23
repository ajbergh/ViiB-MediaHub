//go:build linux || darwin

package api

import (
    "errors"
    "golang.org/x/sys/unix"
)

// podcastAvailableDiskBytes returns space available to this process on the
// actual filesystem containing the private podcast episode directory.
func podcastAvailableDiskBytes(path string)(uint64,error){
    var info unix.Statfs_t
    if err:=unix.Statfs(path,&info);err!=nil{return 0,err}
    blocks:=uint64(info.Bavail)
    blockSize:=uint64(info.Bsize)
    if blockSize==0{return 0,errors.New("unknown disk block size")}
    if blocks>^uint64(0)/blockSize{return ^uint64(0),nil}
    return blocks*blockSize,nil
}
