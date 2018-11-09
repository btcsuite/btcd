// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package commandline

// ExecutablePathProvider wraps class responsible for executable
// path resolution
type ExecutablePathProvider interface {
	// Executable returns full path to an executable target file
	Executable() string
}
