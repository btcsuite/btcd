// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"os"
	"path/filepath"
)

const addrIndexBuildDirName = "addrindexbuild"

// createAddrBuildStager creates the staging directory for a new build.  It
// creates the staging files and the initial manifest in staging.tmp and then
// renames it to staging, so a staging directory always holds a manifest.
func createAddrBuildStager(dataDir string,
	manifest *addrBuildManifest) (*addrStager, error) {

	buildDir := filepath.Join(dataDir, addrIndexBuildDirName)
	if err := os.MkdirAll(buildDir, 0700); err != nil {
		return nil, err
	}
	if err := syncAddrBuildDir(filepath.Dir(buildDir)); err != nil {
		return nil, err
	}

	stagingDir := filepath.Join(buildDir, "staging")
	tmpDir := stagingDir + ".tmp"
	if err := os.Mkdir(tmpDir, 0700); err != nil {
		return nil, err
	}
	stager, err := newAddrStager(tmpDir)
	if err != nil {
		return nil, err
	}
	if err := stager.checkpoint(manifest, nil); err != nil {
		stager.close()
		return nil, err
	}
	stager.close()

	if err := os.Rename(tmpDir, stagingDir); err != nil {
		return nil, err
	}
	if err := syncAddrBuildDir(buildDir); err != nil {
		return nil, err
	}
	stager, err = openAddrStager(stagingDir, manifest, nil)
	if err != nil {
		return nil, err
	}
	return stager, nil
}

// removeAddrIndexBuildDeleteDir removes addrindexbuild.delete, which an
// interrupted removeAddrIndexBuildDir can leave behind, and syncs the data
// directory.  It does nothing when addrindexbuild.delete does not exist.
func removeAddrIndexBuildDeleteDir(dataDir string) error {
	deleteDir := filepath.Join(dataDir, addrIndexBuildDirName) + ".delete"
	if _, err := os.Stat(deleteDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.RemoveAll(deleteDir); err != nil {
		return err
	}
	return syncAddrBuildDir(dataDir)
}

// removeAddrIndexBuildDir removes the build directory.  It first renames the
// directory to addrindexbuild.delete, which the next start removes, so a crash
// during the removal cannot leave a staging directory with some of its files
// gone.
func removeAddrIndexBuildDir(dataDir string) error {
	buildDir := filepath.Join(dataDir, addrIndexBuildDirName)
	deleteDir := buildDir + ".delete"
	if err := removeAddrIndexBuildDeleteDir(dataDir); err != nil {
		return err
	}
	if err := os.Rename(buildDir, deleteDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else if err := syncAddrBuildDir(dataDir); err != nil {
		return err
	}
	if err := os.RemoveAll(deleteDir); err != nil {
		return err
	}
	return syncAddrBuildDir(dataDir)
}

// addrIndexBuildDirExists returns the path of the build directory under
// dataDir and whether it exists.
func addrIndexBuildDirExists(dataDir string) (string, bool, error) {
	dir := filepath.Join(dataDir, addrIndexBuildDirName)
	_, err := os.Stat(dir)
	if err == nil {
		return dir, true, nil
	}
	if os.IsNotExist(err) {
		return dir, false, nil
	}
	return dir, false, err
}
