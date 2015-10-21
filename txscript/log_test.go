// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/btcsuite/btcd/txscript"
)

func TestSetLogWriter(t *testing.T) {
	tests := []struct {
		name     string
		w        io.Writer
		level    string
		expected error
	}{
		{
			name:     "nil writer",
			w:        nil,
			level:    "trace",
			expected: errors.New("nil writer"),
		},
		{
			name:     "invalid log level",
			w:        os.Stdout,
			level:    "wrong",
			expected: errors.New("invalid log level"),
		},
		{
			name:     "use off level",
			w:        os.Stdout,
			level:    "off",
			expected: errors.New("min level can't be greater than max. Got min: 6, max: 5"),
		},
		{
			name:     "pass",
			w:        os.Stdout,
			level:    "debug",
			expected: nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		err := txscript.SetLogWriter(test.w, test.level)
		if err != nil {
			if err.Error() != test.expected.Error() {
				t.Errorf("SetLogWriter #%d (%s) wrong result\n"+
					"got: %v\nwant: %v", i, test.name, err,
					test.expected)
			}
		} else {
			if test.expected != nil {
				t.Errorf("SetLogWriter #%d (%s) wrong result\n"+
					"got: %v\nwant: %v", i, test.name, err,
					test.expected)
			}
		}
	}
}
