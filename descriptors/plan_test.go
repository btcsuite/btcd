package descriptors

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

const (
	// testEcdsaSigHex is a valid DER-encoded ECDSA signature (with its
	// sighash byte) used in the P2WSH plan tests.
	testEcdsaSigHex = "3045022100e621a7686d51fb23e761adff4367881a6fb16bc5" +
		"635ff34eea39afdaf033e4d702207998512f52bd3dae100951a6df9e66bc" +
		"b78c194dcaa3c7fd2451180b5cc94d4e01"
)

var (
	// testTapLeafSig is a 64-byte dummy Schnorr signature used in the plan
	// tests.
	testTapLeafSig = []byte(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaa",
	)

	// txVersionTwo is the transaction version the plan tests spend with,
	// which is the lowest one that enforces a relative locktime (BIP68).
	txVersionTwo = int32(2)
)

// TestPlanAt exercises the spending-plan construction and satisfaction for
// taproot key-path, taproot script-path and P2WSH (native and P2SH-wrapped)
// descriptors, against the descriptors-go reference values.
func TestPlanAt(t *testing.T) {
	t.Parallel()

	descriptorTr, err := NewDescriptor(testTr)
	require.NoError(t, err)

	// The definite key strings the lookups should be called with, at
	// multipath index 0 and derivation index 0.
	internalDef := "[e81a5744/48'/0'/0'/2']xpub6Duv8Gj9gZeA3sUo5nUMPEv6FZ" +
		"81GHn3feyaUej5KqcjPKsYLww4xBX4MmYZUPX5NqzaVJWYdYZwGLECtgQruG" +
		"4FkZMh566RkfUT2pbzsEg/0/0"
	leafDef := "[3c157b79/48'/0'/0'/2']xpub6DdSN9RNZi3eDjhZWA8PJ5mSuWgfmP" +
		"dBduXWzSP91Y3GxKWNwkjyc5mF9FcpTFymUh9C4Bar45b6rWv6Y5kSbi9yJD" +
		"juJUDzQSWUh3ijzXP/0/0"
	leafHash := "fc5460a80d4b2477db9612cb453da10d33c8dffa569c7c40efe94e05" +
		"91451120"

	// A taproot leaf script spend fails without a relative locktime, and
	// with one that is too small.
	t.Run("taproot leaf script fail", func(t *testing.T) {
		t.Parallel()

		leafSig := func(pk, lh string) (uint32, bool) {
			return 64, true
		}

		_, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: leafSig,
		})
		require.Error(t, err)

		tooSmall := uint32(65535 - 1)
		_, err = descriptorTr.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: leafSig,
			TxVersion:              &txVersionTwo,
			TxInputSequence:        &tooSmall,
		})
		require.Error(t, err)
	})

	t.Run("taproot leaf script OK", func(t *testing.T) {
		t.Parallel()

		locktimeOK := uint32(65535)
		plan, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: func(pk, lh string) (uint32,
				bool) {

				require.Equal(t, leafDef, pk)
				require.Equal(t, leafHash, lh)
				return 64, true
			},
			TxVersion:       &txVersionTwo,
			TxInputSequence: &locktimeOK,
		})
		require.NoError(t, err)
		require.Equal(t, uint64(144), plan.SatisfactionWeight())
		require.Equal(t, uint64(1), plan.ScriptSigSize())
		require.Equal(t, uint64(140), plan.WitnessSize())

		result, err := plan.Satisfy(&Satisfier{
			LookupTapLeafScriptSig: func(pk, lh string) ([]byte,
				bool) {

				return testTapLeafSig, true
			},
		})
		require.NoError(t, err)

		script, err := hex.DecodeString(
			"200b44e43e2f276697d23c2248f80bb09e84f702ddae399d194f" +
				"5132f472bf8713ad03ffff00b2",
		)
		require.NoError(t, err)
		controlBlock, err := hex.DecodeString(
			"c126547ceb5352bd238ca7e1da004e9d6625baf3324feda4ead6" +
				"9436042a535104",
		)
		require.NoError(t, err)
		require.Equal(t, &SatisfyResult{
			Witness: [][]byte{
				testTapLeafSig, script, controlBlock,
			},
			ScriptSig: []byte{},
		}, result)
	})

	t.Run("taproot key path spend OK", func(t *testing.T) {
		t.Parallel()

		plan, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapKeySpendSig: func(pk string) (uint32, bool) {
				require.Equal(t, internalDef, pk)
				return 64, true
			},
		})
		require.NoError(t, err)
		require.Equal(t, uint64(70), plan.SatisfactionWeight())

		result, err := plan.Satisfy(&Satisfier{
			LookupTapKeySpendSig: func() ([]byte, bool) {
				return testTapLeafSig, true
			},
		})
		require.NoError(t, err)
		require.Equal(t, &SatisfyResult{
			Witness:   [][]byte{testTapLeafSig},
			ScriptSig: []byte{},
		}, result)
	})

	t.Run("wsh OK", func(t *testing.T) {
		t.Parallel()

		descriptor, err := NewDescriptor("wsh(pk(" + testXpub1 + "))")
		require.NoError(t, err)

		// testXpub1 is the same key as the taproot internal key, so its
		// definite string at (0, 0) is internalDef.
		plan, err := descriptor.PlanAt(0, 0, Assets{
			LookupEcdsaSig: func(pk string) bool {
				require.Equal(t, internalDef, pk)
				return true
			},
		})
		require.NoError(t, err)

		// The witness holds the 72-byte signature and the 35-byte
		// witness script, each with its length prefix, plus the
		// element count: 1 + 73 + 36 bytes.
		require.Equal(t, uint64(114), plan.SatisfactionWeight())
		require.Equal(t, uint64(1), plan.ScriptSigSize())
		require.Equal(t, uint64(110), plan.WitnessSize())

		sig, err := hex.DecodeString(testEcdsaSigHex)
		require.NoError(t, err)
		result, err := plan.Satisfy(&Satisfier{
			LookupEcdsaSig: func(pk string) ([]byte, bool) {
				return sig, true
			},
		})
		require.NoError(t, err)

		// A P2WSH witness ends with the witness script, which is what
		// the descriptor's script code is.
		witnessScript, err := descriptor.ScriptCodeAt(0, 0)
		require.NoError(t, err)
		require.Len(t, witnessScript, 35)

		require.Equal(t, &SatisfyResult{
			Witness:   [][]byte{sig, witnessScript},
			ScriptSig: []byte{},
		}, result)
	})

	t.Run("wsh-sh OK", func(t *testing.T) {
		t.Parallel()

		descriptor, err := NewDescriptor(
			"sh(wsh(pk(" + testXpub1 + ")))",
		)
		require.NoError(t, err)

		plan, err := descriptor.PlanAt(0, 0, Assets{
			LookupEcdsaSig: func(pk string) bool {
				return true
			},
		})
		require.NoError(t, err)

		// The scriptSig is the 35-byte push of the P2WSH redeem script
		// plus the var-int that prefixes it in the transaction, and the
		// witness is the same as for the native P2WSH above.
		require.Equal(t, uint64(254), plan.SatisfactionWeight())
		require.Equal(t, uint64(36), plan.ScriptSigSize())
		require.Equal(t, uint64(110), plan.WitnessSize())

		sig, err := hex.DecodeString(testEcdsaSigHex)
		require.NoError(t, err)
		result, err := plan.Satisfy(&Satisfier{
			LookupEcdsaSig: func(pk string) ([]byte, bool) {
				return sig, true
			},
		})
		require.NoError(t, err)

		scriptSig, err := hex.DecodeString(
			"220020d5c86b71799a3e4f4db05698009efa8eed80a86ea47b5c" +
				"aebc47b01d5384b2f1",
		)
		require.NoError(t, err)

		witnessScript, err := descriptor.ScriptCodeAt(0, 0)
		require.NoError(t, err)

		require.Equal(t, &SatisfyResult{
			Witness:   [][]byte{sig, witnessScript},
			ScriptSig: scriptSig,
		}, result)
	})

	t.Run("plan satisfy fail", func(t *testing.T) {
		t.Parallel()

		plan, err := descriptorTr.PlanAt(0, 0, Assets{
			LookupTapKeySpendSig: func(pk string) (uint32, bool) {
				return 64, true
			},
		})
		require.NoError(t, err)

		_, err = plan.Satisfy(&Satisfier{})
		require.EqualError(t, err, "could not satisfy")
	})
}

// TestPlanTapscriptTreeSpend plans and completes a script-path spend for each
// leaf of an unbalanced taproot script tree, then verifies the resulting
// witness (leaf script and control block) with the script engine. This
// exercises control-block construction at multiple tree depths end to end.
func TestPlanTapscriptTreeSpend(t *testing.T) {
	t.Parallel()

	// Generate an internal key and three leaf keys, keyed by name.
	priv := make(map[string]*btcec.PrivateKey)
	xhex := make(map[string]string)
	for _, name := range []string{"I", "A", "B", "C"} {
		p, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		priv[name] = p
		xhex[name] = hex.EncodeToString(schnorr.SerializePubKey(
			p.PubKey(),
		))
	}

	// An unbalanced tree: leaf A at depth 1, leaves B and C at depth 2.
	desc := fmt.Sprintf("tr(%s,{pk(%s),{pk(%s),pk(%s)}})",
		xhex["I"], xhex["A"], xhex["B"], xhex["C"])
	d, err := NewDescriptor(desc)
	require.NoError(t, err)

	outputKey, err := d.taprootOutputKey(d.root, 0, 0)
	require.NoError(t, err)
	pkScript, err := txscript.PayToTaprootScript(outputKey)
	require.NoError(t, err)

	const amount = int64(100000)

	for _, leaf := range []string{"A", "B", "C"} {
		t.Run("leaf "+leaf, func(t *testing.T) {
			t.Parallel()

			leafHex := xhex[leaf]

			// Only this leaf's signature is available, so the plan
			// must choose this leaf's script path.
			plan, err := d.PlanAt(0, 0, Assets{
				LookupTapLeafScriptSig: func(pk,
					lh string) (uint32, bool) {

					return 64, pk == leafHex
				},
			})
			require.NoError(t, err)

			// Build a transaction spending the taproot output.
			prevFetcher := txscript.NewCannedPrevOutputFetcher(
				pkScript, amount,
			)
			spendTx := wire.NewMsgTx(2)
			spendTx.AddTxIn(wire.NewTxIn(
				&wire.OutPoint{}, nil, nil,
			))
			spendTx.AddTxOut(&wire.TxOut{
				Value: amount - 500, PkScript: pkScript,
			})
			sigHashes := txscript.NewTxSigHashes(
				spendTx, prevFetcher,
			)

			// Satisfy the plan by signing the chosen leaf.
			result, err := plan.Satisfy(&Satisfier{
				LookupTapLeafScriptSig: func(pk,
					lh string) ([]byte, bool) {

					if pk != leafHex {
						return nil, false
					}
					return signLeaf(
						t, spendTx, sigHashes, amount,
						pkScript, leafHex, priv[leaf],
					), true
				},
			})
			require.NoError(t, err)
			require.Empty(t, result.ScriptSig)

			// The witness must satisfy the taproot output under the
			// standard consensus rules.
			spendTx.TxIn[0].Witness = result.Witness
			engine, err := txscript.NewEngine(
				pkScript, spendTx, 0,
				txscript.StandardVerifyFlags, nil, sigHashes,
				amount, prevFetcher,
			)
			require.NoError(t, err)
			require.NoError(t, engine.Execute())
		})
	}
}

// signLeaf produces a BIP340 signature over the tapscript leaf "pk(<key>)" for
// the given signer, used to complete a plan in the taproot tree spend test.
func signLeaf(t *testing.T, tx *wire.MsgTx, sigHashes *txscript.TxSigHashes,
	amount int64, pkScript []byte, keyHex string,
	signer *btcec.PrivateKey) []byte {

	t.Helper()

	// Reconstruct the selected leaf so the signature commits to its exact
	// script, not just to the public key used by the lookup.
	keyBytes, err := hex.DecodeString(keyHex)
	require.NoError(t, err)

	leafScript, err := txscript.NewScriptBuilder().AddData(keyBytes).
		AddOp(txscript.OP_CHECKSIG).Script()
	require.NoError(t, err)

	sig, err := txscript.RawTxInTapscriptSignature(
		tx, sigHashes, 0, amount, pkScript,
		txscript.NewBaseTapLeaf(leafScript), txscript.SigHashDefault,
		signer,
	)
	require.NoError(t, err)
	return sig
}

// TestPlanWeights checks the weight accounting of a plan: the satisfaction
// weight is the witness size plus four times the scriptSig size, the getters
// return the stored sizes, and Satisfy dispatches to the plan's closure.
func TestPlanWeights(t *testing.T) {
	t.Parallel()

	called := false
	plan := &Plan{
		witnessSize:   100,
		scriptSigSize: 35,
		satisfy: func(*Satisfier) (*SatisfyResult, error) {
			called = true
			return &SatisfyResult{ScriptSig: []byte{0x01}}, nil
		},
	}

	require.Equal(t, uint64(100), plan.WitnessSize())
	require.Equal(t, uint64(35), plan.ScriptSigSize())
	require.Equal(t, uint64(100+35*4), plan.SatisfactionWeight())

	result, err := plan.Satisfy(&Satisfier{})
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, []byte{0x01}, result.ScriptSig)
}

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

			checkLocktimePlan(t, tc.after, Assets{
				LookupEcdsaSig: func(string) bool {
					return true
				},
				TxLockTime:      tc.lockTime,
				TxInputSequence: tc.sequence,
			}, tc.want)
		})
	}
}

// TestBranchHash checks that the taproot branch hash matches the reference
// txscript implementation and is independent of the argument order (the two
// child hashes are sorted before hashing).
func TestBranchHash(t *testing.T) {
	t.Parallel()

	leafA := txscript.NewBaseTapLeaf([]byte{txscript.OP_1})
	leafB := txscript.NewBaseTapLeaf([]byte{txscript.OP_2})
	hashA, hashB := leafA.TapHash(), leafB.TapHash()

	want := txscript.NewTapBranch(leafA, leafB).TapHash()
	require.Equal(t, want, branchHash(hashA, hashB))

	// Swapping the two children yields the same branch hash.
	require.Equal(t, branchHash(hashA, hashB), branchHash(hashB, hashA))
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

	// A taproot script-path plan has a satisfy closure of its own, and it
	// is only chosen when the key path is unavailable, so it is planned
	// separately: the single leaf of testTr needs a relative locktime of
	// 65535 blocks.
	t.Run("taproot leaf script", func(t *testing.T) {
		t.Parallel()

		d, err := NewDescriptor(testTr)
		require.NoError(t, err)

		leafSequence := uint32(65535)
		plan, err := d.PlanAt(0, 0, Assets{
			LookupTapLeafScriptSig: func(string, string) (uint32,
				bool) {

				return 64, true
			},
			TxVersion:       &version,
			TxInputSequence: &leafSequence,
		})
		require.NoError(t, err)

		_, err = plan.Satisfy(nil)
		require.ErrorIs(t, err, errCouldNotSatisfy)
	})
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
		"tr(" + key[2:] + ")",
		testTr,
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
			require.Equal(t, tc.want, k.definiteString(
				tc.mp, tc.idx,
			))
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

			checkLocktimePlan(t, tc.older, Assets{
				LookupEcdsaSig: func(string) bool {
					return true
				},
				TxVersion:       tc.version,
				TxInputSequence: tc.sequence,
			}, tc.want)
		})
	}
}

// checkLocktimePlan plans a spend of a key-and-timelock descriptor holding the
// given locktime fragment and asserts whether the given assets can satisfy it.
func checkLocktimePlan(t *testing.T, fragment string, assets Assets,
	want bool) {

	t.Helper()

	// Require both a signature and the locktime, so no alternative path can
	// make missing transaction context appear sufficient.
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

// TestPlanSizesMatchSatisfaction checks, for every descriptor in the corpus,
// that the sizes a plan reports cover exactly the witness and scriptSig its
// Satisfy produces. Signatures are handed out in exactly the sizes the plan
// assumes, so the comparison is exact.
//
// The legacy P2SH plans used to fail this: they produced a scriptSig without
// the redeem script push, which is not a valid spend at all, while reporting a
// size that did not account for it either.
func TestPlanSizesMatchSatisfaction(t *testing.T) {
	t.Parallel()

	// The plan assumes 72 bytes for an ECDSA signature; the taproot lookups
	// state the size themselves, so 64 is what they promise below.
	ecdsaSig := make([]byte, assumedEcdsaSigLen)
	schnorrSig := make([]byte, 64)

	sequence, lockTime := uint32(65535), uint32(499999999)
	assets := Assets{
		LookupEcdsaSig: func(string) bool { return true },
		LookupTapKeySpendSig: func(string) (uint32, bool) {
			return uint32(len(schnorrSig)), true
		},
		LookupTapLeafScriptSig: func(string, string) (uint32, bool) {
			return uint32(len(schnorrSig)), true
		},
		TxVersion:       &txVersionTwo,
		TxLockTime:      &lockTime,
		TxInputSequence: &sequence,
	}
	satisfier := &Satisfier{
		LookupEcdsaSig: func(string) ([]byte, bool) {
			return ecdsaSig, true
		},
		LookupTapKeySpendSig: func() ([]byte, bool) {
			return schnorrSig, true
		},
		LookupTapLeafScriptSig: func(string, string) ([]byte, bool) {
			return schnorrSig, true
		},
	}

	checked := 0
	for _, desc := range loadCorpus(t) {
		d, err := NewDescriptor(desc)
		require.NoErrorf(t, err, "parse %s", desc)

		plan, err := d.PlanAt(0, 0, assets)
		if err != nil {
			continue
		}

		result, err := plan.Satisfy(satisfier)
		require.NoErrorf(t, err, "satisfy %s", desc)
		checked++

		// A non-segwit spend has no witness at all, which the plan
		// reports as a size of zero rather than as the one byte an
		// empty witness stack would serialize to.
		wantWitness := uint64(0)
		if len(result.Witness) > 0 {
			wantWitness = witnessSerializedSize(result.Witness)
		}
		require.Equalf(
			t, wantWitness, plan.WitnessSize(),
			"witness size for %s", desc,
		)
		require.Equalf(
			t, uint64(varintLen(uint64(len(result.ScriptSig)))+
				len(result.ScriptSig)), plan.ScriptSigSize(),
			"scriptSig size for %s", desc,
		)
	}

	require.NotZero(t, checked)
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
			satisfier.LookupPreimage = func(string, []byte) ([]byte,
				bool) {

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
	wantScriptSig = append(wantScriptSig, txscript.OP_PUSHDATA1, byte(
		len(redeem),
	))
	wantScriptSig = append(wantScriptSig, redeem...)
	require.Equal(t, wantScriptSig, multiResult.ScriptSig)

	// The size the plan reported has to cover exactly those bytes, plus the
	// var-int that prefixes the scriptSig in the transaction.
	require.Equal(
		t, uint64(1+len(wantScriptSig)), multiPlan.ScriptSigSize(),
	)
}

// TestPlanAssetSizeBounds checks that a signature size reported by an asset
// provider is bounded before it is turned into a plan's dummy signature: a size
// that cannot describe a BIP341 signature is treated as unavailable rather than
// allocated, so a provider backed by untrusted data cannot make PlanAt
// allocate 4 GB per lookup.
func TestPlanAssetSizeBounds(t *testing.T) {
	t.Parallel()

	descriptor, err := NewDescriptor(testTr)
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		size    uint32
		wantErr bool
	}{{
		// A signature of no bytes is not one that is available but the
		// dissatisfaction of one, which the satisfier produces itself
		// where a fragment allows it.
		name:    "empty signature",
		size:    0,
		wantErr: true,
	}, {
		name:    "one byte",
		size:    1,
		wantErr: true,
	}, {
		name:    "one byte too small",
		size:    63,
		wantErr: true,
	}, {
		name: "default sighash signature",
		size: 64,
	}, {
		name: "explicit sighash signature",
		size: 65,
	}, {
		name:    "one byte too large",
		size:    66,
		wantErr: true,
	}, {
		name:    "four gigabytes",
		size:    0xffffffff,
		wantErr: true,
	}} {

		t.Run(tc.name+" key spend", func(t *testing.T) {
			t.Parallel()

			plan, err := descriptor.PlanAt(0, 0, Assets{
				LookupTapKeySpendSig: func(string) (uint32,
					bool) {

					return tc.size, true
				},
			})
			if tc.wantErr {
				require.ErrorIs(t, err, errCouldNotPlan)
				return
			}

			require.NoError(t, err)
			require.Equal(
				t, uint64(tc.size)+6, plan.SatisfactionWeight(),
			)
		})

		t.Run(tc.name+" leaf script", func(t *testing.T) {
			t.Parallel()

			// The single leaf of testTr also requires a relative
			// locktime of 65535 blocks.
			sequence := uint32(65535)
			plan, err := descriptor.PlanAt(0, 0, Assets{
				LookupTapLeafScriptSig: func(string,
					string) (uint32, bool) {

					return tc.size, true
				},
				TxVersion:       &txVersionTwo,
				TxInputSequence: &sequence,
			})
			if tc.wantErr {
				require.ErrorIs(t, err, errCouldNotPlan)
				return
			}

			require.NoError(t, err)
			require.NotZero(t, plan.SatisfactionWeight())
		})
	}
}

// TestPlanTapSigSizes checks that a concrete taproot signature which cannot be
// a valid BIP341 one is refused at satisfaction time rather than put into a
// witness: only 64 bytes, or 65 bytes ending in a sighash type other than the
// default, are valid, and script validation rejects everything else.
func TestPlanTapSigSizes(t *testing.T) {
	t.Parallel()

	descriptor, err := NewDescriptor(testTr)
	require.NoError(t, err)

	tests := []struct {
		name    string
		sig     []byte
		wantErr bool
	}{{
		name: "default sighash",
		sig:  make([]byte, 64),
	}, {
		name: "explicit sighash",
		sig:  append(make([]byte, 64), byte(txscript.SigHashAll)),
	}, {
		// A 65-byte signature carries the sighash type it was made
		// for, and the default type is the one that is expressed by
		// leaving the byte out (BIP341).
		name: "default sighash spelled out",
		sig: append(
			make([]byte, 64), byte(txscript.SigHashDefault),
		),
		wantErr: true,
	}, {
		name:    "empty",
		sig:     []byte{},
		wantErr: true,
	}, {
		name:    "one byte too small",
		sig:     make([]byte, 63),
		wantErr: true,
	}, {
		name:    "one byte too large",
		sig:     make([]byte, 66),
		wantErr: true,
	}}

	// The single leaf of testTr also requires a relative locktime of 65535
	// blocks, so a leaf-script plan needs the transaction context too.
	sequence := uint32(65535)
	for flag := range 256 {
		// BIP341 defines exactly six explicit sighash bytes. The
		// default is only permitted through the implicit 64-byte
		// encoding.
		valid := flag == 1 || flag == 2 || flag == 3 || flag == 0x81 ||
			flag == 0x82 || flag == 0x83
		tests = append(tests, struct {
			name    string
			sig     []byte
			wantErr bool
		}{
			name:    fmt.Sprintf("sighash byte %02x", flag),
			sig:     append(make([]byte, 64), byte(flag)),
			wantErr: !valid,
		})
	}

	for _, tc := range tests {
		t.Run(tc.name+" key spend", func(t *testing.T) {
			t.Parallel()

			plan, err := descriptor.PlanAt(0, 0, Assets{
				LookupTapKeySpendSig: func(string) (uint32,
					bool) {

					return 64, true
				},
			})
			require.NoError(t, err)

			result, err := plan.Satisfy(&Satisfier{
				LookupTapKeySpendSig: func() ([]byte, bool) {
					return tc.sig, true
				},
			})
			if tc.wantErr {
				require.ErrorIs(t, err, errCouldNotSatisfy)

				return
			}

			require.NoError(t, err)
			require.Equal(t, [][]byte{tc.sig}, result.Witness)
		})

		t.Run(tc.name+" leaf script", func(t *testing.T) {
			t.Parallel()

			plan, err := descriptor.PlanAt(0, 0, Assets{
				LookupTapLeafScriptSig: func(string,
					string) (uint32, bool) {

					return 64, true
				},
				TxVersion:       &txVersionTwo,
				TxInputSequence: &sequence,
			})
			require.NoError(t, err)

			result, err := plan.Satisfy(&Satisfier{
				LookupTapLeafScriptSig: func(string,
					string) ([]byte, bool) {

					return tc.sig, true
				},
			})
			if tc.wantErr {
				require.ErrorIs(t, err, errCouldNotSatisfy)

				return
			}

			require.NoError(t, err)

			// The witness is the signature, the leaf script and the
			// control block.
			require.Len(t, result.Witness, 3)
			require.Equal(t, tc.sig, result.Witness[0])
		})
	}
}

// TestPlanKeySpellingTwins checks that a descriptor which spells the same
// public key in two different ways can be planned and satisfied with the
// signature registered under either spelling. The 33-byte compressed and the
// 32-byte x-only form of a point are the same key in the P2TR context, so the
// key expressions of two tap leaves can derive to identical bytes.
func TestPlanKeySpellingTwins(t *testing.T) {
	t.Parallel()

	// Derive one point and take both of its spellings.
	key, err := parseDescKey(basicTestXpub, keyFormCompressed)
	require.NoError(t, err)
	pub, err := key.derivePub(0, 0)
	require.NoError(t, err)
	compressed := hex.EncodeToString(pub.SerializeCompressed())
	xOnly := hex.EncodeToString(schnorr.SerializePubKey(pub))

	// Both leaves are spendable with a signature of the same point, one
	// spelled x-only and one compressed.
	descriptor, err := NewDescriptor(
		"tr(" + testXpub1 + ",{pk(" + xOnly + "),pkh(" + compressed +
			")})",
	)
	require.NoError(t, err)

	for _, spelling := range []string{xOnly, compressed} {
		t.Run(spelling[:8], func(t *testing.T) {
			t.Parallel()

			plan, err := descriptor.PlanAt(0, 0, Assets{
				LookupTapLeafScriptSig: func(pk,
					_ string) (uint32, bool) {

					return 64, pk == spelling
				},
			})
			require.NoError(t, err)

			result, err := plan.Satisfy(&Satisfier{
				LookupTapLeafScriptSig: func(pk,
					_ string) ([]byte, bool) {

					return testTapLeafSig, pk == spelling
				},
			})
			require.NoError(t, err)
			require.Equal(t, testTapLeafSig, result.Witness[0])
		})
	}
}
