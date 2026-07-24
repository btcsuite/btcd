package miniscript

import (
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestCombineTimelocks checks the time-lock folding used to detect time-lock
// mixing. A conjunction (k > 1) that puts a height-based and a time-based lock
// of the same kind on one spending path flags an unspendable combination; a
// disjunction (k == 1) never does. The individual lock flags are always OR'd
// across the sub expressions.
func TestCombineTimelocks(t *testing.T) {
	t.Parallel()

	csvHeight := timelockInfo{csvWithHeight: true}
	csvTime := timelockInfo{csvWithTime: true}
	cltvHeight := timelockInfo{cltvWithHeight: true}
	cltvTime := timelockInfo{cltvWithTime: true}
	combined := timelockInfo{containsCombination: true}

	tests := []struct {
		name            string
		k               int
		subs            []timelockInfo
		wantCombination bool
	}{{
		name:            "and mixes relative height and time",
		k:               2,
		subs:            []timelockInfo{csvHeight, csvTime},
		wantCombination: true,
	}, {
		name:            "and mixes absolute height and time",
		k:               2,
		subs:            []timelockInfo{cltvHeight, cltvTime},
		wantCombination: true,
	}, {
		name:            "or does not mix",
		k:               1,
		subs:            []timelockInfo{csvHeight, csvTime},
		wantCombination: false,
	}, {
		name:            "relative and absolute do not conflict",
		k:               2,
		subs:            []timelockInfo{csvHeight, cltvTime},
		wantCombination: false,
	}, {
		name:            "same subtype does not conflict",
		k:               2,
		subs:            []timelockInfo{csvHeight, csvHeight},
		wantCombination: false,
	}, {
		name:            "threshold k>1 mixes",
		k:               3,
		subs:            []timelockInfo{csvHeight, csvTime, {}},
		wantCombination: true,
	}, {
		name:            "existing combination propagates under or",
		k:               1,
		subs:            []timelockInfo{combined, {}},
		wantCombination: true,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := combineTimelocks(tc.k, tc.subs...)
			require.Equal(
				t, tc.wantCombination,
				got.containsCombination,
			)
		})
	}

	// The and/or helpers are thin wrappers over combineTimelocks with k=2
	// and k=1 respectively.
	require.True(t, combineTimelocksAnd(
		csvHeight, csvTime,
	).containsCombination)
	require.False(t, combineTimelocksOr(
		csvHeight, csvTime,
	).containsCombination)

	// The individual lock flags are OR'd across all sub expressions.
	got := combineTimelocksOr(csvHeight, csvTime)
	require.True(t, got.csvWithHeight)
	require.True(t, got.csvWithTime)
}

// TestComputeTimelocksLeaf checks that the after and older leaves land in the
// right lock category: after uses the absolute-locktime threshold to
// distinguish block heights from Unix time, older uses the BIP68 seconds flag.
func TestComputeTimelocksLeaf(t *testing.T) {
	t.Parallel()

	leaf := func(id string, num uint64) timelockInfo {
		node := &AST{identifier: id, args: []*AST{{num: num}}}
		out, err := computeTimelocks(node)
		require.NoError(t, err)
		return out.timelock
	}

	threshold := uint64(txscript.LockTimeThreshold)
	secondsBit := uint64(wire.SequenceLockTimeIsSeconds)

	require.Equal(
		t, timelockInfo{cltvWithHeight: true}, leaf(f_after, 100),
	)
	require.Equal(
		t, timelockInfo{cltvWithTime: true}, leaf(f_after, threshold),
	)
	require.Equal(
		t, timelockInfo{csvWithHeight: true}, leaf(f_older, 100),
	)
	require.Equal(
		t, timelockInfo{csvWithTime: true},
		leaf(f_older, secondsBit|100),
	)
}
