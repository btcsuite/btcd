// Copyright (c) 2015 The btcsuite developers Use of this source code is
// governed by an ISC license that can be found in the LICENSE file.

package peer_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/btcsuite/btcd/peer"
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
			w:        bytes.NewBuffer(nil),
			level:    "wrong",
			expected: errors.New("invalid log level"),
		},
		{
			name:     "use off level",
			w:        bytes.NewBuffer(nil),
			level:    "off",
			expected: errors.New("min level can't be greater than max. Got min: 6, max: 5"),
		},
		{
			name:     "pass",
			w:        bytes.NewBuffer(nil),
			level:    "debug",
			expected: nil,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		err := peer.SetLogWriter(test.w, test.level)
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
