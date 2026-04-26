//go:build !linux

package scanner

import "os"

const hardlinkDetectionAvailable = false

func inode(_ os.FileInfo) (uint64, bool) {
	return 0, false
}
