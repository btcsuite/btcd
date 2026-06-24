package debugstream

import "github.com/btcsuite/btclog"

var strLog btclog.Logger
var cliLog btclog.Logger

func init() {
	DisableLog()
}

// DisableLog disables all library log output.  Logging output is disabled
// by default until either UseLogger or SetLogWriter are called.
func DisableLog() {
	strLog, cliLog = btclog.Disabled, btclog.Disabled
}

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using btclog.
func UseLoggers(strLogger, cliLogger btclog.Logger) {
	strLog, cliLog = strLogger, cliLogger
}
