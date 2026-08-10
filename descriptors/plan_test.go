package descriptors

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// testEcdsaSigHex is a valid DER-encoded ECDSA signature (with its sighash
// byte) used in the P2WSH plan tests.
const testEcdsaSigHex = "3045022100e621a7686d51fb23e761adff4367881a6fb16b" +
	"c5635ff34eea39afdaf033e4d702207998512f52bd3dae100951a6df9e66bcb78" +
	"c194dcaa3c7fd2451180b5cc94d4e01"

// testTapLeafSig is a 64-byte dummy Schnorr signature used in the plan tests.
var testTapLeafSig = []byte(
	"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
)

// TestWitnessSerializedSize checks the serialized size of a witness stack: the
// element-count var-int plus each element's length var-int and its bytes.
func TestWitnessSerializedSize(t *testing.T) {
	t.Parallel()

	// Empty stack: only the count var-int (one byte for zero).
	require.Equal(t, uint64(1), witnessSerializedSize([][]byte{}))

	// One 64-byte element: count(1) + len(1) + 64.
	require.Equal(t, uint64(66), witnessSerializedSize(
		[][]byte{make([]byte, 64)},
	))

	// Two elements: count(1) + (1 + 72) + (1 + 33).
	require.Equal(t, uint64(108), witnessSerializedSize(
		[][]byte{make([]byte, 72), make([]byte, 33)},
	))

	// A 253-byte element needs a three-byte length var-int: count(1) + 3 +
	// 253.
	require.Equal(t, uint64(257), witnessSerializedSize(
		[][]byte{make([]byte, 253)},
	))
}

// TestPlanAbsoluteLocktime checks every condition an after(n) fragment depends
// on: BIP65 only enforces an absolute locktime for an input whose sequence is
// not SEQUENCE_FINAL, the transaction locktime has to reach the required value,
// and both have to be of the same kind (block height or unix time).
//
// The final-sequence condition could not even be expressed before, so a plan
// was produced for a spend that consensus rejects.
func TestPlanAbsoluteLocktime(t *testing.T) {
	t.Parallel()

	// The value at which a locktime stops being a block height and starts
	// being a unix timestamp (BIP65), and a locktime just above it, i.e.
	// one that is a point in time rather than a height.
	const (
		threshold  = uint32(500000000)
		afterATime = "after(500000100)"
	)

	lock := func(v uint32) *uint32 { return &v }

	tests := []struct {
		name     string
		after    string
		lockTime *uint32
		sequence *uint32
		want     bool
	}{{
		name:  "no transaction context",
		after: "after(100)",
	}, {
		name:     "no locktime",
		after:    "after(100)",
		sequence: lock(0),
	}, {
		name:     "no sequence",
		after:    "after(100)",
		lockTime: lock(200),
	}, {
		// OP_CHECKLOCKTIMEVERIFY fails the script for an input whose
		// sequence is final, whatever the transaction locktime is.
		name:     "sequence final",
		after:    "after(100)",
		lockTime: lock(200),
		sequence: lock(wire.MaxTxInSequenceNum),
	}, {
		name:     "height reached",
		after:    "after(100)",
		lockTime: lock(200),
		sequence: lock(0xfffffffe),
		want:     true,
	}, {
		name:     "height equal",
		after:    "after(200)",
		lockTime: lock(200),
		sequence: lock(0xfffffffe),
		want:     true,
	}, {
		name:     "height not reached",
		after:    "after(300)",
		lockTime: lock(200),
		sequence: lock(0xfffffffe),
	}, {
		name:     "time reached",
		after:    afterATime,
		lockTime: lock(threshold + 200),
		sequence: lock(0xfffffffe),
		want:     true,
	}, {
		name:     "height required, time given",
		after:    "after(100)",
		lockTime: lock(threshold + 5),
		sequence: lock(0xfffffffe),
	}, {
		name:     "time required, height given",
		after:    afterATime,
		lockTime: lock(200),
		sequence: lock(0xfffffffe),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checkLocktimePlan(
				t, tc.after, Assets{
					LookupEcdsaSig: func(string) bool {
						return true
					},
					TxLockTime:      tc.lockTime,
					TxInputSequence: tc.sequence,
				}, tc.want,
			)
		})
	}
}

// TestPushScriptAndAll checks the script-push helpers: pushScript pushes a
// single element, pushAll pushes several in order.
func TestPushScriptAndAll(t *testing.T) {
	t.Parallel()

	data := bytes.Repeat([]byte{0xab}, 34)
	script, err := pushScript(data)
	require.NoError(t, err)
	require.Equal(t, append([]byte{0x22}, data...), script)

	first := bytes.Repeat([]byte{0x01}, 33)
	second := bytes.Repeat([]byte{0x02}, 20)
	all, err := pushAll([][]byte{first, second})
	require.NoError(t, err)

	want := append([]byte{0x21}, first...)
	want = append(want, 0x14)
	want = append(want, second...)
	require.Equal(t, want, all)
}

// TestPlanSatisfyNil checks that Satisfy reports a nil satisfier as an error
// for every kind of plan. The path-specific closures read the lookups off the
// pointer, so a nil one used to panic and take the calling process down instead
// of returning the error the signature promises.
func TestPlanSatisfyNil(t *testing.T) {
	t.Parallel()

	key := testCompressedKeys(2)[0]

	// A version 2 transaction with a non-final input sequence, so that a
	// plan can rely on either kind of locktime.
	version := int32(2)
	lockTime, sequence := uint32(1), uint32(0xfffffffe)
	assets := Assets{
		LookupEcdsaSig: func(string) bool { return true },
		LookupTapKeySpendSig: func(string) (uint32, bool) {
			return 64, true
		},
		TxVersion:       &version,
		TxLockTime:      &lockTime,
		TxInputSequence: &sequence,
	}

	for _, desc := range planKinds(key) {
		t.Run(desc, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(desc)
			require.NoError(t, err)

			plan, err := d.PlanAt(0, 0, assets)
			require.NoError(t, err)

			_, err = plan.Satisfy(nil)
			require.ErrorIs(t, err, errCouldNotSatisfy)
			require.ErrorContains(t, err, "no satisfier provided")
		})
	}
}

// planKinds returns one descriptor of every kind that can be planned, all of
// them satisfiable with a signature for the given key.
func planKinds(key string) []string {
	return []string{
		"wpkh(" + key + ")",
		"pkh(" + key + ")",
		"pk(" + key + ")",
		"multi(1," + key + ")",
		"wsh(pk(" + key + "))",
		"wsh(and_v(v:pk(" + key + "),after(1)))",
		"sh(wpkh(" + key + "))",
		"sh(wsh(pk(" + key + ")))",
		"sh(pk(" + key + "))",
	}
}

// TestDescKeyDefiniteString checks that a key's path is resolved at a given
// multipath and derivation index: the multipath element is replaced by its
// selected value and the wildcard by the derivation index, while the origin and
// fixed steps are kept.
func TestDescKeyDefiniteString(t *testing.T) {
	t.Parallel()

	origin := "[e81a5744/48'/0'/0'/2']"
	tests := []struct {
		name string
		raw  string
		mp   uint32
		idx  uint32
		want string
	}{{
		name: "no path",
		raw:  basicTestXpub,
		want: basicTestXpub,
	}, {
		name: "wildcard",
		raw:  basicTestXpub + "/*",
		idx:  5,
		want: basicTestXpub + "/5",
	}, {
		name: "fixed path",
		raw:  basicTestXpub + "/3/7",
		want: basicTestXpub + "/3/7",
	}, {
		name: "origin multipath wildcard",
		raw:  origin + basicTestXpub + "/<0;1>/*",
		mp:   1,
		idx:  4,
		want: origin + basicTestXpub + "/1/4",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			k, err := parseDescKey(tc.raw, keyFormCompressed)
			require.NoError(t, err)
			require.Equal(
				t, tc.want,
				k.definiteString(tc.mp, tc.idx),
			)
		})
	}
}

// TestPlanRelativeLocktime checks every condition an older(n) fragment depends
// on: BIP68 only enforces a relative locktime for a transaction of version 2 or
// later and for an input whose sequence has the disable flag (bit 31) clear,
// and the sequence has to reach the required value in the same unit.
//
// The planner used to see the sequence alone, so it produced plans for spends
// that consensus rejects: one with the default sequence of a wallet, which has
// the disable flag set, and one with a version 1 transaction.
func TestPlanRelativeLocktime(t *testing.T) {
	t.Parallel()

	const (
		seconds  = uint32(wire.SequenceLockTimeIsSeconds)
		disabled = uint32(wire.SequenceLockTimeDisabled)
	)

	// A relative locktime of five 512-second units: the BIP68 type flag
	// plus the value.
	const olderFiveUnits = "older(4194309)"

	version := func(v int32) *int32 { return &v }
	seq := func(v uint32) *uint32 { return &v }

	tests := []struct {
		name     string
		older    string
		version  *int32
		sequence *uint32
		want     bool
	}{{
		name:  "no transaction context",
		older: "older(5)",
	}, {
		name:     "no version",
		older:    "older(5)",
		sequence: seq(10),
	}, {
		name:    "no sequence",
		older:   "older(5)",
		version: version(2),
	}, {
		// BIP112 fails the script for a transaction that predates
		// BIP68, whatever the sequence says.
		name:     "version too low",
		older:    "older(5)",
		version:  version(1),
		sequence: seq(10),
	}, {
		name:     "height reached",
		older:    "older(5)",
		version:  version(2),
		sequence: seq(10),
		want:     true,
	}, {
		name:     "height equal",
		older:    "older(10)",
		version:  version(2),
		sequence: seq(10),
		want:     true,
	}, {
		name:     "height not reached",
		older:    "older(20)",
		version:  version(2),
		sequence: seq(10),
	}, {
		name:     "time reached",
		older:    olderFiveUnits,
		version:  version(2),
		sequence: seq(seconds | 10),
		want:     true,
	}, {
		name:     "unit mismatch",
		older:    olderFiveUnits,
		version:  version(2),
		sequence: seq(10),
	}, {
		// SEQUENCE_FINAL, the value wallets use by default, has the
		// disable flag set, so OP_CHECKSEQUENCEVERIFY fails the script
		// no matter what its value bits say.
		name:     "sequence final",
		older:    "older(5)",
		version:  version(2),
		sequence: seq(0xffffffff),
	}, {
		// The other common default, 0xfffffffd, is disable-flagged too.
		name:     "replaceable final sequence",
		older:    "older(5)",
		version:  version(2),
		sequence: seq(0xfffffffd),
	}, {
		name:     "disable flag with matching value",
		older:    "older(5)",
		version:  version(2),
		sequence: seq(disabled | 10),
	}, {
		name:     "disable flag with matching time value",
		older:    olderFiveUnits,
		version:  version(2),
		sequence: seq(disabled | seconds | 10),
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			checkLocktimePlan(
				t, tc.older, Assets{
					LookupEcdsaSig: func(string) bool {
						return true
					},
					TxVersion:       tc.version,
					TxInputSequence: tc.sequence,
				}, tc.want,
			)
		})
	}
}

// checkLocktimePlan plans a spend of a key-and-timelock descriptor holding the
// given locktime fragment and asserts whether the given assets can satisfy it.
func checkLocktimePlan(t *testing.T, fragment string, assets Assets,
	want bool) {

	key := testCompressedKeys(1)[0]
	d, err := NewDescriptor(
		"wsh(and_v(v:pk(" + key + ")," + fragment + "))",
	)
	require.NoError(t, err)

	plan, err := d.PlanAt(0, 0, assets)
	if !want {
		require.ErrorIs(t, err, errCouldNotPlan)

		return
	}

	require.NoError(t, err)
	require.NotZero(t, plan.SatisfactionWeight())
}

// TestHashFragmentPlan checks that planning a descriptor with a hash fragment
// works as well: the same lookup runs while building the satisfaction, so a
// spending path that avoids the hash branch used to fail with `unknown key`
// rather than being planned.
func TestHashFragmentPlan(t *testing.T) {
	t.Parallel()

	keys := testCompressedKeys(2)

	// Either the first key signs, or the second one signs and reveals a
	// preimage. Only the first path is plannable with the assets below,
	// which hold no preimage.
	d, err := NewDescriptor(
		"wsh(or_d(pk(" + keys[0] + "),and_v(v:pkh(" + keys[1] +
			"),sha256(" + testSha256 + "))))",
	)
	require.NoError(t, err)

	plan, err := d.PlanAt(0, 0, Assets{
		LookupEcdsaSig: func(pk string) bool {
			return pk == keys[0]
		},
	})
	require.NoError(t, err)
	require.NotZero(t, plan.WitnessSize())

	sig, err := hex.DecodeString(testEcdsaSigHex)
	require.NoError(t, err)

	result, err := plan.Satisfy(&Satisfier{
		LookupEcdsaSig: func(pk string) ([]byte, bool) {
			if pk != keys[0] {
				return nil, false
			}

			return sig, true
		},
	})
	require.NoError(t, err)

	// The or_d satisfaction is the signature of the first key, followed by
	// the witness script the spend has to reveal.
	witnessScript, err := d.ScriptCodeAt(0, 0)
	require.NoError(t, err)
	require.Equal(t, [][]byte{sig, witnessScript}, result.Witness)

	// Without a signature for the first key there is no plan, since the
	// other path needs a preimage that the assets do not have.
	_, err = d.PlanAt(0, 0, Assets{
		LookupEcdsaSig: func(pk string) bool {
			return pk == keys[1]
		},
	})
	require.ErrorIs(t, err, errCouldNotPlan)
}

// TestPlanHashPreimage checks that every one of the four hash fragments can be
// planned and satisfied through the preimage lookups of Assets and Satisfier.
// Without them the whole fragment class was unspendable through this API: a
// hashlocked descriptor parsed and derived addresses, and at spend time there
// was no way to hand it the preimage.
func TestPlanHashPreimage(t *testing.T) {
	t.Parallel()

	key := testCompressedKeys(1)[0]

	sig, err := hex.DecodeString(testEcdsaSigHex)
	require.NoError(t, err)

	preimage := bytes.Repeat([]byte{0x11}, hashPreimageLen)

	// The hash value of a fragment is the digest of its hash function, so
	// the 20-byte one is used for the two that end in RIPEMD160.
	for _, tc := range []struct {
		hashFunc string
		hash     string
	}{{
		hashFunc: "sha256",
		hash:     testSha256,
	}, {
		hashFunc: "hash256",
		hash:     testSha256,
	}, {
		hashFunc: "ripemd160",
		hash:     testHash160,
	}, {
		hashFunc: "hash160",
		hash:     testHash160,
	}} {

		t.Run(tc.hashFunc, func(t *testing.T) {
			t.Parallel()

			d, err := NewDescriptor(
				"wsh(and_v(v:pk(" + key + ")," +
					tc.hashFunc + "(" + tc.hash + ")))",
			)
			require.NoError(t, err)

			wantHash, err := hex.DecodeString(tc.hash)
			require.NoError(t, err)

			assets := Assets{
				LookupEcdsaSig: func(string) bool {
					return true
				},

				// The lookup is called with the fragment and
				// the hash value the descriptor holds.
				LookupPreimage: func(hashFunc string,
					hash []byte) bool {

					require.Equal(t, tc.hashFunc, hashFunc)
					require.Equal(t, wantHash, hash)

					return true
				},
			}

			// Without the preimage the path cannot be planned at
			// all, since the signature alone does not satisfy the
			// script.
			noPreimage := assets
			noPreimage.LookupPreimage = nil
			_, err = d.PlanAt(0, 0, noPreimage)
			require.ErrorIs(t, err, errCouldNotPlan)

			plan, err := d.PlanAt(0, 0, assets)
			require.NoError(t, err)

			satisfier := &Satisfier{
				LookupEcdsaSig: func(string) ([]byte, bool) {
					return sig, true
				},
				LookupPreimage: func(hashFunc string,
					hash []byte) ([]byte, bool) {

					require.Equal(t, tc.hashFunc, hashFunc)
					require.Equal(t, wantHash, hash)

					return preimage, true
				},
			}
			result, err := plan.Satisfy(satisfier)
			require.NoError(t, err)

			// The script checks the signature before the hash, so
			// the preimage sits below the signature on the stack,
			// with the witness script last.
			witnessScript, err := d.ScriptCodeAt(0, 0)
			require.NoError(t, err)
			require.Equal(t, [][]byte{
				preimage, sig, witnessScript,
			}, result.Witness)

			// The size the plan reported covers exactly that
			// witness.
			require.Equal(
				t, witnessSerializedSize(result.Witness),
				plan.WitnessSize(),
			)

			// A preimage of any other size cannot satisfy a hash
			// fragment, so it is refused rather than put into the
			// witness.
			satisfier.LookupPreimage = func(string,
				[]byte) ([]byte, bool) {

				return preimage[:hashPreimageLen-1], true
			}
			_, err = plan.Satisfy(satisfier)
			require.ErrorIs(t, err, errCouldNotSatisfy)
		})
	}
}

// TestUncompressedKeyWeightAndPlan checks that the satisfaction of an
// uncompressed key is accounted for and produced with the key in the form the
// script commits to: 32 bytes more per key push than a compressed one.
func TestUncompressedKeyWeightAndPlan(t *testing.T) {
	t.Parallel()

	uncompressed, err := NewDescriptor("pkh(" + genUncompressed + ")")
	require.NoError(t, err)
	compressed, err := NewDescriptor("pkh(" + genCompressed + ")")
	require.NoError(t, err)

	// The scriptSig of a P2PKH spend pushes the signature and the key, so
	// the uncompressed variant is 32 bytes, or 128 weight units, heavier.
	uncompressedWeight, err := uncompressed.MaxWeightToSatisfy()
	require.NoError(t, err)
	compressedWeight, err := compressed.MaxWeightToSatisfy()
	require.NoError(t, err)
	require.Equal(t, compressedWeight+4*32, uncompressedWeight)

	// A plan for the uncompressed key produces a scriptSig holding the key
	// as written.
	plan, err := uncompressed.PlanAt(0, 0, Assets{
		LookupEcdsaSig: func(pk string) bool {
			require.Equal(t, genUncompressed, pk)
			return true
		},
	})
	require.NoError(t, err)
	require.Zero(t, plan.WitnessSize())

	sig, err := hex.DecodeString(testEcdsaSigHex)
	require.NoError(t, err)
	result, err := plan.Satisfy(&Satisfier{
		LookupEcdsaSig: func(pk string) ([]byte, bool) {
			return sig, true
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Witness)

	keyBytes, err := hex.DecodeString(genUncompressed)
	require.NoError(t, err)
	require.Contains(t, string(result.ScriptSig), string(keyBytes))

	// The same for a legacy multisig mixing both key forms, whose
	// satisfaction is the dummy element plus one signature.
	multi, err := NewDescriptor(
		"sh(multi(1," + genCompressed + "," + genUncompressed + "))",
	)
	require.NoError(t, err)

	multiPlan, err := multi.PlanAt(0, 0, Assets{
		LookupEcdsaSig: func(pk string) bool {
			return pk == genUncompressed
		},
	})
	require.NoError(t, err)

	multiResult, err := multiPlan.Satisfy(&Satisfier{
		LookupEcdsaSig: func(pk string) ([]byte, bool) {
			if pk != genUncompressed {
				return nil, false
			}
			return sig, true
		},
	})
	require.NoError(t, err)
	require.Empty(t, multiResult.Witness)

	// The scriptSig is OP_0 - the dummy element the OP_CHECKMULTISIG
	// off-by-one bug consumes - the push of the one signature, and the push
	// of the redeem script, which holds the uncompressed key.
	redeem, err := multi.ScriptCodeAt(0, 0)
	require.NoError(t, err)
	require.Contains(t, string(redeem), string(keyBytes))

	wantScriptSig := []byte{0x00, byte(len(sig))}
	wantScriptSig = append(wantScriptSig, sig...)
	wantScriptSig = append(
		wantScriptSig, txscript.OP_PUSHDATA1, byte(len(redeem)),
	)
	wantScriptSig = append(wantScriptSig, redeem...)
	require.Equal(t, wantScriptSig, multiResult.ScriptSig)

	// The size the plan reported has to cover exactly those bytes, plus the
	// var-int that prefixes the scriptSig in the transaction.
	require.Equal(
		t, uint64(1+len(wantScriptSig)), multiPlan.ScriptSigSize(),
	)
}
