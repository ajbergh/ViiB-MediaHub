//go:build !windows && !linux && !darwin

package api

import "errors"

// Unrecognized hosts must never bypass the offline disk-space safeguard.
func podcastAvailableDiskBytes(string)(uint64,error){
    return 0,errors.New("podcast disk-space probing is unsupported")
}
