// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpcclient

import (
	"errors"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/stretchr/testify/require"
)

func TestGenerateBlockResult(t *testing.T) {
	tests := []struct {
		name   string
		result string
		want   btcjson.GenerateBlockResult
	}{
		{"submitted", `{"hash":"abcd"}`, btcjson.GenerateBlockResult{Hash: "abcd"}},
		{"unsubmitted", `{"hash":"abcd","hex":"0123"}`,
			btcjson.GenerateBlockResult{Hash: "abcd", Hex: "0123"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			future := make(FutureGenerateBlockResult, 1)
			future <- &Response{result: []byte(test.result)}
			got, err := future.Receive()
			require.NoError(t, err)
			require.Equal(t, &test.want, got)
		})
	}

	future := make(FutureGenerateBlockResult, 1)
	future <- &Response{result: []byte(`{"hash":3}`)}
	_, err := future.Receive()
	require.Error(t, err)

	rpcErr := errors.New("transaction not in mempool")
	future <- &Response{err: rpcErr}
	_, err = future.Receive()
	require.ErrorIs(t, err, rpcErr)
}
