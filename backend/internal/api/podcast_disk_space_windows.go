//go:build windows

package api

import "golang.org/x/sys/windows"

// Windows uses per-user available bytes, not the total unused volume size.
func podcastAvailableDiskBytes(path string)(uint64,error){
    wide,err:=windows.UTF16PtrFromString(path)
    if err!=nil{return 0,err}
    var available,total,unused uint64
    if err:=windows.GetDiskFreeSpaceEx(wide,&available,&total,&unused);err!=nil{return 0,err}
    return available,nil
}
