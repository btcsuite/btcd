// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/addrmgr"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/blockchain/indexers"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/mining/cpuminer"
	"github.com/btcsuite/btcd/netsync"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/txscript"

	"github.com/btcsuite/btclog"
	"github.com/jrick/logrotate/rotator"
)

// logWriter implements an io.Writer that outputs to both standard output and
// the write-end pipe of an initialized log rotator.
type logWriter struct{}

func (logWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	LogRotator.Write(p)
	return len(p), nil
}

// Loggers per subsystem.  A single backend logger is created and all subsystem
// loggers created from it will write to the backend.  When adding new
// subsystems, add the subsystem logger variable here and to the
// subsystemLoggers map.
//
// Loggers can not be used before the log rotator has been initialized with a
// log file.  This must be performed early during application startup by calling
// initLogRotator.
var (
	// backendLog is the logging backend used to create all subsystem loggers.
	// The backend must not be used before the log rotator has been initialized,
	// or data races and/or nil pointer dereferences will occur.
	backendLog = btclog.NewBackend(logWriter{})

	// LogRotator is one of the logging outputs.  It should be closed on
	// application shutdown.
	LogRotator *rotator.Rotator

	adxrLog = backendLog.Logger("ADXR")
	AmgrLog = backendLog.Logger("AMGR")
	cmgrLog = backendLog.Logger("CMGR")
	bcdbLog = backendLog.Logger("BCDB")
	BtcdLog = backendLog.Logger("BTCD")
	chanLog = backendLog.Logger("CHAN")
	discLog = backendLog.Logger("DISC")
	IndxLog = backendLog.Logger("INDX")
	minrLog = backendLog.Logger("MINR")
	PeerLog = backendLog.Logger("PEER")
	RpcsLog = backendLog.Logger("RPCS")
	scrpLog = backendLog.Logger("SCRP")
	SrvrLog = backendLog.Logger("SRVR")
	syncLog = backendLog.Logger("SYNC")
	TxmpLog = backendLog.Logger("TXMP")
)

// Initialize package-global logger variables.
func init() {
	addrmgr.UseLogger(AmgrLog)
	connmgr.UseLogger(cmgrLog)
	database.UseLogger(bcdbLog)
	blockchain.UseLogger(chanLog)
	indexers.UseLogger(IndxLog)
	mining.UseLogger(minrLog)
	cpuminer.UseLogger(minrLog)
	peer.UseLogger(PeerLog)
	txscript.UseLogger(scrpLog)
	netsync.UseLogger(syncLog)
	mempool.UseLogger(TxmpLog)
}

// SubsystemLoggers maps each subsystem identifier to its associated logger.
var SubsystemLoggers = map[string]btclog.Logger{
	"ADXR": adxrLog,
	"AMGR": AmgrLog,
	"CMGR": cmgrLog,
	"BCDB": bcdbLog,
	"BTCD": BtcdLog,
	"CHAN": chanLog,
	"DISC": discLog,
	"INDX": IndxLog,
	"MINR": minrLog,
	"PEER": PeerLog,
	"RPCS": RpcsLog,
	"SCRP": scrpLog,
	"SRVR": SrvrLog,
	"SYNC": syncLog,
	"TXMP": TxmpLog,
}

// InitLogRotator initializes the logging rotater to write logs to logFile and
// create roll files in the same directory.  It must be called before the
// package-global log rotater variables are used.
func InitLogRotator(logFile string) {
	logDir, _ := filepath.Split(logFile)
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log directory: %v\n", err)
		os.Exit(1)
	}
	r, err := rotator.New(logFile, 10*1024, false, 3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create file rotator: %v\n", err)
		os.Exit(1)
	}

	LogRotator = r
}

// SetLogLevel sets the logging level for provided subsystem.  Invalid
// subsystems are ignored.  Uninitialized subsystems are dynamically created as
// needed.
func SetLogLevel(subsystemID string, logLevel string) {
	// Ignore invalid subsystems.
	logger, ok := SubsystemLoggers[subsystemID]
	if !ok {
		return
	}

	// Defaults to info if the log level is invalid.
	level, _ := btclog.LevelFromString(logLevel)
	logger.SetLevel(level)
}

// SetLogLevels sets the log level for all subsystem loggers to the passed
// level.  It also dynamically creates the subsystem loggers as needed, so it
// can be used to initialize the logging system.
func SetLogLevels(logLevel string) {
	// Configure all sub-systems with the new logging level.  Dynamically
	// create loggers as needed.
	for subsystemID := range SubsystemLoggers {
		SetLogLevel(subsystemID, logLevel)
	}
}

// DirectionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func DirectionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// PickNoun returns the singular or plural form of a noun depending
// on the count n.
func PickNoun(n uint64, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
