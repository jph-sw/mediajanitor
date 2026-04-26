//go:build linux

package scanner

import (
	"os"
	"syscall"
)

const hardlinkDetectionAvailable = true

func inode(fi os.FileInfo) (uint64, bool) {
	sys, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return sys.Ino, true
}
