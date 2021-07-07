package node

import (
	"sync"

	"github.com/btcsuite/btclog"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
var log btclog.Logger

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output.  Logging output is disabled
// by default until either UseLogger or SetLogWriter are called.
func DisableLog() {
	log = btclog.Disabled
}

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using btclog.
func UseLogger(logger btclog.Logger) {
	log = logger
}

var loggedStrings = map[string]bool{} // is this gonna get too large?
var loggedStringsMutex sync.Mutex

func LogOnce(s string) {
	loggedStringsMutex.Lock()
	defer loggedStringsMutex.Unlock()
	if loggedStrings[s] {
		return
	}
	loggedStrings[s] = true
	log.Info(s)
}

func Warn(s string) {
	log.Warn(s)
}
