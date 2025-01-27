// +build windows

// Copyright (c) 2013-2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"os"
	"syscall"
	"unsafe"
)

// getAvailableDiskSpace returns the number of bytes of available disk space.
func getAvailableDiskSpace() (uint64, error) {
	wd, err := os.Getwd()
	if err != nil {
		return 0, err
	}

	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	var freeBytes int64
	_, _, err := c.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(wd))),
		uintptr(unsafe.Pointer(&freeBytes)), nil, nil)
	if err != nil {
		return 0, err
	}

	return uint64(freeBytes), nil
}
