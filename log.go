// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/conformal/btcd/addrmgr"

	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	"github.com/conformal/btclog"
	"github.com/conformal/btcscript"
	"github.com/conformal/btcwire"
	"github.com/conformal/seelog"
)

const (
	// lockTimeThreshold is the number below which a lock time is
	// interpreted to be a block number.  Since an average of one block
	// is generated per 10 minutes, this allows blocks for about 9,512
	// years.  However, if the field is interpreted as a timestamp, given
	// the lock time is a uint32, the max is sometime around 2106.
	lockTimeThreshold uint32 = 5e8 // Tue Nov 5 00:53:20 1985 UTC

	// maxRejectReasonLen is the maximum length of a sanitized reject reason
	// that will be logged.
	maxRejectReasonLen = 200
)

// Loggers per subsytem.  Note that backendLog is a seelog logger that all of
// the subsystem loggers route their messages to.  When adding new subsystems,
// add a reference here, to the subsystemLoggers map, and the useLogger
// function.
var (
	backendLog = seelog.Disabled
	amgrLog    = btclog.Disabled
	bcdbLog    = btclog.Disabled
	bmgrLog    = btclog.Disabled
	btcdLog    = btclog.Disabled
	chanLog    = btclog.Disabled
	discLog    = btclog.Disabled
	minrLog    = btclog.Disabled
	peerLog    = btclog.Disabled
	rpcsLog    = btclog.Disabled
	scrpLog    = btclog.Disabled
	srvrLog    = btclog.Disabled
	txmpLog    = btclog.Disabled
)

// subsystemLoggers maps each subsystem identifier to its associated logger.
var subsystemLoggers = map[string]btclog.Logger{
	"AMGR": amgrLog,
	"BCDB": bcdbLog,
	"BMGR": bmgrLog,
	"BTCD": btcdLog,
	"CHAN": chanLog,
	"DISC": discLog,
	"MINR": minrLog,
	"PEER": peerLog,
	"RPCS": rpcsLog,
	"SCRP": scrpLog,
	"SRVR": srvrLog,
	"TXMP": txmpLog,
}

// logClosure is used to provide a closure over expensive logging operations
// so don't have to be performed when the logging level doesn't warrant it.
type logClosure func() string

// String invokes the underlying function and returns the result.
func (c logClosure) String() string {
	return c()
}

// newLogClosure returns a new closure over a function that returns a string
// which itself provides a Stringer interface so that it can be used with the
// logging system.
func newLogClosure(c func() string) logClosure {
	return logClosure(c)
}

// useLogger updates the logger references for subsystemID to logger.  Invalid
// subsystems are ignored.
func useLogger(subsystemID string, logger btclog.Logger) {
	if _, ok := subsystemLoggers[subsystemID]; !ok {
		return
	}
	subsystemLoggers[subsystemID] = logger

	switch subsystemID {
	case "AMGR":
		amgrLog = logger
		addrmgr.UseLogger(logger)

	case "BCDB":
		bcdbLog = logger
		btcdb.UseLogger(logger)

	case "BMGR":
		bmgrLog = logger

	case "BTCD":
		btcdLog = logger

	case "CHAN":
		chanLog = logger
		btcchain.UseLogger(logger)

	case "DISC":
		discLog = logger

	case "MINR":
		minrLog = logger

	case "PEER":
		peerLog = logger

	case "RPCS":
		rpcsLog = logger

	case "SCRP":
		scrpLog = logger
		btcscript.UseLogger(logger)

	case "SRVR":
		srvrLog = logger

	case "TXMP":
		txmpLog = logger
	}
}

// initSeelogLogger initializes a new seelog logger that is used as the backend
// for all logging subsytems.
func initSeelogLogger(logFile string) {
	config := `
	<seelog type="adaptive" mininterval="2000000" maxinterval="100000000"
		critmsgcount="500" minlevel="trace">
		<outputs formatid="all">
			<console />
			<rollingfile type="size" filename="%s" maxsize="10485760" maxrolls="3" />
		</outputs>
		<formats>
			<format id="all" format="%%Time %%Date [%%LEV] %%Msg%%n" />
		</formats>
	</seelog>`
	config = fmt.Sprintf(config, logFile)

	logger, err := seelog.LoggerFromConfigAsString(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v", err)
		os.Exit(1)
	}

	backendLog = logger
}

// setLogLevel sets the logging level for provided subsystem.  Invalid
// subsystems are ignored.  Uninitialized subsystems are dynamically created as
// needed.
func setLogLevel(subsystemID string, logLevel string) {
	// Ignore invalid subsystems.
	logger, ok := subsystemLoggers[subsystemID]
	if !ok {
		return
	}

	// Default to info if the log level is invalid.
	level, ok := btclog.LogLevelFromString(logLevel)
	if !ok {
		level = btclog.InfoLvl
	}

	// Create new logger for the subsystem if needed.
	if logger == btclog.Disabled {
		logger = btclog.NewSubsystemLogger(backendLog, subsystemID+": ")
		useLogger(subsystemID, logger)
	}
	logger.SetLevel(level)
}

// setLogLevels sets the log level for all subsystem loggers to the passed
// level.  It also dynamically creates the subsystem loggers as needed, so it
// can be used to initialize the logging system.
func setLogLevels(logLevel string) {
	// Configure all sub-systems with the new logging level.  Dynamically
	// create loggers as needed.
	for subsystemID := range subsystemLoggers {
		setLogLevel(subsystemID, logLevel)
	}
}

// directionString is a helper function that returns a string that represents
// the direction of a connection (inbound or outbound).
func directionString(inbound bool) string {
	if inbound {
		return "inbound"
	}
	return "outbound"
}

// formatLockTime returns a transaction lock time as a human-readable string.
func formatLockTime(lockTime uint32) string {
	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the lockTimeThreshold.  When it is under the
	// threshold it is a block height.
	if lockTime < lockTimeThreshold {
		return fmt.Sprintf("height %d", lockTime)
	}

	return time.Unix(int64(lockTime), 0).String()
}

// invSummary returns an inventory message as a human-readable string.
func invSummary(invList []*btcwire.InvVect) string {
	// No inventory.
	invLen := len(invList)
	if invLen == 0 {
		return "empty"
	}

	// One inventory item.
	if invLen == 1 {
		iv := invList[0]
		switch iv.Type {
		case btcwire.InvTypeError:
			return fmt.Sprintf("error %s", iv.Hash)
		case btcwire.InvTypeBlock:
			return fmt.Sprintf("block %s", iv.Hash)
		case btcwire.InvTypeTx:
			return fmt.Sprintf("tx %s", iv.Hash)
		}

		return fmt.Sprintf("unknown (%d) %s", uint32(iv.Type), iv.Hash)
	}

	// More than one inv item.
	return fmt.Sprintf("size %d", invLen)
}

// locatorSummary returns a block locator as a human-readable string.
func locatorSummary(locator []*btcwire.ShaHash, stopHash *btcwire.ShaHash) string {
	if len(locator) > 0 {
		return fmt.Sprintf("locator %s, stop %s", locator[0], stopHash)
	}

	return fmt.Sprintf("no locator, stop %s", stopHash)

}

// sanitizeString strips any characters which are even remotely dangerous, such
// as html control characters, from the passed string.  It also limits it to
// the passed maximum size, which can be 0 for unlimited.  When the string is
// limited, it will also add "..." to the string to indicate it was truncated.
func sanitizeString(str string, maxLength uint) string {
	const safeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXY" +
		"Z01234567890 .,;_/:?@"

	// Strip any characters not in the safeChars string removed.
	str = strings.Map(func(r rune) rune {
		if strings.IndexRune(safeChars, r) >= 0 {
			return r
		}
		return -1
	}, str)

	// Limit the string to the max allowed length.
	if maxLength > 0 && uint(len(str)) > maxLength {
		str = str[:maxLength]
		str = str + "..."
	}
	return str
}

// messageSummary returns a human-readable string which summarizes a message.
// Not all messages have or need a summary.  This is used for debug logging.
func messageSummary(msg btcwire.Message) string {
	switch msg := msg.(type) {
	case *btcwire.MsgVersion:
		return fmt.Sprintf("agent %s, pver %d, block %d",
			msg.UserAgent, msg.ProtocolVersion, msg.LastBlock)

	case *btcwire.MsgVerAck:
		// No summary.

	case *btcwire.MsgGetAddr:
		// No summary.

	case *btcwire.MsgAddr:
		return fmt.Sprintf("%d addr", len(msg.AddrList))

	case *btcwire.MsgPing:
		// No summary - perhaps add nonce.

	case *btcwire.MsgPong:
		// No summary - perhaps add nonce.

	case *btcwire.MsgAlert:
		// No summary.

	case *btcwire.MsgMemPool:
		// No summary.

	case *btcwire.MsgTx:
		hash, _ := msg.TxSha()
		return fmt.Sprintf("hash %s, %d inputs, %d outputs, lock %s",
			hash, len(msg.TxIn), len(msg.TxOut),
			formatLockTime(msg.LockTime))

	case *btcwire.MsgBlock:
		header := &msg.Header
		hash, _ := msg.BlockSha()
		return fmt.Sprintf("hash %s, ver %d, %d tx, %s", hash,
			header.Version, len(msg.Transactions), header.Timestamp)

	case *btcwire.MsgInv:
		return invSummary(msg.InvList)

	case *btcwire.MsgNotFound:
		return invSummary(msg.InvList)

	case *btcwire.MsgGetData:
		return invSummary(msg.InvList)

	case *btcwire.MsgGetBlocks:
		return locatorSummary(msg.BlockLocatorHashes, &msg.HashStop)

	case *btcwire.MsgGetHeaders:
		return locatorSummary(msg.BlockLocatorHashes, &msg.HashStop)

	case *btcwire.MsgHeaders:
		return fmt.Sprintf("num %d", len(msg.Headers))

	case *btcwire.MsgReject:
		// Ensure the variable length strings don't contain any
		// characters which are even remotely dangerous such as HTML
		// control characters, etc.  Also limit them to sane length for
		// logging.
		rejCommand := sanitizeString(msg.Cmd, btcwire.CommandSize)
		rejReason := sanitizeString(msg.Reason, maxRejectReasonLen)
		summary := fmt.Sprintf("cmd %v, code %v, reason %v", rejCommand,
			msg.Code, rejReason)
		if rejCommand == btcwire.CmdBlock || rejCommand == btcwire.CmdTx {
			summary += fmt.Sprintf(", hash %v", msg.Hash)
		}
		return summary
	}

	// No summary for other messages.
	return ""
}
