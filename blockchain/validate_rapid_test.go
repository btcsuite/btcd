package blockchain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestAssertNoTimeWarpProperties uses property-based testing to verify that
// the assertNoTimeWarp function correctly implements the BIP-94 rule. This
// helps catch edge cases that might be missed with regular unit tests.
func TestAssertNoTimeWarpProperties(t *testing.T) {
	t.Parallel()

	// Define constant for blocks per retarget (similar to Bitcoin's 2016).
	const blocksPerRetarget = 2016

	// Rapid test that only the retarget blocks are checked.
	t.Run("only_checks_retarget_blocks", rapid.MakeCheck(func(t *rapid.T) {
		// Generate block height that is not a retarget block.
		height := rapid.Int32Range(
			1, 1000000,
		).Filter(func(h int32) bool {
			return h%blocksPerRetarget != 0
		}).Draw(t, "height")

		// Even with an "extreme" time warp, the function should return
		// nil because it only applies the check to retarget blocks.
		// Define headerTime as the Unix epoch start.
		headerTime := time.Unix(0, 0)

		// Define prevBlockTime as the current time (creating an
		// extreme gap).
		prevBlockTime := time.Now()

		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.NoError(
			t, err, "expected nil error for non-retarget block "+
				"but got: %v.", err,
		)
	}))

	// Rapid test that retarget blocks with acceptable timestamps pass
	// validation.
	t.Run("valid_timestamps_pass", rapid.MakeCheck(func(t *rapid.T) {
		// Generate block height that is a retarget block
		height := rapid.Int32Range(blocksPerRetarget, 1000000).
			Filter(func(h int32) bool {
				return h%blocksPerRetarget == 0
			}).Draw(t, "height")

		// Generate a previous block timestamp.
		prevTimeUnix := rapid.Int64Range(
			1000000, 2000000000,
		).Draw(t, "prev_time")
		prevBlockTime := time.Unix(prevTimeUnix, 0)

		// Generate a header timestamp that is not more than
		// maxTimeWarp earlier than the previous block timestamp.
		minValidHeaderTime := prevBlockTime.Add(
			-maxTimeWarp,
		).Add(time.Second)

		// Generate any valid header time between the minimum valid
		// time and prevBlockTime to ensure it passes the time warp
		// check.
		minTimeUnix := minValidHeaderTime.Unix()
		maxTimeUnix := prevBlockTime.Unix()

		// Ensure min is always less than max.
		if minTimeUnix >= maxTimeUnix {
			// If a valid range cannot be generated, use the
			// previous block time which is guaranteed to pass the
			// test.
			headerTime := prevBlockTime
			err := assertNoTimeWarp(
				height, blocksPerRetarget, headerTime, prevBlockTime,
			)
			require.NoError(t, err, "expected valid timestamps to "+
				"pass but got: %v.")
			return
		}

		headerTimeUnix := rapid.Int64Range(
			minTimeUnix, maxTimeUnix,
		).Draw(t, "header_time_unix")
		headerTime := time.Unix(headerTimeUnix, 0)

		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.NoError(t, err, "expected valid timestamps to pass but "+
			"got: %v.")
	}))

	// Rapid test that retarget blocks with invalid timestamps fail
	t.Run("invalid_timestamps_fail", rapid.MakeCheck(func(t *rapid.T) {
		// validation.
		// Generate block height that is a retarget block.
		height := rapid.Int32Range(blocksPerRetarget, 1000000).
			Filter(func(h int32) bool {
				return h%blocksPerRetarget == 0
			}).Draw(t, "height")

		// Generate a previous block timestamp.
		prevTimeUnix := rapid.Int64Range(
			1000000, 2000000000,
		).Draw(t, "prev_time")
		prevBlockTime := time.Unix(prevTimeUnix, 0)

		// Invalid header timestamp: more than maxTimeWarp earlier than
		// prevBlockTime Ensure we generate a time that is definitely
		// beyond the maxTimeWarp (which is 600 seconds) by using at
		// least 601 seconds.
		invalidDelta := time.Duration(
			-rapid.Int64Range(601, 86400).Draw(t, "invalid_delta"),
		) * time.Second
		headerTime := prevBlockTime.Add(invalidDelta)

		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.Error(t, err, "expected error for time-warped header but got nil.")

		// Verify the correct error type is returned.
		require.IsType(
			t, RuleError{}, err, "expected RuleError but got: %T.", err,
		)

		// Verify it's the expected ErrTimewarpAttack error.
		ruleErr, ok := err.(RuleError)
		require.True(t, ok, "expected RuleError but got: %T.", err)
		require.Equal(
			t, ErrTimewarpAttack, ruleErr.ErrorCode, "expected "+
				"ErrTimewarpAttack but got: %v.", ruleErr.ErrorCode,
		)
	}))

	// Test the edge case right at the boundary of maxTimeWarp.
	t.Run("boundary_timestamps", rapid.MakeCheck(func(t *rapid.T) {
		// Generate block height that is a retarget block.
		height := rapid.Int32Range(blocksPerRetarget, 1000000).
			Filter(func(h int32) bool {
				return h%blocksPerRetarget == 0
			}).Draw(t, "height")

		// Generate a previous block timestamp with enough padding
		// to avoid time.Time precision issues.
		prevTimeUnix := rapid.Int64Range(
			1000000, 2000000000,
		).Draw(t, "prev_time")
		prevBlockTime := time.Unix(prevTimeUnix, 0)

		// Test exact boundary: headerTime is exactly maxTimeWarp earlier.
		headerTime := prevBlockTime.Add(-maxTimeWarp)

		// Check the actual implementation (looking at
		// validate.go:797-798) The comparison is
		// "headerTimestamp.Before(prevBlockTimestamp.Add(-maxTimeWarp))"
		// This means at exact boundary (headerTime ==
		// prevBlockTime.Add(-maxTimeWarp)) it should NOT fail, since
		// Before() is strict < not <=.
		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.NoError(
			t, err, "expected no error at exact boundary but "+
				"got: %v.",
		)

		// Test 1 nanosecond BEYOND the boundary (which should fail).
		headerTime = prevBlockTime.Add(-maxTimeWarp).Add(
			-time.Nanosecond,
		)

		// This should fail as it is just beyond the maxTimeWarp limit.
		err = assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.Error(
			t, err, "expected error just beyond boundary but "+
				"got nil.",
		)
	}))
}

// TestAssertNoTimeWarpInvariants uses property-based testing to verify the
// invariants of the assertNoTimeWarp function regardless of inputs.
func TestAssertNoTimeWarpInvariants(t *testing.T) {
	t.Parallel()

	// Invariant: The function should never panic regardless of input.
	t.Run("never_panics", rapid.MakeCheck(func(t *rapid.T) {
		// Generate any possible inputs
		height := rapid.Int32().Draw(t, "height")
		blocksPerRetarget := rapid.Int32Range(
			1, 10000,
		).Draw(t, "blocks_per_retarget")
		headerTimeUnix := rapid.Int64().Draw(t, "header_time")
		prevTimeUnix := rapid.Int64().Draw(t, "prev_time")

		headerTime := time.Unix(headerTimeUnix, 0)
		prevBlockTime := time.Unix(prevTimeUnix, 0)

		// The function should never panic regardless of input
		_ = assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
	}))

	// Invariant: For non-retarget blocks, the function always returns nil.
	// nolint:lll.
	t.Run("non_retarget_blocks_return_nil", rapid.MakeCheck(func(t *rapid.T) {
		// Generate height and blocksPerRetarget such that height is
		// not a multiple of blocksPerRetarget.
		blocksPerRetarget := rapid.Int32Range(2, 10000).Draw(
			t, "blocks_per_retarget",
		)

		// Ensure height is not a multiple of blocksPerRetarget.
		remainders := rapid.Int32Range(1, blocksPerRetarget-1).Draw(
			t, "remainder",
		)
		height := rapid.Int32Range(0, 1000000).Draw(
			t, "base",
		)*blocksPerRetarget + remainders

		// Generate any timestamps, even invalid ones.
		headerTime := time.Unix(rapid.Int64().Draw(t, "header_time"), 0)
		prevBlockTime := time.Unix(
			rapid.Int64().Draw(t, "prev_time"), 0,
		)

		// For non-retarget blocks, should always return nil.
		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.NoError(
			t, err, "expected nil for non-retarget block "+
				"(height=%d, blocks_per_retarget=%d) but "+
				"got: %v.", height, blocksPerRetarget, err,
		)
	}))
}

// TestAssertNoTimeWarpSecurity tests the security properties of the
// assertNoTimeWarp function. This verifies that the function properly prevents
// "time warp" attacks where miners might attempt to manipulate timestamps for
// difficulty adjustment blocks.
func TestAssertNoTimeWarpSecurity(t *testing.T) {
	t.Parallel()

	const blocksPerRetarget = 2016

	// Test that all difficulty adjustment blocks are protected from timewarp.
	t.Run("all_retarget_blocks_protected", rapid.MakeCheck(func(t *rapid.T) { //nolint:lll
		// Generate any retarget block height (multiples of
		// blocksPerRetarget).
		multiplier := rapid.Int32Range(1, 1000).Draw(t, "multiplier")
		height := multiplier * blocksPerRetarget

		// Generate a reasonable previous block timestamp.
		prevTimeUnix := rapid.Int64Range(
			1000000, 2000000000,
		).Draw(t, "prev_time")
		prevBlockTime := time.Unix(prevTimeUnix, 0)

		// Generate a test header timestamp that's significantly before
		// the previous timestamp This should always be rejected for
		// retarget blocks.
		timeDiff := rapid.Int64Range(
			int64(maxTimeWarp+time.Second),
			int64(maxTimeWarp+time.Hour*24*7),
		).Draw(t, "warp_amount")
		invalidDelta := time.Duration(-timeDiff)
		headerTime := prevBlockTime.Add(invalidDelta)

		// This should always fail with ErrTimewarpAttack for any retarget block.
		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.Error(
			t, err, "security vulnerability: Time warp attack not "+
				"detected for height %d.", height,
		)

		// Verify it's the expected error type.
		ruleErr, ok := err.(RuleError)
		require.True(t, ok, "expected RuleError but got: %T.", err)
		require.Equal(
			t, ErrTimewarpAttack, ruleErr.ErrorCode,
			"expected ErrTimewarpAttack but got: %v.",
			ruleErr.ErrorCode,
		)
	}))

	// Test that non-adjustment blocks are not subject to the same check.
	// nolint:lll.
	t.Run("non_retarget_blocks_not_affected", rapid.MakeCheck(func(t *rapid.T) {
		// Generate any non-retarget block height.
		baseHeight := rapid.Int32Range(0, 1000).Draw(
			t, "base_height",
		) * blocksPerRetarget
		offset := rapid.Int32Range(1, blocksPerRetarget-1).Draw(
			t, "offset",
		)
		height := baseHeight + offset

		// Generate a reasonable previous block timestamp.
		prevTimeUnix := rapid.Int64Range(1000000, 2000000000).Draw(
			t, "prev_time",
		)
		prevBlockTime := time.Unix(prevTimeUnix, 0)

		// Generate a test header timestamp that's significantly before
		// the previous timestamp. Even though this would be rejected
		// for retarget blocks, it shouldn't matter here.
		timeDiff := rapid.Int64Range(
			int64(maxTimeWarp+time.Second),
			int64(maxTimeWarp+time.Hour*24*7),
		).Draw(t, "warp_amount")
		invalidDelta := time.Duration(-timeDiff)
		headerTime := prevBlockTime.Add(invalidDelta)

		// This should NOT fail for non-retarget blocks, even with
		// extreme timewarp.
		err := assertNoTimeWarp(
			height, blocksPerRetarget, headerTime, prevBlockTime,
		)
		require.NoError(
			t, err, "non-retarget blocks should not be affected "+
				"by time warp check, but got: %v.", err,
		)
	}))
}
