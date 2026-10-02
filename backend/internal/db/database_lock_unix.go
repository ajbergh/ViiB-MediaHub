//go:build !windows

package db

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockDatabaseFile(file *os.File, exclusive bool) error {
	flags := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		flags = unix.LOCK_EX | unix.LOCK_NB
	}
	return unix.Flock(int(file.Fd()), flags)
}
