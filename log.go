// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain

import (
	"errors"
	"github.com/conformal/seelog"
	"io"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
var log seelog.LoggerInterface

// The default amount of logging is none.
func init() {
	DisableLog()
}

// DisableLog disables all library log output.  Logging output is disabled
// by default until either UseLogger or SetLogWriter are called.
func DisableLog() {
	log = seelog.Disabled
}

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using seelog.
func UseLogger(logger seelog.LoggerInterface) {
	log = logger
}

// SetLogWriter uses a specified io.Writer to output package logging info.
// This allows a caller to direct package logging output without needing a
// dependency on seelog.  If the caller is also using seelog, UseLogger should
// be used instead.
func SetLogWriter(w io.Writer) error {
	if w == nil {
		return errors.New("nil writer")
	}

	l, err := seelog.LoggerFromWriterWithMinLevel(w, seelog.TraceLvl)
	if err != nil {
		return err
	}

	UseLogger(l)
	return nil
}

// LogClosure is a closure that can be printed with %v to be used to
// generate expensive-to-create data for a detailed log level and avoid doing
// the work if the data isn't printed.
type logClosure func() string

func (c logClosure) String() string {
	return c()
}

func newLogClosure(c func() string) logClosure {
	return logClosure(c)
}
