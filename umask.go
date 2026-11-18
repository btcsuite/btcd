// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build unix

package main

import "syscall"

func init() {
	// Restrict newly created files to their owner by default: the log
	// file, for one, holds peer addresses.  Setting the mask in init
	// applies it before any file is created.
	syscall.Umask(0077)
}
