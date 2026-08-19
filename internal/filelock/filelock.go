// Package filelock provides advisory whole-file locks that serialize
// work between separate slk processes. Nothing stops a user running two
// instances against one config and one data directory, and the OS
// releases these locks when the holding process exits — however it
// exits — so a crashed instance cannot wedge the next one.
package filelock

import "os"

// Lock is an exclusive advisory lock on a lock file.
//
// The lock file is a sidecar, never the file being protected: an atomic
// save replaces its target by renaming a new file over it, so a lock
// taken on the target would end up held on an inode no other process
// opens.
type Lock struct {
	path string
	f    *os.File
}

// New returns an unheld Lock on path. The file is created by the first
// Lock or TryLock call.
func New(path string) *Lock {
	return &Lock{path: path}
}
