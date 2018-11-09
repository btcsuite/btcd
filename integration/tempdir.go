// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package integration

import "path/filepath"

// TempDirHandler offers temporary directories management.
type TempDirHandler struct {
	target string
}

// NewTempDir creates new immutable instance of the TempDirHandler.
func NewTempDir(targetParent string, targetName string) *TempDirHandler {
	return &TempDirHandler{
		target: filepath.Join(targetParent, targetName),
	}
}

// Dispose is required for TempDirHandler to implement LeakyAsset
func (t *TempDirHandler) Dispose() {
	if t.Exists() {
		DeleteFile(t.target)
	}
	DeRegisterDisposableAsset(t)
}

// MakeDir ensures target folder and all it's parents exist.
// Registers created folder as a leaky asset.
func (t *TempDirHandler) MakeDir() *TempDirHandler {
	if !t.Exists() {
		MakeDirs(t.target)
	}
	RegisterDisposableAsset(t)
	return t
}

// Exists returns true when target exists.
func (t *TempDirHandler) Exists() bool {
	return FileExists(t.target)
}

// Path string of the temp folder.
func (t *TempDirHandler) Path() string {
	return t.target
}
