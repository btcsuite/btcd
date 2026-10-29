// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
)

// TestNewRetargetParams ensures the derived retarget values match the expected
// numbers for several networks and agree with what a BlockChain created by New
// reports through the ChainCtx methods.
func TestNewRetargetParams(t *testing.T) {
	tests := []struct {
		name   string
		params *chaincfg.Params
		want   RetargetParams
	}{{
		name:   "mainnet",
		params: &chaincfg.MainNetParams,
		want: RetargetParams{
			BlocksPerRetarget:   2016,
			MinRetargetTimespan: 302400,
			MaxRetargetTimespan: 4838400,
		},
	}, {
		name:   "testnet3",
		params: &chaincfg.TestNet3Params,
		want: RetargetParams{
			BlocksPerRetarget:   2016,
			MinRetargetTimespan: 302400,
			MaxRetargetTimespan: 4838400,
		},
	}, {
		name:   "regtest",
		params: &chaincfg.RegressionNetParams,
		want: RetargetParams{
			BlocksPerRetarget:   2016,
			MinRetargetTimespan: 302400,
			MaxRetargetTimespan: 4838400,
		},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := NewRetargetParams(test.params)
			if got != test.want {
				t.Fatalf("got %+v, want %+v", got, test.want)
			}

			chain, teardown, err := chainSetup(
				"retargetparams"+test.name, test.params,
			)
			if err != nil {
				t.Fatalf("chainSetup: %v", err)
			}
			defer teardown()

			var ctx ChainCtx = chain
			if ctx.BlocksPerRetarget() != got.BlocksPerRetarget {
				t.Fatalf("BlocksPerRetarget: got %d, want %d",
					ctx.BlocksPerRetarget(),
					got.BlocksPerRetarget)
			}
			if ctx.MinRetargetTimespan() != got.MinRetargetTimespan {
				t.Fatalf("MinRetargetTimespan: got %d, want %d",
					ctx.MinRetargetTimespan(),
					got.MinRetargetTimespan)
			}
			if ctx.MaxRetargetTimespan() != got.MaxRetargetTimespan {
				t.Fatalf("MaxRetargetTimespan: got %d, want %d",
					ctx.MaxRetargetTimespan(),
					got.MaxRetargetTimespan)
			}
		})
	}
}
