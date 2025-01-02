// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"io"
	"os"
	"path/filepath"
)

// dirEmpty returns whether or not the specified directory path is empty.
func dirEmpty(dirPath string) (bool, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Read the names of a max of one entry from the directory.  When the
	// directory is empty, an io.EOF error will be returned, so allow it.
	names, err := f.Readdirnames(1)
	if err != nil && err != io.EOF {
		return false, err
	}

	return len(names) == 0, nil
}

// oldBtcdHomeDir returns the OS specific home directory btcd used prior to
// version 0.3.3.  This has since been replaced with btcutil.AppDataDir, but
// this function is still provided for the automatic upgrade path.
func oldBtcdHomeDir() string {
	// Search for Windows APPDATA first.  This won't exist on POSIX OSes.
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "btcd")
	}

	// Fall back to standard HOME directory that works for most POSIX OSes.
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".btcd")
	}

	// In the worst case, use the current directory.
	return "."
}

// upgradeDBPathNet moves the database for a specific network from its
// location prior to btcd version 0.2.0 and uses heuristics to ascertain the old
// database type to rename to the new format.
func upgradeDBPathNet(oldDbPath, netName string) error {
	// Prior to version 0.2.0, the database was named the same thing for
	// both sqlite and leveldb.  Use heuristics to figure out the type
	// of the database and move it to the new path and name introduced with
	// version 0.2.0 accordingly.
	fi, err := os.Stat(oldDbPath)
	if err == nil {
		oldDbType := "sqlite"
		if fi.IsDir() {
			oldDbType = "leveldb"
		}

		// The new database name is based on the database type and
		// resides in a directory named after the network type.
		newDbRoot := filepath.Join(filepath.Dir(cfg.DataDir), netName)
		newDbName := blockDbNamePrefix + "_" + oldDbType
		if oldDbType == "sqlite" {
			newDbName = newDbName + ".db"
		}
		newDbPath := filepath.Join(newDbRoot, newDbName)

		// Create the new path if needed.
		err = os.MkdirAll(newDbRoot, 0700)
		if err != nil {
			return err
		}

		// Move and rename the old database.
		err := os.Rename(oldDbPath, newDbPath)
		if err != nil {
			return err
		}
	}

	return nil
}

// upgradeDBPaths moves the databases from their locations prior to btcd
// version 0.2.0 to their new locations.
func upgradeDBPaths() error {
	// Prior to version 0.2.0, the databases were in the "db" directory and
	// their names were suffixed by "testnet" and "regtest" for their
	// respective networks.  Check for the old database and update it to the
	// new path introduced with version 0.2.0 accordingly.
	oldDbRoot := filepath.Join(oldBtcdHomeDir(), "db")
	upgradeDBPathNet(filepath.Join(oldDbRoot, "btcd.db"), "mainnet")
	upgradeDBPathNet(filepath.Join(oldDbRoot, "btcd_testnet.db"), "testnet")
	upgradeDBPathNet(filepath.Join(oldDbRoot, "btcd_regtest.db"), "regtest")

	// Remove the old db directory.
	return os.RemoveAll(oldDbRoot)
}

// upgradeDataPaths moves the application data from its location prior to btcd
// version 0.3.3 to its new location.
func upgradeDataPaths() error {
	// No need to migrate if the old and new home paths are the same.
	oldHomePath := oldBtcdHomeDir()
	newHomePath := defaultHomeDir
	if oldHomePath == newHomePath {
		return nil
	}

	// Only migrate if the old path exists and the new one doesn't.
	if fileExists(oldHomePath) && !fileExists(newHomePath) {
		// Create the new path.
		btcdLog.Infof("Migrating application home path from '%s' to '%s'",
			oldHomePath, newHomePath)
		err := os.MkdirAll(newHomePath, 0700)
		if err != nil {
			return err
		}

		// Move old btcd.conf into new location if needed.
		oldConfPath := filepath.Join(oldHomePath, defaultConfigFilename)
		newConfPath := filepath.Join(newHomePath, defaultConfigFilename)
		if fileExists(oldConfPath) && !fileExists(newConfPath) {
			err := os.Rename(oldConfPath, newConfPath)
			if err != nil {
				return err
			}
		}

		// Move old data directory into new location if needed.
		oldDataPath := filepath.Join(oldHomePath, defaultDataDirname)
		newDataPath := filepath.Join(newHomePath, defaultDataDirname)
		if fileExists(oldDataPath) && !fileExists(newDataPath) {
			err := os.Rename(oldDataPath, newDataPath)
			if err != nil {
				return err
			}
		}

		// Remove the old home if it is empty or show a warning if not.
		ohpEmpty, err := dirEmpty(oldHomePath)
		if err != nil {
			return err
		}
		if ohpEmpty {
			err := os.Remove(oldHomePath)
			if err != nil {
				return err
			}
		} else {
			btcdLog.Warnf("Not removing '%s' since it contains files "+
				"not created by this application.  You may "+
				"want to manually move them or delete them.",
				oldHomePath)
		}
	}

	return nil
}

// doUpgrades performs upgrades to btcd as new versions require it.
func doUpgrades() error {
	err := upgradeDBPaths()
	if err != nil {
		return err
	}
	return upgradeDataPaths()
}
Edit

Preview

Show Diff
Loading preview…There are no changes to show. But you can preview the whole file.
btcd's Reproducible Build System
This package contains the build script that the btcd project uses in order to build binaries for each new release. As of go1.13, with some new build flags, binaries are now reproducible, allowing developers to build the binary on distinct machines, and end up with a byte-for-byte identical binary. Every release should note which Go version was used to build the release, so that version should be used for verifying the release.

Building a New Release
Tagging and pushing a new tag (for maintainers)
Before running release scripts, a few things need to happen in order to finally create a release and make sure there are no mistakes in the release process.

First, make sure that before the tagged commit there are modifications to the CHANGES file committed. The CHANGES file should be a changelog that roughly mirrors the release notes. Generally, the PRs that have been merged since the last release have been listed in the CHANGES file and categorized. For example, these changes have had the following format in the past:

Changes in X.YY.Z (Month Day Year):
  - Protocol and Network-related changes:
    - PR Title One (#PRNUM)
    - PR Title Two (#PRNUMTWO)
    ...
  - RPC changes:
  - Crypto changes:
  ...

  - Contributors (alphabetical order):
    - Contributor A
    - Contributor B
    - Contributor C
    ...
If the previous tag is, for example, vA.B.C, then you can get the list of contributors (from vA.B.C until the current HEAD) using the following command:

git log vA.B.C..HEAD --pretty="%an" | sort | uniq
After committing changes to the CHANGES file, the tagged release commit should be created.

The tagged commit should be a commit that bumps version numbers in version.go and cmd/btcctl/version.go. For example (taken from f3ec130):

diff --git a/cmd/btcctl/version.go b/cmd/btcctl/version.go
index 2195175c71..f65cacef7e 100644
--- a/cmd/btcctl/version.go
+++ b/cmd/btcctl/version.go
@@ -18,7 +18,7 @@ const semanticAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqr
 const (
 	appMajor uint = 0
 	appMinor uint = 20
-	appPatch uint = 0
+	appPatch uint = 1
 
 	// appPreRelease MUST only contain characters from semanticAlphabet
 	// per the semantic versioning spec.
diff --git a/version.go b/version.go
index 92fd60fdd4..fba55b5a37 100644
--- a/version.go
+++ b/version.go
@@ -18,7 +18,7 @@ const semanticAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqr
 const (
 	appMajor uint = 0
 	appMinor uint = 20
-	appPatch uint = 0
+	appPatch uint = 1
 
 	// appPreRelease MUST only contain characters from semanticAlphabet
 	// per the semantic

