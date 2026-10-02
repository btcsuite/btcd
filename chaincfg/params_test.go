// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestInvalidHashStr ensures the newShaHashFromStr function panics when used to
// with an invalid hash string.
func TestInvalidHashStr(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid hash, got nil")
		}
	}()
	newHashFromStr("banana")
}

// TestMustRegisterPanic ensures the mustRegister function panics when used to
// register an invalid network.
func TestMustRegisterPanic(t *testing.T) {
	t.Parallel()

	// Setup a defer to catch the expected panic to ensure it actually
	// paniced.
	defer func() {
		if err := recover(); err == nil {
			t.Error("mustRegister did not panic as expected")
		}
	}()

	// Intentionally try to register duplicate params to force a panic.
	mustRegister(&MainNetParams)
}

func TestRegisterHDKeyID(t *testing.T) {
	t.Parallel()

	// Ref: https://github.com/satoshilabs/slips/blob/master/slip-0132.md
	hdKeyIDZprv := []byte{0x02, 0xaa, 0x7a, 0x99}
	hdKeyIDZpub := []byte{0x02, 0xaa, 0x7e, 0xd3}

	if err := RegisterHDKeyID(hdKeyIDZpub, hdKeyIDZprv); err != nil {
		t.Fatalf("RegisterHDKeyID: expected no error, got %v", err)
	}

	got, err := HDPrivateKeyToPublicKeyID(hdKeyIDZprv)
	if err != nil {
		t.Fatalf("HDPrivateKeyToPublicKeyID: expected no error, got %v", err)
	}

	if !bytes.Equal(got, hdKeyIDZpub) {
		t.Fatalf("HDPrivateKeyToPublicKeyID: expected result %v, got %v",
			hdKeyIDZpub, got)
	}
}

func TestInvalidHDKeyID(t *testing.T) {
	t.Parallel()

	prvValid := []byte{0x02, 0xaa, 0x7a, 0x99}
	pubValid := []byte{0x02, 0xaa, 0x7e, 0xd3}
	prvInvalid := []byte{0x00}
	pubInvalid := []byte{0x00}

	if err := RegisterHDKeyID(pubInvalid, prvValid); err != ErrInvalidHDKeyID {
		t.Fatalf("RegisterHDKeyID: want err ErrInvalidHDKeyID, got %v", err)
	}

	if err := RegisterHDKeyID(pubValid, prvInvalid); err != ErrInvalidHDKeyID {
		t.Fatalf("RegisterHDKeyID: want err ErrInvalidHDKeyID, got %v", err)
	}

	if err := RegisterHDKeyID(pubInvalid, prvInvalid); err != ErrInvalidHDKeyID {
		t.Fatalf("RegisterHDKeyID: want err ErrInvalidHDKeyID, got %v", err)
	}

	// FIXME: The error type should be changed to ErrInvalidHDKeyID.
	if _, err := HDPrivateKeyToPublicKeyID(prvInvalid); err != ErrUnknownHDKeyID {
		t.Fatalf("HDPrivateKeyToPublicKeyID: want err ErrUnknownHDKeyID, got %v", err)
	}
}

func TestSigNetPowLimit(t *testing.T) {
	sigNetPowLimitHex, _ := hex.DecodeString(
		"00000377ae000000000000000000000000000000000000000000000000000000",
	)
	powLimit := new(big.Int).SetBytes(sigNetPowLimitHex)
	if sigNetPowLimit.Cmp(powLimit) != 0 {
		t.Fatalf("Signet PoW limit bits (%s) not equal to big int (%s)",
			sigNetPowLimit.Text(16), powLimit.Text(16))
	}

	if compactToBig(sigNetGenesisBlock.Header.Bits).Cmp(powLimit) != 0 {
		t.Fatalf("Signet PoW limit header bits (%d) not equal to big "+
			"int (%s)", sigNetGenesisBlock.Header.Bits,
			powLimit.Text(16))
	}
}

// TestSigNetMagic makes sure that the default signet has the expected bitcoin
// network magic.
func TestSigNetMagic(t *testing.T) {
	require.Equal(t, wire.SigNet, SigNetParams.Net)
}

// TestCustomSignetParamsWithBlockTime checks that custom block intervals are
// validated and that other signet parameters retain their existing values.
func TestCustomSignetParamsWithBlockTime(t *testing.T) {
	t.Parallel()

	challenge := []byte{0x51}
	seeds := []DNSSeed{{Host: "seed.example", HasFiltering: true}}
	legacy := CustomSignetParams(challenge, seeds)

	tests := []struct {
		name     string
		interval time.Duration
		wantErr  error
	}{
		{
			name:    "zero",
			wantErr: ErrInvalidSignetBlockTime,
		},
		{
			name:     "negative",
			interval: -time.Second,
			wantErr:  ErrInvalidSignetBlockTime,
		},
		{
			name:     "subsecond",
			interval: 500 * time.Millisecond,
			wantErr:  ErrInvalidSignetBlockTime,
		},
		{
			name:     "fractional seconds",
			interval: 1500 * time.Millisecond,
			wantErr:  ErrInvalidSignetBlockTime,
		},
		{
			name:     "above timespan",
			interval: signetTargetTimespan + time.Second,
			wantErr:  ErrInvalidSignetBlockTime,
		},
		{
			name:     "one second",
			interval: time.Second,
		},
		{
			name:     "mutinynet",
			interval: 30 * time.Second,
		},
		{
			name:     "default interval",
			interval: 10 * time.Minute,
		},
		{
			name:     "nondivisible timespan",
			interval: 11 * time.Second,
		},
		{
			name:     "equal to timespan",
			interval: signetTargetTimespan,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			params, err := CustomSignetParamsWithBlockTime(
				challenge, seeds, tc.interval,
			)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, Params{}, params)
				return
			}
			require.NoError(t, err)

			// Changing the interval must only affect the interval
			// itself and the value derived from it.
			expected := legacy
			expected.TargetTimePerBlock = tc.interval
			expected.MinDiffReductionTime = tc.interval * 2
			require.Equal(t, expected, params)
		})
	}
}

// TestCustomSignetParamsEmptyChallenge makes sure that a custom signet cannot
// be constructed without a block challenge script.
func TestCustomSignetParamsEmptyChallenge(t *testing.T) {
	t.Parallel()

	_, err := CustomSignetParamsWithBlockTime(
		nil, DefaultSignetDNSSeeds, 30*time.Second,
	)
	require.ErrorIs(t, err, ErrEmptySignetChallenge)
}

// TestCustomSignetParamsDefaults makes sure that the default signet parameters
// are reproduced when the default challenge, seeds and 10-minute interval are
// passed to the validating constructor.
func TestCustomSignetParamsDefaults(t *testing.T) {
	t.Parallel()

	require.Equal(t, 10*time.Minute, SigNetParams.TargetTimePerBlock)

	params, err := CustomSignetParamsWithBlockTime(
		DefaultSignetChallenge, DefaultSignetDNSSeeds, 10*time.Minute,
	)
	require.NoError(t, err)
	require.Equal(t, SigNetParams, params)
}

// compactToBig is a copy of the blockchain.CompactToBig function. We copy it
// here so we don't run into a circular dependency just because of a test.
func compactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes to represent the full 256-bit number.  So,
	// treat the exponent as the number of bytes and shift the mantissa
	// right or left accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}
