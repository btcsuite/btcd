// +build !windows

// Copyright (c) 2013-2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"os"
	"syscall"
)

// getAvailableDiskSpace returns the number of bytes of available disk space.
func getAvailableDiskSpace() (uint64, error) {
	var stat syscall.Statfs_t

	wd, err := os.Getwd()
	if err != nil {
		return 0, err
	}

	syscall.Statfs(wd, &stat)

	return stat.Bavail * uint64(stat.Bsize), nil
}
