// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb

import (
	"errors"
	"io"

	"github.com/conformal/btclog"
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

// SetLogWriter uses a specified io.Writer to output package logging info.
// This allows a caller to direct package logging output without needing a
// dependency on seelog.  If the caller is also using btclog, UseLogger should
// be used instead.
func SetLogWriter(w io.Writer, level string) error {
	if w == nil {
		return errors.New("nil writer")
	}

	lvl, ok := btclog.LogLevelFromString(level)
	if !ok {
		return errors.New("invalid log level")
	}

	l, err := btclog.NewLoggerFromWriter(w, lvl)
	if err != nil {
		return err
	}

	UseLogger(l)
	return nil
}

// GetLog returns the currently active logger.
func GetLog() btclog.Logger {
	return log
}
