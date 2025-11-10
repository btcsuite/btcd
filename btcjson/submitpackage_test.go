// Copyright (c) 2025 The btcsuite developers.
// Use of this source code is governed by an ISC license that can be found in
// the LICENSE file.

package btcjson_test

import (
	"encoding/json"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// TestSubmitPackageCmd tests all of the submit package commands marshal and
// unmarshal into valid results include handling of optional fields being
// omitted in the marshalled command, while optional fields with defaults have
// the default assigned on unmarshalled commands.
func TestSubmitPackageCmd(t *testing.T) {
	t.Parallel()

	const testID = 1
	tests := []struct {
		name         string
		newCmd       func() (interface{}, error)
		staticCmd    func() interface{}
		marshalled   string
		unmarshalled interface{}
	}{
		{
			name: "submitpackage minimal",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"submitpackage",
					[]string{"hex1", "hex2"},
				)
			},
			staticCmd: func() interface{} {
				return btcjson.NewJsonSubmitPackageCmd(
					[]string{"hex1", "hex2"}, nil, nil,
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitpackage","params":[["hex1","hex2"]],"id":1}`,
			unmarshalled: &btcjson.JsonSubmitPackageCmd{
				RawTxs:        []string{"hex1", "hex2"},
				MaxFeeRate:    nil,
				MaxBurnAmount: nil,
			},
		},
		{
			name: "submitpackage with maxfeerate",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"submitpackage",
					[]string{"hex1", "hex2"}, 0.1,
				)
			},
			staticCmd: func() interface{} {
				maxFeeRate := 0.1
				return btcjson.NewJsonSubmitPackageCmd(
					[]string{"hex1", "hex2"},
					&maxFeeRate, nil,
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitpackage","params":[["hex1","hex2"],0.1],"id":1}`,
			unmarshalled: &btcjson.JsonSubmitPackageCmd{
				RawTxs:        []string{"hex1", "hex2"},
				MaxFeeRate:    btcjson.Float64(0.1),
				MaxBurnAmount: nil,
			},
		},
		{
			name: "submitpackage with all optional params",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"submitpackage",
					[]string{"hex1", "hex2", "hex3"},
					0.25, 0.001,
				)
			},
			staticCmd: func() interface{} {
				maxFeeRate := 0.25
				maxBurnAmount := 0.001
				return btcjson.NewJsonSubmitPackageCmd(
					[]string{"hex1", "hex2", "hex3"},
					&maxFeeRate, &maxBurnAmount,
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitpackage","params":[["hex1","hex2","hex3"],0.25,0.001],"id":1}`,
			unmarshalled: &btcjson.JsonSubmitPackageCmd{
				RawTxs:        []string{"hex1", "hex2", "hex3"},
				MaxFeeRate:    btcjson.Float64(0.25),
				MaxBurnAmount: btcjson.Float64(0.001),
			},
		},
		{
			name: "submitpackage single tx",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"submitpackage",
					[]string{"hex1"},
				)
			},
			staticCmd: func() interface{} {
				return btcjson.NewJsonSubmitPackageCmd(
					[]string{"hex1"}, nil, nil,
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitpackage","params":[["hex1"]],"id":1}`,
			unmarshalled: &btcjson.JsonSubmitPackageCmd{
				RawTxs:        []string{"hex1"},
				MaxFeeRate:    nil,
				MaxBurnAmount: nil,
			},
		},
		{
			name: "submitpackage with zero maxfeerate",
			newCmd: func() (interface{}, error) {
				return btcjson.NewCmd(
					"submitpackage",
					[]string{"hex1", "hex2"},
					0.0,
				)
			},
			staticCmd: func() interface{} {
				maxFeeRate := 0.0
				return btcjson.NewJsonSubmitPackageCmd(
					[]string{"hex1", "hex2"},
					&maxFeeRate, nil,
				)
			},
			marshalled: `{"jsonrpc":"1.0","method":"submitpackage","params":[["hex1","hex2"],0],"id":1}`,
			unmarshalled: &btcjson.JsonSubmitPackageCmd{
				RawTxs:        []string{"hex1", "hex2"},
				MaxFeeRate:    btcjson.Float64(0.0),
				MaxBurnAmount: nil,
			},
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Marshal the command as created by the new static command
		// creation function.
		marshalled, err := btcjson.MarshalCmd(
			btcjson.RpcVersion1, testID, test.staticCmd(),
		)
		require.NoError(
			t, err, "MarshalCmd #%d (%s) unexpected in test", i,
			test.name,
		)

		require.Equal(
			t, test.marshalled, string(marshalled),
			"Test #%d (%s) unexpected marshalled data",
			i, test.name,
		)

		// Ensure the command is created without error via the generic
		// new command creation function.
		cmd, err := test.newCmd()
		require.NoError(
			t, err, "NewCmd #%d (%s) unexpected error in test",
			i, test.name,
		)

		// Marshal the command as created by the generic new command
		// creation function.
		marshalled, err = btcjson.MarshalCmd(
			btcjson.RpcVersion1, testID, cmd,
		)
		require.NoError(
			t, err, "MarshalCmd #%d (%s) unexpected in test", i,
			test.name,
		)

		// Ensure the marshalled data matches the expected value.
		require.Equal(
			t, test.marshalled, string(marshalled),
			"Test #%d (%s) unexpected marshalled data",
			i, test.name,
		)

		var request btcjson.Request
		err = json.Unmarshal(marshalled, &request)
		require.NoError(
			t, err,
			"UnmarshalCmd #%d (%s) unexpected error in test", i,
			test.name,
		)

		cmd, err = btcjson.UnmarshalCmd(&request)
		require.NoError(
			t, err,
			"UnmarshalCmd #%d (%s) unexpected error in test", i,
			test.name,
		)

		require.Equal(
			t, test.unmarshalled, cmd,
			"Test #%d (%s) unexpected unmarshalled command",
			i, test.name,
		)
	}
}

// TestSubmitPackageResultUnmarshalJSON tests the UnmarshalJSON method of
// SubmitPackageResult to ensure it properly converts from JSON format to the
// higher-level Go types.
func TestSubmitPackageResultUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		json     string
		expected btcjson.SubmitPackageResult
		wantErr  bool
	}{
		{
			name: "successful package",
			json: `{
				"package_msg": "success",
				"tx-results": {
					"wtxid1": {
						"txid": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
						"vsize": 150,
						"fees": {
							"base": 0.00001000,
							"effective-feerate": 0.00010000,
							"effective-includes": ["wtxid1", "wtxid2"]
						}
					},
					"wtxid2": {
						"txid": "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
						"vsize": 200,
						"fees": {
							"base": 0.00002000
						}
					}
				},
				"replaced-transactions": ["abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"]
			}`,
			expected: btcjson.SubmitPackageResult{
				PackageMsg: "success",
				TxResults: map[string]btcjson.SubmitPackageTxResult{
					"wtxid1": {
						TxID: *mustParseHash("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
					},
					"wtxid2": {
						TxID: *mustParseHash("fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"),
					},
				},
				ReplacedTransactions: []chainhash.Hash{
					*mustParseHash("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"),
				},
			},
		},
		{
			name: "package with errors",
			json: `{
				"package_msg": "transaction failed",
				"tx-results": {
					"wtxid1": {
						"txid": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
						"vsize": 150,
						"fees": {
							"base": 0.00001000
						},
						"error": "insufficient fee"
					}
				}
			}`,
			expected: btcjson.SubmitPackageResult{
				PackageMsg: "transaction failed",
				TxResults: map[string]btcjson.SubmitPackageTxResult{
					"wtxid1": {
						TxID:  *mustParseHash("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
						Error: btcjson.String("insufficient fee"),
					},
				},
			},
		},
		{
			name: "package with other-wtxid",
			json: `{
				"package_msg": "already in mempool",
				"tx-results": {
					"wtxid1": {
						"txid": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
						"other-wtxid": "fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
						"vsize": 150,
						"fees": {
							"base": 0.00001000
						}
					}
				}
			}`,
			expected: btcjson.SubmitPackageResult{
				PackageMsg: "already in mempool",
				TxResults: map[string]btcjson.SubmitPackageTxResult{
					"wtxid1": {
						TxID:       *mustParseHash("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
						OtherWtxid: mustParseHash("fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"),
					},
				},
			},
		},
		{
			name: "minimal response",
			json: `{
				"package_msg": "success",
				"tx-results": {}
			}`,
			expected: btcjson.SubmitPackageResult{
				PackageMsg:           "success",
				TxResults:            map[string]btcjson.SubmitPackageTxResult{},
				ReplacedTransactions: nil,
			},
		},
		{
			name: "invalid txid",
			json: `{
				"package_msg": "success",
				"tx-results": {
					"wtxid1": {
						"txid": "invalid-txid",
						"vsize": 150,
						"fees": {
							"base": 0.00001000
						}
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "invalid replaced transaction",
			json: `{
				"package_msg": "success",
				"tx-results": {},
				"replaced-transactions": ["not-a-valid-hash"]
			}`,
			wantErr: true,
		},
		{
			name: "invalid other-wtxid",
			json: `{
				"package_msg": "success",
				"tx-results": {
					"wtxid1": {
						"txid": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
						"other-wtxid": "invalid-wtxid",
						"vsize": 150,
						"fees": {
							"base": 0.00001000
						}
					}
				}
			}`,
			wantErr: true,
		},
		{
			name: "empty other-wtxid",
			json: `{
				"package_msg": "success",
				"tx-results": {
					"wtxid1": {
						"txid": "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
						"other-wtxid": "",
						"vsize": 150,
						"fees": {
							"base": 0.00001000
						}
					}
				}
			}`,
			expected: btcjson.SubmitPackageResult{
				PackageMsg: "success",
				TxResults: map[string]btcjson.SubmitPackageTxResult{
					"wtxid1": {
						TxID:       *mustParseHash("1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
						OtherWtxid: nil,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result btcjson.SubmitPackageResult
			err := result.UnmarshalJSON([]byte(test.json))

			if test.wantErr {
				require.Error(
					t, err, "expected error for test: %s",
					test.name,
				)

				return
			}

			require.NoError(
				t, err, "unexpected error for test: %s",
				test.name,
			)

			require.Equal(
				t, test.expected.PackageMsg, result.PackageMsg,
				"unexpected PackageMsg for test: %s", test.name,
			)
		})
	}
}

// mustParseHash parses a hash string and panics on error.
// This is a helper for tests only.
func mustParseHash(s string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(s)
	if err != nil {
		panic(err)
	}

	return hash
}
