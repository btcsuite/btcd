// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpctest

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	rpc "github.com/btcsuite/btcd/rpcclient"
)

// nodeConfig contains all the args, and data required to launch a btcd process
// and connect the rpc client to it.
type nodeConfig struct {
	rpcUser    string
	rpcPass    string
	listen     string
	rpcListen  string
	rpcConnect string
	dataDir    string
	logDir     string
	profile    string
	debugLevel string
	extra      []string
	nodeDir    string

	exe          string
	endpoint     string
	certFile     string
	keyFile      string
	certificates []byte
}

// newConfig returns a newConfig with all default values.
func newConfig(nodeDir, certFile, keyFile string, extra []string,
	customExePath string) (*nodeConfig, error) {

	var btcdPath string
	if customExePath != "" {
		btcdPath = customExePath
	} else {
		var err error
		btcdPath, err = btcdExecutablePath()
		if err != nil {
			btcdPath = "btcd"
		}
	}

	a := &nodeConfig{
		listen:    "127.0.0.1:18555",
		rpcListen: "127.0.0.1:18556",
		rpcUser:   "user",
		rpcPass:   "pass",
		extra:     extra,
		nodeDir:   nodeDir,
		exe:       btcdPath,
		endpoint:  "ws",
		certFile:  certFile,
		keyFile:   keyFile,
	}
	if err := a.setDefaults(); err != nil {
		return nil, err
	}
	return a, nil
}

// setDefaults sets the default values of the config. It also creates the
// temporary data, and log directories which must be cleaned up with a call to
// cleanup().
func (n *nodeConfig) setDefaults() error {
	n.dataDir = filepath.Join(n.nodeDir, "data")
	n.logDir = filepath.Join(n.nodeDir, "logs")
	cert, err := os.ReadFile(n.certFile)
	if err != nil {
		return err
	}
	n.certificates = cert
	return nil
}

// arguments returns an array of arguments that be used to launch the btcd
// process.
func (n *nodeConfig) arguments() []string {
	args := []string{}
	if n.rpcUser != "" {
		// --rpcuser
		args = append(args, fmt.Sprintf("--rpcuser=%s", n.rpcUser))
	}
	if n.rpcPass != "" {
		// --rpcpass
		args = append(args, fmt.Sprintf("--rpcpass=%s", n.rpcPass))
	}
	if n.listen != "" {
		// --listen
		args = append(args, fmt.Sprintf("--listen=%s", n.listen))
	}
	if n.rpcListen != "" {
		// --rpclisten
		args = append(args, fmt.Sprintf("--rpclisten=%s", n.rpcListen))
	}
	if n.rpcConnect != "" {
		// --rpcconnect
		args = append(args, fmt.Sprintf("--rpcconnect=%s", n.rpcConnect))
	}
	// --rpccert
	args = append(args, fmt.Sprintf("--rpccert=%s", n.certFile))
	// --rpckey
	args = append(args, fmt.Sprintf("--rpckey=%s", n.keyFile))
	if n.dataDir != "" {
		// --datadir
		args = append(args, fmt.Sprintf("--datadir=%s", n.dataDir))
	}
	if n.logDir != "" {
		// --logdir
		args = append(args, fmt.Sprintf("--logdir=%s", n.logDir))
	}
	if n.profile != "" {
		// --profile
		args = append(args, fmt.Sprintf("--profile=%s", n.profile))
	}
	if n.debugLevel != "" {
		// --debuglevel
		args = append(args, fmt.Sprintf("--debuglevel=%s", n.debugLevel))
	}
	args = append(args, n.extra...)
	return args
}

// command returns the exec.Cmd which will be used to start the btcd process.
func (n *nodeConfig) command() *exec.Cmd {
	return exec.Command(n.exe, n.arguments()...)
}

// rpcConnConfig returns the rpc connection config that can be used to connect
// to the btcd process that is launched via Start().
func (n *nodeConfig) rpcConnConfig() rpc.ConnConfig {
	return rpc.ConnConfig{
		Host:                 n.rpcListen,
		Endpoint:             n.endpoint,
		User:                 n.rpcUser,
		Pass:                 n.rpcPass,
		Certificates:         n.certificates,
		DisableAutoReconnect: true,
	}
}

// String returns the string representation of this nodeConfig.
func (n *nodeConfig) String() string {
	return n.nodeDir
}

// node houses the necessary state required to configure, launch, and manage a
// btcd process.
type node struct {
	config *nodeConfig

	cmd     *exec.Cmd
	pidFile string

	dataDir string
}

// newNode creates a new node instance according to the passed config. dataDir
// will be used to hold a file recording the pid of the launched process, and
// as the base for the log and data directories for btcd.
func newNode(config *nodeConfig, dataDir string) (*node, error) {
	return &node{
		config:  config,
		dataDir: dataDir,
		cmd:     config.command(),
	}, nil
}

// start creates a new btcd process, and writes its pid in a file reserved for
// recording the pid of the launched process. This file can be used to
// terminate the process in case of a hang, or panic. In the case of a failing
// test case, or panic, it is important that the process be stopped via stop(),
// otherwise, it will persist unless explicitly killed.
func (n *node) start() error {
	if err := n.cmd.Start(); err != nil {
		return err
	}

	pid, err := os.Create(filepath.Join(n.dataDir, "btcd.pid"))
	if err != nil {
		return err
	}

	n.pidFile = pid.Name()
	if _, err = fmt.Fprintf(pid, "%d\n", n.cmd.Process.Pid); err != nil {
		return err
	}

	if err := pid.Close(); err != nil {
		return err
	}

	return nil
}

// stop interrupts the running btcd process process, and waits until it exits
// properly. On windows, interrupt is not supported, so a kill signal is used
// instead
func (n *node) stop() error {
	if n.cmd == nil || n.cmd.Process == nil {
		// return if not properly initialized
		// or error starting the process
		return nil
	}
	defer n.cmd.Wait()
	if runtime.GOOS == "windows" {
		return n.cmd.Process.Signal(os.Kill)
	}
	return n.cmd.Process.Signal(os.Interrupt)
}

// cleanup cleanups process and args files. The file housing the pid of the
// created process will be deleted, as well as any directories created by the
// process.
func (n *node) cleanup() error {
	if n.pidFile != "" {
		if err := os.Remove(n.pidFile); err != nil {
			log.Printf("unable to remove file %s: %v", n.pidFile,
				err)
		}
	}

	// Since the node's main data directory is passed in to the node config,
	// it isn't our responsibility to clean it up. So we're done after
	// removing the pid file.
	return nil
}

// shutdown terminates the running btcd process, and cleans up all
// file/directories created by node.
func (n *node) shutdown() error {
	if err := n.stop(); err != nil {
		return err
	}
	if err := n.cleanup(); err != nil {
		return err
	}
	return nil
}

// genCertPair generates a key/cert pair to the paths provided.
func genCertPair(certFile, keyFile string) error {
	org := "rpctest autogenerated cert"
	validUntil := time.Now().Add(10 * 365 * 24 * time.Hour)
	cert, key, err := btcutil.NewTLSCertPair(org, validUntil, nil)
	if err != nil {
		return err
	}

	// Write cert and key files.
	if err = os.WriteFile(certFile, cert, 0666); err != nil {
		return err
	}
	if err = os.WriteFile(keyFile, key, 0600); err != nil {
		_ = os.Remove(certFile)
		return err
	}

	return nil
}
