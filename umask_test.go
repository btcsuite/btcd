// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jrick/logrotate/rotator"
	"github.com/stretchr/testify/require"
)

// fileModesDirEnv names the directory where the child process started by
// TestFileModes creates its files.
const fileModesDirEnv = "BTCD_TEST_FILE_MODES_DIR"

// TestFileModes checks that the log and the RPC key and certificate are
// created private to the user.  The files are created by a child process
// started under umask 022, so the result does not depend on the umask of
// the caller, and the umask of this process is left alone.
func TestFileModes(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip("btcd pledges at startup without proc/exec, so this test's subprocess re-exec cannot run under pledge on OpenBSD")
	}

	if dir := os.Getenv(fileModesDirEnv); dir != "" {
		// Create the log file the way initLogRotator does.
		r, err := rotator.New(filepath.Join(dir, "btcd.log"), 10*1024,
			false, 3)
		require.NoError(t, err)
		require.NoError(t, r.Close())

		err = genCertPair(filepath.Join(dir, "rpc.cert"),
			filepath.Join(dir, "rpc.key"))
		require.NoError(t, err)
		return
	}

	dir := t.TempDir()
	cmd := exec.Command("sh", "-c",
		`umask 022 && exec "$0" -test.run='^TestFileModes$'`, os.Args[0])
	cmd.Env = append(os.Environ(), fileModesDirEnv+"="+dir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	for _, name := range []string{"btcd.log", "rpc.key", "rpc.cert"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), fi.Mode().Perm(), name)
	}
}
